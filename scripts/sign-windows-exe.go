//go:build ignore

package main

import (
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

func main() {
	exePath := flag.String("exe", "", "executable to sign")
	pfxBase64 := flag.String("pfx-base64", os.Getenv("WINDOWS_CODESIGN_PFX_BASE64"), "base64-encoded PFX")
	pfxPassword := flag.String("pfx-password", os.Getenv("WINDOWS_CODESIGN_PASSWORD"), "PFX password")
	timestampURL := flag.String("timestamp-url", "http://timestamp.digicert.com", "RFC3161 timestamp URL")
	skipIfMissing := flag.Bool("skip-if-missing", false, "skip signing when code-signing material is not configured")
	flag.Parse()
	if *exePath == "" {
		fmt.Fprintln(os.Stderr, "-exe is required")
		os.Exit(2)
	}
	if *pfxBase64 == "" || *pfxPassword == "" {
		if *skipIfMissing {
			fmt.Println("Code-signing material is not configured; skipping Authenticode signing.")
			return
		}
		fmt.Fprintln(os.Stderr, "-pfx-base64 and -pfx-password are required")
		os.Exit(2)
	}
	if err := signWindowsExe(*exePath, *pfxBase64, *pfxPassword, *timestampURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// signWindowsExe signs exePath with the certificate contained in the
// base64-encoded PFX. The PFX password is only ever handed to the Windows
// CryptoAPI as an in-process UTF-16 string; it is never written to disk nor
// passed on any command line. The certificate is imported into the current
// user's personal store, signtool signs the binary by thumbprint, and the
// imported certificate plus its private key are removed afterwards.
func signWindowsExe(exePath, pfxBase64, pfxPassword, timestampURL string) error {
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("executable not found: %s: %w", exePath, err)
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windows Authenticode signing is only supported on Windows")
	}
	signtool := findSigntool()
	if signtool == "" {
		return fmt.Errorf("signtool.exe not found. Install Windows SDK or run on windows-latest")
	}
	pfxBytes, err := base64.StdEncoding.DecodeString(pfxBase64)
	if err != nil {
		return fmt.Errorf("decode PFX: %w", err)
	}

	memStore, err := pfxImportCertStore(pfxBytes, pfxPassword)
	if err != nil {
		return err
	}
	defer certCloseStore(memStore)

	cert, err := findSigningCert(memStore)
	if err != nil {
		return err
	}
	defer procCertFreeCertificateContext.Call(cert)

	thumbprint, err := certThumbprint(cert)
	if err != nil {
		return err
	}

	myStore, err := openCurrentUserStore("MY")
	if err != nil {
		return err
	}
	defer certCloseStore(myStore)

	stored, err := addCertToStore(myStore, cert)
	if err != nil {
		return err
	}
	// Remove the certificate and its private key from the machine once signing
	// is done so no code-signing material lingers after the build step.
	defer func() {
		deleteKeyContainer(stored)
		procCertDeleteCertificateFromStore.Call(stored) // also frees the context
	}()

	if err := run(signtool, "sign", "/fd", "SHA256", "/tr", timestampURL, "/td", "SHA256", "/sha1", thumbprint, exePath); err != nil {
		return err
	}
	if err := run(signtool, "verify", "/pa", "/v", exePath); err != nil {
		return err
	}
	fmt.Printf("Signed %s (thumbprint %s)\n", exePath, thumbprint)
	return nil
}

var (
	crypt32  = syscall.NewLazyDLL("crypt32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procPFXImportCertStore                = crypt32.NewProc("PFXImportCertStore")
	procCertEnumCertificatesInStore       = crypt32.NewProc("CertEnumCertificatesInStore")
	procCertGetCertificateContextProperty = crypt32.NewProc("CertGetCertificateContextProperty")
	procCertOpenStore                     = crypt32.NewProc("CertOpenStore")
	procCertCloseStore                    = crypt32.NewProc("CertCloseStore")
	procCertAddCertificateContextToStore  = crypt32.NewProc("CertAddCertificateContextToStore")
	procCertDeleteCertificateFromStore    = crypt32.NewProc("CertDeleteCertificateFromStore")
	procCertFreeCertificateContext        = crypt32.NewProc("CertFreeCertificateContext")
	procCertDuplicateCertificateContext   = crypt32.NewProc("CertDuplicateCertificateContext")
	procCryptAcquireContextW              = advapi32.NewProc("CryptAcquireContextW")
)

const (
	certKeyProvInfoPropID       = 2
	certHashPropID              = 3
	certStoreProvSystemW        = 10
	certSystemStoreCurrentUser  = 0x00010000
	certStoreAddReplaceExisting = 3

	cryptUserKeyset = 0x00001000 // PFXImportCertStore: import keys into the user store

	cryptDeleteKeyset = 0x00000010 // CryptAcquireContext: delete the named key container
	cryptSilent       = 0x00000040
)

type cryptDataBlob struct {
	cbData uint32
	pbData *byte
}

// cryptKeyProvInfo mirrors CRYPT_KEY_PROV_INFO. Only the fields needed to
// locate and delete the key container are read.
type cryptKeyProvInfo struct {
	ContainerName *uint16
	ProvName      *uint16
	ProvType      uint32
	Flags         uint32
	ProvParamCnt  uint32
	ProvParam     uintptr
	KeySpec       uint32
}

// pfxImportCertStore imports a PFX blob into an in-memory certificate store,
// persisting the private key so a separate signtool process can use it.
func pfxImportCertStore(pfx []byte, password string) (syscall.Handle, error) {
	blob := cryptDataBlob{cbData: uint32(len(pfx))}
	if len(pfx) > 0 {
		blob.pbData = &pfx[0]
	}
	pw, err := syscall.UTF16PtrFromString(password)
	if err != nil {
		return 0, err
	}
	r, _, e := procPFXImportCertStore.Call(
		uintptr(unsafe.Pointer(&blob)),
		uintptr(unsafe.Pointer(pw)),
		uintptr(cryptUserKeyset),
	)
	if r == 0 {
		return 0, fmt.Errorf("PFXImportCertStore failed (wrong password or invalid PFX): %w", e)
	}
	return syscall.Handle(r), nil
}

// findSigningCert returns a duplicated context for the first certificate in the
// store that carries private-key provider info. The caller owns the returned
// context and must free it with CertFreeCertificateContext.
func findSigningCert(store syscall.Handle) (uintptr, error) {
	var prev uintptr
	for {
		cert, _, _ := procCertEnumCertificatesInStore.Call(uintptr(store), prev)
		if cert == 0 {
			return 0, fmt.Errorf("no certificate with an associated private key found in PFX")
		}
		if certHasProperty(cert, certKeyProvInfoPropID) {
			dup, _, _ := procCertDuplicateCertificateContext.Call(cert)
			procCertFreeCertificateContext.Call(cert)
			if dup == 0 {
				return 0, fmt.Errorf("CertDuplicateCertificateContext failed")
			}
			return dup, nil
		}
		prev = cert
	}
}

func certHasProperty(cert uintptr, propID uint32) bool {
	var size uint32
	r, _, _ := procCertGetCertificateContextProperty.Call(cert, uintptr(propID), 0, uintptr(unsafe.Pointer(&size)))
	return r != 0
}

func certThumbprint(cert uintptr) (string, error) {
	size := uint32(20)
	buf := make([]byte, size)
	r, _, e := procCertGetCertificateContextProperty.Call(cert, certHashPropID, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "", fmt.Errorf("read certificate thumbprint failed: %w", e)
	}
	return hex.EncodeToString(buf[:size]), nil
}

func openCurrentUserStore(name string) (syscall.Handle, error) {
	storeName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	r, _, e := procCertOpenStore.Call(
		uintptr(certStoreProvSystemW),
		0,
		0,
		uintptr(certSystemStoreCurrentUser),
		uintptr(unsafe.Pointer(storeName)),
	)
	if r == 0 {
		return 0, fmt.Errorf("CertOpenStore(%s) failed: %w", name, e)
	}
	return syscall.Handle(r), nil
}

// addCertToStore copies cert into store and returns the store's own context so
// the certificate can later be deleted again.
func addCertToStore(store syscall.Handle, cert uintptr) (uintptr, error) {
	var stored uintptr
	r, _, e := procCertAddCertificateContextToStore.Call(
		uintptr(store),
		cert,
		uintptr(certStoreAddReplaceExisting),
		uintptr(unsafe.Pointer(&stored)),
	)
	if r == 0 {
		return 0, fmt.Errorf("CertAddCertificateContextToStore failed: %w", e)
	}
	return stored, nil
}

// deleteKeyContainer best-effort removes the persisted CAPI key container tied
// to the certificate. CNG-backed keys are left to the (ephemeral) runner.
func deleteKeyContainer(cert uintptr) {
	var size uint32
	if r, _, _ := procCertGetCertificateContextProperty.Call(cert, certKeyProvInfoPropID, 0, uintptr(unsafe.Pointer(&size))); r == 0 || size == 0 {
		return
	}
	buf := make([]byte, size)
	if r, _, _ := procCertGetCertificateContextProperty.Call(cert, certKeyProvInfoPropID, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return
	}
	info := (*cryptKeyProvInfo)(unsafe.Pointer(&buf[0]))
	if info.ProvType == 0 {
		return // CNG key; not deletable via CryptAcquireContext
	}
	var hProv uintptr
	procCryptAcquireContextW.Call(
		uintptr(unsafe.Pointer(&hProv)),
		uintptr(unsafe.Pointer(info.ContainerName)),
		uintptr(unsafe.Pointer(info.ProvName)),
		uintptr(info.ProvType),
		uintptr(cryptDeleteKeyset|cryptSilent),
	)
}

func certCloseStore(store syscall.Handle) {
	procCertCloseStore.Call(uintptr(store), 0)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findSigntool() string {
	if p, err := exec.LookPath("signtool.exe"); err == nil {
		return p
	}
	if runtime.GOOS != "windows" {
		return ""
	}
	root := os.Getenv("ProgramFiles(x86)")
	if root == "" {
		return ""
	}
	kits := filepath.Join(root, "Windows Kits", "10", "bin")
	var matches []string
	_ = filepath.WalkDir(kits, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(entry.Name(), "signtool.exe") && strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/x64/") {
			matches = append(matches, path)
		}
		return nil
	})
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}
