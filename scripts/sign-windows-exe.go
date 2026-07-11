//go:build ignore

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func main() {
	exePath := flag.String("exe", "", "executable to sign")
	pfxBase64 := flag.String("pfx-base64", os.Getenv("WINDOWS_CODESIGN_PFX_BASE64"), "base64-encoded PFX")
	pfxPassword := flag.String("pfx-password", os.Getenv("WINDOWS_CODESIGN_PASSWORD"), "PFX password")
	timestampURL := flag.String("timestamp-url", "http://timestamp.digicert.com", "RFC3161 timestamp URL")
	flag.Parse()
	if *exePath == "" || *pfxBase64 == "" || *pfxPassword == "" {
		fmt.Fprintln(os.Stderr, "-exe, -pfx-base64, and -pfx-password are required")
		os.Exit(2)
	}
	if err := signWindowsExe(*exePath, *pfxBase64, *pfxPassword, *timestampURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func signWindowsExe(exePath, pfxBase64, pfxPassword, timestampURL string) error {
	if _, err := os.Stat(exePath); err != nil {
		return fmt.Errorf("executable not found: %s: %w", exePath, err)
	}
	signtool := findSigntool()
	if signtool == "" {
		return fmt.Errorf("signtool.exe not found. Install Windows SDK or run on windows-latest")
	}
	pfxBytes, err := base64.StdEncoding.DecodeString(pfxBase64)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "cervterm-codesign-*.pfx")
	if err != nil {
		return err
	}
	pfxPath := tmp.Name()
	defer os.Remove(pfxPath)
	if _, err := tmp.Write(pfxBytes); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := run(signtool, "sign", "/fd", "SHA256", "/tr", timestampURL, "/td", "SHA256", "/f", pfxPath, "/p", pfxPassword, exePath); err != nil {
		return err
	}
	if err := run(signtool, "verify", "/pa", "/v", exePath); err != nil {
		return err
	}
	fmt.Printf("Signed %s\n", exePath)
	return nil
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
