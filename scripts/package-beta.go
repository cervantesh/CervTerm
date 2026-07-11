//go:build ignore

package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	version := flag.String("version", "v0.2.0-beta.1", "package version")
	outDir := flag.String("outdir", "dist", "output directory")
	reuse := flag.Bool("reuse", false, "reuse an existing package directory and only rewrite the zip")
	flag.Parse()
	must(packageBeta(*version, *outDir, *reuse))
}

func packageBeta(version, outDir string, reuse bool) error {
	pkgDir := filepath.Join(outDir, "cervterm-"+version+"-windows")
	zipPath := filepath.Join(outDir, "cervterm-"+version+"-windows.zip")
	if !reuse {
		if err := os.RemoveAll(pkgDir); err != nil {
			return err
		}
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			return err
		}
		exe := filepath.Join(pkgDir, "cervterm.exe")
		cmd := exec.Command("go", "build", "-tags", "glfw", "-ldflags", "-X cervterm/internal/buildinfo.Version="+version, "-o", exe, "./cmd/cervterm")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		cfg, err := exec.Command(exe, "--print-default-config").Output()
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "cervterm.lua"), cfg, 0o644); err != nil {
			return err
		}
		for _, file := range []string{"README.md", "CHANGELOG.md", "SUPPORT.md"} {
			if err := copyFile(file, filepath.Join(pkgDir, file)); err != nil {
				return err
			}
		}
		for _, dir := range []string{"docs", "packaging"} {
			if err := copyDir(dir, filepath.Join(pkgDir, dir)); err != nil {
				return err
			}
		}
		if err := copyFontSources(outDir, filepath.Join(pkgDir, "font-sources")); err != nil {
			return err
		}
	} else if _, err := os.Stat(pkgDir); err != nil {
		return fmt.Errorf("reuse requested but package directory is missing: %s", pkgDir)
	}
	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := zipDirContents(pkgDir, zipPath); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", zipPath)
	return nil
}

func copyFontSources(outDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	cacheDir := filepath.Join(outDir, "font-sources")
	cachePath := filepath.Join(cacheDir, "NotoColorEmoji.ttf")
	candidates := []string{cachePath, filepath.Join("font-sources", "NotoColorEmoji.ttf")}
	var source string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			source = candidate
			break
		}
	}
	if source == "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return err
		}
		url := "https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoColorEmoji.ttf"
		fmt.Printf("Downloading Noto Color Emoji from %s\n", url)
		if err := downloadFile(url, cachePath); err != nil {
			return err
		}
		source = cachePath
	}
	if err := copyFile(source, filepath.Join(dstDir, "NotoColorEmoji.ttf")); err != nil {
		return err
	}
	return copyFile(filepath.Join("internal", "fontglyph", "testdata", "NotoEmoji-LICENSE.txt"), filepath.Join(dstDir, "NotoEmoji-LICENSE.txt"))
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s failed: %s", url, resp.Status)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
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

func zipDirContents(srcDir, zipPath string) error {
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
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(writer, in)
		return err
	})
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
