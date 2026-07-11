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
	vttestExe := flag.String("vttest", "", "vttest executable path")
	output := flag.String("output", filepath.Join("internal", "vt", "testdata", "vttest-manual.vt"), "capture output path")
	rows := flag.Int("rows", 24, "PTY rows")
	cols := flag.Int("cols", 80, "PTY columns")
	cervtermExe := flag.String("cervterm", filepath.Join("dist", "cervterm-capture.exe"), "cervterm executable path")
	msys2Bash := flag.String("msys2-bash", `C:\msys64\usr\bin\bash.exe`, "MSYS2 bash path")
	timeout := flag.String("timeout", "30s", "capture timeout")
	flag.Parse()
	if *vttestExe == "" {
		fmt.Fprintln(os.Stderr, "-vttest is required")
		os.Exit(2)
	}
	if err := captureVttest(*vttestExe, *output, *rows, *cols, *cervtermExe, *msys2Bash, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func captureVttest(vttestExe, output string, rows, cols int, cervtermExe, msys2Bash, timeout string) error {
	if _, err := os.Stat(vttestExe); err != nil {
		return fmt.Errorf("vttest executable not found: %s", vttestExe)
	}
	if _, err := os.Stat(cervtermExe); err != nil {
		cmd := exec.Command("go", "build", "-o", cervtermExe, "./cmd/cervterm")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	args := []string{"--capture-vt", output, "--capture-rows", fmt.Sprint(rows), "--capture-cols", fmt.Sprint(cols), "--capture-timeout", timeout}
	if _, err := os.Stat(msys2Bash); err == nil {
		resolved, err := filepath.Abs(vttestExe)
		if err != nil {
			return err
		}
		args = append(args, "--capture-program", msys2Bash, "--capture-arg", "-lc", "--capture-arg", "TERM=xterm '"+toMsysPath(resolved)+"'")
	} else {
		args = append(args, "--capture-program", vttestExe)
	}
	cmd := exec.Command(cervtermExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("capture-vttest warning: capture exited with %v; verify output before using it\n", err)
	}
	fmt.Printf("Captured %s\n", output)
	return nil
}

func toMsysPath(path string) string {
	s := filepath.ToSlash(path)
	if len(s) >= 3 && s[1] == ':' && s[2] == '/' {
		return "/" + strings.ToLower(s[:1]) + "/" + s[3:]
	}
	return s
}
