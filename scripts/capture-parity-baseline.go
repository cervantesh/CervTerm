//go:build ignore

// capture-parity-baseline records reproducible correctness and hot-path evidence
// before a parity roadmap phase changes behavior or performance.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type command struct {
	name string
	args []string
}

func main() {
	out := flag.String("out", filepath.Join("dist", "parity-baseline.txt"), "report output path")
	full := flag.Bool("full", false, "also run the full test, vet, and maturity gates")
	count := flag.Int("count", 3, "benchmark repetition count")
	flag.Parse()

	if *count < 1 {
		fatalf("count must be at least 1")
	}

	root, err := repositoryRoot()
	if err != nil {
		fatalf("locate repository: %v", err)
	}

	commands := []command{
		{name: "VT parser and reuse benchmarks", args: []string{"test", "./internal/vt", "-run", "^$", "-bench", "Benchmark(ParserThroughput|CoreReuseVsNew)$", "-benchmem", "-count", fmt.Sprint(*count)}},
		{name: "render snapshot benchmark", args: []string{"test", "./internal/render", "-run", "^$", "-bench", "BenchmarkCaptureReuse$", "-benchmem", "-count", fmt.Sprint(*count)}},
	}
	if *full {
		commands = append(commands,
			command{name: "full tests", args: []string{"test", "./...", "-count=1"}},
			command{name: "vet", args: []string{"vet", "-unsafeptr=false", "./..."}},
			command{name: "maturity gates", args: []string{"run", "./scripts/check-maturity-gates.go"}},
		)
	}

	var report bytes.Buffer
	fmt.Fprintf(&report, "# CervTerm parity baseline\n\n")
	fmt.Fprintf(&report, "captured_utc: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&report, "host: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&report, "go: %s\n", runtime.Version())
	fmt.Fprintf(&report, "commit: %s\n", gitValue(root, "rev-parse", "HEAD"))
	fmt.Fprintf(&report, "dirty: %t\n", strings.TrimSpace(gitValue(root, "status", "--short")) != "")

	for _, c := range commands {
		fmt.Printf("running %s...\n", c.name)
		fmt.Fprintf(&report, "\n## %s\n\n$ go %s\n", c.name, strings.Join(c.args, " "))
		cmd := exec.Command("go", c.args...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		report.Write(output)
		if len(output) > 0 && output[len(output)-1] != '\n' {
			report.WriteByte('\n')
		}
		if err != nil {
			_ = writeReport(root, *out, report.Bytes())
			fatalf("%s failed: %v", c.name, err)
		}
	}

	if err := writeReport(root, *out, report.Bytes()); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("parity baseline written: %s\n", filepath.Join(root, *out))
}

func repositoryRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitValue(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func writeReport(root, path string, data []byte) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "capture parity baseline: "+format+"\n", args...)
	os.Exit(1)
}
