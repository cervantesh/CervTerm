//go:build ignore

// daily-driver-smoke runs CI-safe end-to-end smoke captures for common
// terminal workflows. It intentionally uses Go as the harness so Windows CI
// does not depend on cmd.exe script orchestration.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type smokeCase struct {
	Name    string
	Program string
	Args    []string
	Markers []string
	Rows    int
	Cols    int
	Timeout string
}

func main() {
	var cervtermExe string
	var workDir string
	var version string
	flag.StringVar(&cervtermExe, "cervterm", "", "path to cervterm.exe; builds one under -workdir when empty")
	flag.StringVar(&workDir, "workdir", filepath.Join("dist", "daily-driver-smoke"), "artifact directory")
	flag.StringVar(&version, "version", "daily-smoke", "version embedded when building a temporary cervterm.exe")
	flag.Parse()

	if runtime.GOOS != "windows" {
		fmt.Printf("daily-driver smoke skipped on %s; Windows ConPTY coverage runs on windows-latest\n", runtime.GOOS)
		return
	}

	must(os.RemoveAll(workDir))
	must(os.MkdirAll(workDir, 0o755))

	resolved, err := resolveCervTerm(cervtermExe, workDir, version)
	must(err)
	fmt.Printf("daily-driver: using %s\n", resolved)

	helpers, err := buildHelper(workDir)
	must(err)

	pagerInput := filepath.Join(workDir, "pager-input.txt")
	must(os.WriteFile(pagerInput, []byte(makePagerInput()), 0o644))
	pagerInput, _ = filepath.Abs(pagerInput)

	gitExe, gitHead, err := resolveGitHead()
	must(err)

	cases := []smokeCase{
		{
			Name:    "cmd-basic",
			Program: "cmd.exe",
			Args:    []string{"/C", "echo CERVTERM_CMD_START && ver && echo CERVTERM_CMD_END"},
			Markers: []string{"CERVTERM_CMD_START", "CERVTERM_CMD_END"},
		},
		{
			Name:    "git-log",
			Program: gitExe,
			Args:    []string{"--no-pager", "log", "--oneline", "-n", "3"},
			Markers: []string{gitHead},
		},
		{
			Name:    "pager-more",
			Program: filepath.Join(os.Getenv("SystemRoot"), "System32", "more.com"),
			Args:    []string{pagerInput},
			Markers: []string{"CERVTERM_PAGER_START", "pager-line-020", "-- More"},
			Timeout: "8s",
		},
		{
			Name:    "alternate-screen",
			Program: helpers,
			Args:    []string{"alternate"},
			Markers: []string{"CERVTERM_ALT_START", "inside-alt-screen", "CERVTERM_ALT_END"},
		},
		{
			Name:    "resize-reflow-40col",
			Program: helpers,
			Args:    []string{"reflow"},
			Markers: []string{"CERVTERM_REFLOW_START", "CERVTERM_REFLOW_END"},
			Cols:    40,
		},
		{
			Name:    "resize-reflow-100col",
			Program: helpers,
			Args:    []string{"reflow"},
			Markers: []string{"CERVTERM_REFLOW_START", "CERVTERM_REFLOW_END"},
			Cols:    100,
		},
		{
			Name:    "long-session",
			Program: helpers,
			Args:    []string{"long"},
			Markers: []string{"CERVTERM_LONG_START", "long-session-line-20", "CERVTERM_LONG_END"},
			Timeout: "10s",
		},
	}

	for _, c := range cases {
		must(runCase(resolved, workDir, c))
	}
	fmt.Printf("daily-driver smoke passed: %s\n", workDir)
}

func resolveCervTerm(path, workDir, version string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return filepath.Abs(path)
	}
	out := filepath.Join(workDir, "cervterm.exe")
	cmd := exec.Command("go", "build", "-tags", "glfw", "-ldflags", "-X cervterm/internal/buildinfo.Version="+version, "-o", out, "./cmd/cervterm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return filepath.Abs(out)
}

func buildHelper(workDir string) (string, error) {
	src, err := os.CreateTemp("", "daily-smoke-helper-*.go")
	if err != nil {
		return "", err
	}
	defer os.Remove(src.Name())
	if _, err := src.WriteString(helperSource); err != nil {
		_ = src.Close()
		return "", err
	}
	if err := src.Close(); err != nil {
		return "", err
	}
	exe := filepath.Join(workDir, "daily-smoke-helper.exe")
	cmd := exec.Command("go", "build", "-o", exe, src.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

func resolveGitHead() (string, string, error) {
	gitExe, err := exec.LookPath("git.exe")
	if err != nil {
		gitExe, err = exec.LookPath("git")
		if err != nil {
			return "", "", errors.New("git executable is required for git-log smoke")
		}
	}
	if strings.Contains(strings.ToLower(filepath.ToSlash(gitExe)), "/mingw64/bin/git.exe") {
		root := filepath.Dir(filepath.Dir(filepath.Dir(gitExe)))
		wrapper := filepath.Join(root, "cmd", "git.exe")
		if _, err := os.Stat(wrapper); err == nil {
			gitExe = wrapper
		}
	}
	out, err := exec.Command(gitExe, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", "", err
	}
	head := strings.TrimSpace(string(out))
	if head == "" {
		return "", "", errors.New("git HEAD marker is empty")
	}
	return gitExe, head, nil
}

func runCase(cervtermExe, workDir string, c smokeCase) error {
	if c.Rows == 0 {
		c.Rows = 24
	}
	if c.Cols == 0 {
		c.Cols = 80
	}
	if c.Timeout == "" {
		c.Timeout = "15s"
	}
	vtPath := filepath.Join(workDir, c.Name+".vt")
	logPath := filepath.Join(workDir, c.Name+".log")
	_ = os.Remove(vtPath)
	_ = os.Remove(logPath)

	args := []string{
		"--capture-vt", vtPath,
		"--capture-program", c.Program,
		"--capture-timeout", c.Timeout,
		"--capture-rows", fmt.Sprint(c.Rows),
		"--capture-cols", fmt.Sprint(c.Cols),
		"--log-file", logPath,
	}
	for _, arg := range c.Args {
		args = append(args, "--capture-arg", arg)
	}
	fmt.Printf("daily-driver: %s (%s %s)\n", c.Name, c.Program, strings.Join(c.Args, " "))
	cmd := exec.Command(cervtermExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("daily-driver warning: %s exited with %v; validating captured markers before failing\n", c.Name, err)
	}
	data, err := os.ReadFile(vtPath)
	if err != nil {
		return fmt.Errorf("%s did not create capture %s: %w", c.Name, vtPath, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("%s capture is empty: %s", c.Name, vtPath)
	}
	for _, marker := range c.Markers {
		if !bytes.Contains(data, []byte(marker)) {
			return fmt.Errorf("%s capture missing marker %q in %s", c.Name, marker, vtPath)
		}
	}
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("%s did not create diagnostics log %s: %w", c.Name, logPath, err)
	}
	fmt.Printf("daily-driver: %s ok -> %s\n", c.Name, vtPath)
	return nil
}

func makePagerInput() string {
	var b strings.Builder
	b.WriteString("CERVTERM_PAGER_START\r\n")
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&b, "pager-line-%03d abcdefghijklmnopqrstuvwxyz\r\n", i)
	}
	b.WriteString("CERVTERM_PAGER_END\r\n")
	return b.String()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const helperSource = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("daily-smoke-helper requires a mode")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "alternate":
		esc := string(rune(27))
		fmt.Println("CERVTERM_ALT_START")
		fmt.Print(esc + "[?1049hALT_SCREEN_BODY" + esc + "[2J" + esc + "[Hinside-alt-screen" + esc + "[?1049l")
		fmt.Println("CERVTERM_ALT_END")
	case "reflow":
		parts := make([]string, 0, 12)
		for i := 1; i <= 12; i++ {
			parts = append(parts, fmt.Sprintf("segment%d", i))
		}
		fmt.Printf("CERVTERM_REFLOW_START %s CERVTERM_REFLOW_END\n", strings.Join(parts, "-"))
	case "long":
		fmt.Println("CERVTERM_LONG_START")
		for i := 1; i <= 20; i++ {
			fmt.Printf("long-session-line-%02d\n", i)
			time.Sleep(75 * time.Millisecond)
		}
		fmt.Println("CERVTERM_LONG_END")
	default:
		fmt.Println("unknown daily-smoke-helper mode")
		os.Exit(2)
	}
}
`
