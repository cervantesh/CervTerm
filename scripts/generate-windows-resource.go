//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	arch := flag.String("arch", "amd64", "target architecture")
	tool := flag.String("tool", "goversioninfo", "goversioninfo executable")
	flag.Parse()
	if err := generateWindowsResource(*arch, *tool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateWindowsResource(arch, tool string) error {
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Errorf("%s not found. Install with: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest", tool)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	src := filepath.Join(root, "packaging", "windows")
	outName := "resource_windows_" + arch + ".syso"
	outPath := filepath.Join(root, "cmd", "cervterm", outName)
	tmp := filepath.Join(root, "dist", "resource-"+arch)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"versioninfo.json", "cervterm.ico", "cervterm.manifest"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(tmp, name)); err != nil {
			return err
		}
	}
	args := []string{"-platform-specific"}
	if strings.Contains(arch, "64") || arch == "amd64" || arch == "arm64" {
		args = append(args, "-64")
	}
	cmd := exec.Command(tool, args...)
	cmd.Dir = tmp
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	generated := filepath.Join(tmp, outName)
	if _, err := os.Stat(generated); err != nil {
		return fmt.Errorf("goversioninfo did not produce %s", outName)
	}
	if err := copyFile(generated, outPath); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", outPath)
	return nil
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
	_, err = out.ReadFrom(in)
	return err
}
