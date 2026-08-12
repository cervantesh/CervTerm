//go:build ignore

// package-macos assembles a CervTerm.app bundle for macOS: it builds the
// GLFW binary, lays out the standard Contents/{MacOS,Resources} bundle
// structure with Info.plist and the app icon, ad-hoc signs it, and zips the
// result. It intentionally does not perform Developer ID signing or Apple
// notarization -- those require an Apple Developer Program certificate that
// this script has no access to; see docs/release-macos.md.
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

func main() {
	version := flag.String("version", "0.1.0-dev", "package version (CFBundleVersion / CFBundleShortVersionString)")
	outDir := flag.String("outdir", "dist", "output directory")
	sign := flag.Bool("adhoc-sign", true, "ad-hoc codesign the assembled .app (codesign --sign -)")
	flag.Parse()
	must(packageMacOS(*version, *outDir, *sign))
}

func packageMacOS(version, outDir string, adHocSign bool) error {
	// The build step below produces a host-native binary, and the bundle is
	// only meaningful with a Mach-O executable inside it. Without this guard a
	// run on Linux or Windows would emit a plausible-looking
	// cervterm-<version>-macos.zip containing a non-Darwin binary. Cross
	// compiling is not a goal here: the GLFW build needs cgo against the macOS
	// system frameworks, and the xattr/codesign steps are macOS-only tools.
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("package-macos must run on macOS: GOOS is %q, want %q", runtime.GOOS, "darwin")
	}
	if err := validatePackageVersion(version); err != nil {
		return err
	}
	appDir := filepath.Join(outDir, "CervTerm.app")
	zipPath := filepath.Join(outDir, "cervterm-"+version+"-macos.zip")

	if err := os.RemoveAll(appDir); err != nil {
		return err
	}
	macOSDir := filepath.Join(appDir, "Contents", "MacOS")
	resourcesDir := filepath.Join(appDir, "Contents", "Resources")
	for _, dir := range []string{macOSDir, resourcesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	exe := filepath.Join(macOSDir, "cervterm")
	// -s -w strip the symbol table and DWARF, matching the Windows beta build.
	ldflags := "-s -w -X cervterm/internal/buildinfo.Version=" + version
	cmd := exec.Command("go", "build", "-tags", "glfw", "-trimpath", "-ldflags", ldflags, "-o", exe, "./cmd/cervterm")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if err := writeInfoPlist(filepath.Join(appDir, "Contents", "Info.plist"), version); err != nil {
		return err
	}
	if err := copyFile(filepath.Join("packaging", "macos", "AppIcon.icns"), filepath.Join(resourcesDir, "AppIcon.icns")); err != nil {
		return err
	}
	for _, file := range []string{"README.md", "CHANGELOG.md", "SUPPORT.md", "LICENSE"} {
		if err := copyFile(file, filepath.Join(resourcesDir, file)); err != nil {
			return err
		}
	}
	if err := copyDir("docs", filepath.Join(resourcesDir, "docs")); err != nil {
		return err
	}

	if adHocSign {
		// macOS (Sequoia and later) stamps a com.apple.provenance xattr on
		// files as they're written/copied; codesign refuses to sign a
		// bundle containing it ("resource fork, Finder information, or
		// similar detritus not allowed"). Strip xattrs recursively first.
		clearCmd := exec.Command("xattr", "-cr", appDir)
		clearCmd.Stdout, clearCmd.Stderr = os.Stdout, os.Stderr
		if err := clearCmd.Run(); err != nil {
			return fmt.Errorf("clear extended attributes: %w", err)
		}
		// Ad-hoc signing (identity "-") requires no certificate; it lets
		// Gatekeeper attribute the binary to a stable identity for local
		// testing. It is not a substitute for Developer ID signing.
		signCmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", appDir)
		signCmd.Stdout, signCmd.Stderr = os.Stdout, os.Stderr
		if err := signCmd.Run(); err != nil {
			return fmt.Errorf("ad-hoc codesign: %w", err)
		}
	}

	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := zipDirContents(outDir, appDir, zipPath); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", zipPath)
	return nil
}

type plistData struct {
	Version      string
	ShortVersion string
}

func writeInfoPlist(dst, version string) error {
	tmplBytes, err := os.ReadFile(filepath.Join("packaging", "macos", "Info.plist.template"))
	if err != nil {
		return err
	}
	tmpl, err := template.New("Info.plist").Parse(string(tmplBytes))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	plistVersion, exact := normalizePlistVersion(version)
	if !exact {
		fmt.Fprintf(os.Stderr, "warning: version %q is not a dotted-integer version; using %q for CFBundleVersion/CFBundleShortVersionString\n", version, plistVersion)
	}
	if err := tmpl.Execute(&buf, plistData{Version: plistVersion, ShortVersion: plistVersion}); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}

// fallbackPlistVersion is emitted when a version string cannot be reduced to
// the form Apple requires. A known-valid placeholder is better than a value
// the platform rejects: macOS parses these keys numerically, so a malformed
// one degrades version comparison and is rejected outright at notarization.
const fallbackPlistVersion = "0.0.0"

// normalizePlistVersion reduces a release version to the form Apple requires
// for CFBundleVersion and CFBundleShortVersionString: one to three
// period-separated integers. It strips a leading "v" and any SemVer
// prerelease/build suffix, so this repo's tag convention "v0.2.0-beta.1"
// becomes "0.2.0". Components are canonicalized as integers ("1.02" -> "1.2"),
// which is how macOS compares them anyway. More than three components are
// truncated to three. Anything left that is not dotted-integer form yields
// fallbackPlistVersion.
//
// The second result reports whether the input was already a clean
// dotted-integer version, so the caller can warn when a value was substituted.
//
// Only the two plist keys are constrained this way. The full unmodified
// version string is still what gets stamped into the binary via
// -X buildinfo.Version and into the release zip filename, so prerelease
// identity is preserved everywhere it is legal to keep it.
func normalizePlistVersion(version string) (string, bool) {
	trimmed := strings.TrimSpace(version)
	exact := trimmed == version
	if rest := strings.TrimPrefix(trimmed, "v"); rest != trimmed {
		trimmed, exact = rest, false
	} else if rest := strings.TrimPrefix(trimmed, "V"); rest != trimmed {
		trimmed, exact = rest, false
	}
	// Cut the SemVer prerelease ("-beta.1") and build metadata ("+abc123")
	// suffixes. This also removes any stray sign character, so the digit check
	// below sees only unsigned components.
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed, exact = trimmed[:idx], false
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 3 {
		parts, exact = parts[:3], false
	}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		// Bound the length so the Atoi below stays well inside int range and
		// absurd components are rejected rather than silently accepted.
		if part == "" || len(part) > 9 {
			return fallbackPlistVersion, false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return fallbackPlistVersion, false
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return fallbackPlistVersion, false
		}
		canonical := strconv.Itoa(n)
		if canonical != part {
			exact = false
		}
		normalized = append(normalized, canonical)
	}
	if len(normalized) == 0 {
		return fallbackPlistVersion, false
	}
	return strings.Join(normalized, "."), exact
}

func validatePackageVersion(version string) error {
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("package version must not be empty")
	}
	if strings.HasPrefix(version, ".") || strings.Contains(version, "..") {
		return fmt.Errorf("unsafe package version %q", version)
	}
	for _, r := range version {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' || r == '+' {
			continue
		}
		return fmt.Errorf("unsafe package version %q", version)
	}
	return nil
}

func copyDir(src, dst string) error {
	// dst inside src means the walk would descend into its own output: with
	// -outdir docs the bundle stages at docs/CervTerm.app, so copying docs/
	// into that bundle's Resources/docs walks the growing copy back into
	// itself. There is no legitimate case for the overlap here, so reject it
	// outright rather than emitting a bundle that contains a partial copy of
	// itself.
	nested, err := isWithin(src, dst)
	if err != nil {
		return err
	}
	if nested {
		return fmt.Errorf("refusing to copy %s into %s: destination is inside the source tree", src, dst)
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// isWithin reports whether child is parent itself or nested underneath it.
// Both paths are resolved to absolute form first so a relative source ("docs")
// and an absolute destination compare correctly.
func isWithin(parent, child string) (bool, error) {
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return false, err
	}
	absChild, err := filepath.Abs(child)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absParent, absChild)
	if err != nil {
		// Unrelated roots (different volumes on Windows) are not nested.
		return false, nil
	}
	if rel == "." {
		return true, nil
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	info, err := in.Stat()
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

func zipDirContents(baseDir, srcDir, zipPath string) (err error) {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	// zip.Writer.Close writes the central directory and os.File.Close flushes
	// it, so both can fail late (disk full, IO error) on an archive whose
	// entries all wrote cleanly. Discarding those errors would leave a
	// truncated, unreadable zip while the caller reports success, so propagate
	// them -- but never let them mask an earlier failure.
	defer func() {
		if closeErr := zw.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("finalize zip %s: %w", zipPath, closeErr)
		}
		if outErr := out.Close(); err == nil && outErr != nil {
			err = fmt.Errorf("close zip %s: %w", zipPath, outErr)
		}
	}()
	return filepath.WalkDir(srcDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		if entry.IsDir() {
			header.Name += "/"
			_, err := zw.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
