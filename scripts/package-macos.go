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
	shortVersion := version
	if idx := strings.IndexAny(version, "-+"); idx >= 0 {
		shortVersion = version[:idx]
	}
	if err := tmpl.Execute(&buf, plistData{Version: version, ShortVersion: shortVersion}); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
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

func zipDirContents(baseDir, srcDir, zipPath string) error {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
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
