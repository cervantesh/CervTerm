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

var suites = []string{"text", "control", "store", "glfw", "glfwframe"}

func main() {
	prefix := flag.String("prefix", "docs/validation/phase-14", "repository-relative output prefix")
	flag.Parse()
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	if filepath.IsAbs(*prefix) || strings.TrimSpace(*prefix) == "" {
		fatalf("prefix must be a non-empty repository-relative path")
	}
	normalizedPrefix := filepath.Clean(filepath.FromSlash(*prefix))
	absolutePrefix := filepath.Join(root, normalizedPrefix)
	relativePrefix, err := filepath.Rel(root, absolutePrefix)
	if err != nil || relativePrefix == "." || relativePrefix == ".." || strings.HasPrefix(relativePrefix, ".."+string(filepath.Separator)) {
		fatalf("prefix must stay inside the repository")
	}
	tempDir, err := os.MkdirTemp("", "cervterm-phase14-baseline-")
	if err != nil {
		fatalf("temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	type result struct{ source, destination string }
	results := make([]result, 0, len(suites))
	for _, suite := range suites {
		source := filepath.Join(tempDir, suite+".txt")
		command := exec.Command("go", "run", "./scripts/capture-phase13-benchmark.go", "-suite", suite, "-out", source)
		command.Dir = root
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			fatalf("capture %s: %v", suite, err)
		}
		results = append(results, result{
			source:      source,
			destination: filepath.Join(root, relativePrefix+"-"+suite+"-baseline.txt"),
		})
	}
	for _, item := range results {
		content, err := os.ReadFile(item.source)
		if err != nil {
			fatalf("read staged %s: %v", item.source, err)
		}
		if err := os.MkdirAll(filepath.Dir(item.destination), 0o755); err != nil {
			fatalf("create output directory: %v", err)
		}
		staged := item.destination + ".tmp"
		if err := os.WriteFile(staged, content, 0o644); err != nil {
			fatalf("stage %s: %v", item.destination, err)
		}
		if err := os.Rename(staged, item.destination); err != nil {
			_ = os.Remove(staged)
			fatalf("publish %s: %v", item.destination, err)
		}
		fmt.Printf("published %s\n", item.destination)
	}
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "capture-phase14-baselines: "+format+"\n", args...)
	os.Exit(1)
}
