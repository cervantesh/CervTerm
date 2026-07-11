//go:build ignore

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
)

func main() {
	zipPath := flag.String("zip", "", "package zip path")
	workDir := flag.String("workdir", filepath.Join("dist", "installed-package-smoke"), "extract and artifact directory")
	expectedVersion := flag.String("expected-version", "", "expected cervterm --version output")
	flag.Parse()
	if strings.TrimSpace(*zipPath) == "" {
		fmt.Fprintln(os.Stderr, "-zip is required")
		os.Exit(2)
	}
	must(smokeInstalledPackage(*zipPath, *workDir, *expectedVersion))
}

func smokeInstalledPackage(zipPath, workDir, expectedVersion string) error {
	zipAbs, err := filepath.Abs(zipPath)
	if err != nil {
		return err
	}
	if err := prepareCleanWorkDir(workDir); err != nil {
		return err
	}
	if err := unzip(zipAbs, workDir); err != nil {
		return err
	}
	exe := filepath.Join(workDir, "cervterm.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("package missing cervterm.exe: %w", err)
	}
	for _, required := range []string{filepath.Join("font-sources", "NotoColorEmoji.ttf"), filepath.Join("font-sources", "NotoEmoji-LICENSE.txt")} {
		if _, err := os.Stat(filepath.Join(workDir, required)); err != nil {
			return fmt.Errorf("package missing %s: %w", filepath.ToSlash(required), err)
		}
	}
	version, err := runOutput(exe, "--version")
	if err != nil {
		return err
	}
	version = strings.TrimSpace(version)
	if expectedVersion != "" && version != expectedVersion {
		return fmt.Errorf("version mismatch: got %q, want %q", version, expectedVersion)
	}
	fmt.Printf("version: %s\n", version)

	buildInfo, err := runOutput(exe, "--build-info")
	if err != nil {
		return err
	}
	if !strings.Contains(buildInfo, "CervTerm") {
		return fmt.Errorf("unexpected build info: %s", strings.TrimSpace(buildInfo))
	}
	fmt.Printf("build-info: %s\n", strings.TrimSpace(buildInfo))

	doctor, err := runOutput(exe, "--doctor")
	if err != nil {
		return err
	}
	for _, required := range []string{"CervTerm doctor", "diagnostics:", "config:"} {
		if !strings.Contains(doctor, required) {
			return fmt.Errorf("doctor output missing %q", required)
		}
	}
	fmt.Println("doctor: ok")

	config, err := runOutput(exe, "--print-default-config")
	if err != nil {
		return err
	}
	if !strings.Contains(config, "return {") {
		return fmt.Errorf("default config did not look like Lua")
	}
	if err := os.WriteFile(filepath.Join(workDir, "generated-default.lua"), []byte(config), 0o644); err != nil {
		return err
	}

	vtPath := filepath.Join(workDir, "smoke.vt")
	logPath := filepath.Join(workDir, "smoke.log")
	cmd := exec.Command(exe, "--capture-vt", vtPath, "--capture-program", "cmd.exe", "--capture-arg", "/C", "--capture-arg", "echo cervterm-smoke", "--capture-timeout", "10s", "--log-file", logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("installed package smoke warning: capture exited with %v; validating artifacts\n", err)
	}
	data, err := os.ReadFile(vtPath)
	if err != nil {
		return fmt.Errorf("capture-vt did not create %s: %w", vtPath, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("capture-vt output is empty")
	}
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("smoke log was not created: %w", err)
	}
	logText, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}
	if bytes.Contains(logText, []byte("emoji coverage warning")) {
		return fmt.Errorf("unexpected emoji coverage warning in installed package smoke log")
	}
	fmt.Printf("installed package smoke passed: %s\n", zipPath)
	return nil
}

func prepareCleanWorkDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("workdir must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root := filepath.VolumeName(abs) + string(os.PathSeparator)
	if abs == root {
		return fmt.Errorf("refusing to remove filesystem root: %s", abs)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return err
	}
	if containsPath(abs, cwd) {
		return fmt.Errorf("refusing to remove repository root or parent: %s", abs)
	}
	if err := os.RemoveAll(abs); err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func containsPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		clean := filepath.Clean(f.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe zip entry: %s", f.Name)
		}
		target := filepath.Join(dst, clean)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
