//go:build ignore

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const metadataPrefix = "# phase13-meta "

type metadata struct {
	Version              int      `json:"version"`
	Suite                string   `json:"suite"`
	GoVersion            string   `json:"go_version"`
	GOOS                 string   `json:"goos"`
	GOARCH               string   `json:"goarch"`
	CPU                  string   `json:"cpu"`
	GOMAXPROCS           int      `json:"gomaxprocs"`
	BenchTime            string   `json:"benchtime"`
	Samples              int      `json:"samples"`
	ProductionCommit     string   `json:"production_commit"`
	HarnessSHA256        string   `json:"harness_sha256"`
	MeasuredSourceSHA256 string   `json:"measured_source_sha256"`
	WorkingTreeDirty     bool     `json:"working_tree_dirty"`
	WarmCommand          []string `json:"warm_command"`
	RecordCommand        []string `json:"record_command"`
}

func main() {
	if len(os.Args) != 2 {
		fatalf("usage: go run ./scripts/check-phase14-frame-baseline.go PATH")
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		fatalf("open: %v", err)
	}
	defer file.Close()
	var meta metadata
	var haveMeta bool
	var goos, goarch, pkg, cpu string
	var samples []float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, metadataPrefix):
			if haveMeta || json.Unmarshal([]byte(strings.TrimPrefix(line, metadataPrefix)), &meta) != nil {
				fatalf("invalid metadata")
			}
			haveMeta = true
		case strings.HasPrefix(line, "goos:"):
			observe(&goos, strings.TrimSpace(strings.TrimPrefix(line, "goos:")), "goos")
		case strings.HasPrefix(line, "goarch:"):
			observe(&goarch, strings.TrimSpace(strings.TrimPrefix(line, "goarch:")), "goarch")
		case strings.HasPrefix(line, "pkg:"):
			observe(&pkg, strings.TrimSpace(strings.TrimPrefix(line, "pkg:")), "pkg")
		case strings.HasPrefix(line, "cpu:"):
			observe(&cpu, strings.TrimSpace(strings.TrimPrefix(line, "cpu:")), "cpu")
		case strings.HasPrefix(line, "Benchmark"):
			fields := strings.Fields(line)
			if len(fields) < 8 {
				fatalf("malformed benchmark row")
			}
			if fields[0] != "BenchmarkPhase13DisabledFrame" {
				fatalf("unexpected benchmark %q", fields[0])
			}
			iterations, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil || iterations == 0 {
				fatalf("invalid iteration count %q", fields[1])
			}
			ns, bytes, allocs := metric(fields, "ns/op"), metric(fields, "B/op"), metric(fields, "allocs/op")
			if math.IsNaN(ns) || ns < 0 || bytes != 0 || allocs != 0 {
				fatalf("invalid sample ns=%v bytes=%v allocs=%v", ns, bytes, allocs)
			}
			samples = append(samples, ns)
		}
	}
	if err := scanner.Err(); err != nil {
		fatalf("scan: %v", err)
	}
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	harnessDigest, err := digestFiles(root, []string{"internal/frontend/glfwgl/phase13_frame_benchmark_test.go"})
	if err != nil {
		fatalf("harness digest: %v", err)
	}
	measuredDigest, err := digestFiles(root, []string{"internal/frontend/glfwgl/terminal_image_draw.go"})
	if err != nil {
		fatalf("measured digest: %v", err)
	}
	warm := []string{"go", "test", "-tags", "glfw", "./internal/frontend/glfwgl", "-run", "^$", "-bench", "^BenchmarkPhase13DisabledFrame$", "-benchmem", "-benchtime", "5s", "-cpu", "1", "-count", "1"}
	record := []string{"go", "test", "-tags", "glfw", "./internal/frontend/glfwgl", "-run", "^$", "-bench", "^BenchmarkPhase13DisabledFrame$", "-benchmem", "-benchtime", "2s", "-cpu", "1", "-count", "10"}
	validMeta := haveMeta && meta.Version == 1 && meta.Suite == "glfwframe" && meta.GoVersion != "" &&
		meta.GOOS != "" && meta.GOARCH != "" && meta.CPU != "" && meta.GOMAXPROCS == 1 &&
		meta.BenchTime == "2s" && meta.Samples == 10 && meta.ProductionCommit != "" &&
		meta.HarnessSHA256 == harnessDigest && meta.MeasuredSourceSHA256 == measuredDigest && !meta.WorkingTreeDirty &&
		slices.Equal(meta.WarmCommand, warm) && slices.Equal(meta.RecordCommand, record)
	if !validMeta || goos != meta.GOOS || goarch != meta.GOARCH || cpu != meta.CPU ||
		pkg != "cervterm/internal/frontend/glfwgl" || len(samples) != 10 {
		fatalf("invalid capture metadata=%+v identity=%q/%q/%q/%q rows=%d", meta, goos, goarch, pkg, cpu, len(samples))
	}
	sort.Float64s(samples)
	median := (samples[4] + samples[5]) / 2
	if median > 8.0 {
		fatalf("median %.3f ns/op exceeds 8.0 ns/op", median)
	}
	fmt.Printf("phase 14 disabled frame gate passed: median %.3f ns/op, 0 B/op, 0 allocs/op\n", median)
}

func metric(fields []string, unit string) float64 {
	for index := 2; index+1 < len(fields); index += 2 {
		if fields[index+1] == unit {
			value, err := strconv.ParseFloat(fields[index], 64)
			if err != nil {
				return math.NaN()
			}
			return value
		}
	}
	return math.NaN()
}

func digestFiles(root string, paths []string) (string, error) {
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		hash.Write([]byte(filepath.ToSlash(path)))
		hash.Write([]byte{0})
		hash.Write(content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

func observe(destination *string, value, label string) {
	if value == "" || (*destination != "" && *destination != value) {
		fatalf("invalid %s %q", label, value)
	}
	*destination = value
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check-phase14-frame-baseline: "+format+"\n", args...)
	os.Exit(1)
}
