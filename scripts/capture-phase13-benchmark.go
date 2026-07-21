//go:build ignore

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const phase13MetadataPrefix = "# phase13-meta "

type phase13Metadata struct {
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
	MeasuredSourceSHA256 string   `json:"measured_source_sha256,omitempty"`
	WorkingTreeDirty     bool     `json:"working_tree_dirty"`
	WarmCommand          []string `json:"warm_command"`
	RecordCommand        []string `json:"record_command"`
}

type benchmarkSuite struct {
	packages  []string
	pattern   string
	buildTags string
	harness   []string
	measured  []string
}

func main() {
	suiteName := flag.String("suite", "", "benchmark suite: text, glfw, control, or store")
	outPath := flag.String("out", "", "raw benchmark output path")
	flag.Parse()
	if *outPath == "" || (*suiteName != "text" && *suiteName != "glfw" && *suiteName != "control" && *suiteName != "store") {
		fatalf("usage: go run ./scripts/capture-phase13-benchmark.go -suite text|glfw|control|store -out PATH")
	}
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	suite := suiteFor(*suiteName)
	warmArgs := benchmarkArgs(suite, "5s", "1")
	if _, err := run(root, "go", warmArgs...); err != nil {
		fatalf("warm benchmark: %v", err)
	}
	recordArgs := benchmarkArgs(suite, "2s", "10")
	output, err := run(root, "go", recordArgs...)
	if err != nil {
		fatalf("record benchmark: %v", err)
	}
	cpu, err := benchmarkCPU(output)
	if err != nil {
		fatalf("benchmark metadata: %v", err)
	}
	metadata, err := buildMetadata(root, *suiteName, suite, cpu, warmArgs, recordArgs)
	if err != nil {
		fatalf("build metadata: %v", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		fatalf("encode metadata: %v", err)
	}
	absoluteOut := *outPath
	if !filepath.IsAbs(absoluteOut) {
		absoluteOut = filepath.Join(root, absoluteOut)
	}
	if err := os.MkdirAll(filepath.Dir(absoluteOut), 0o755); err != nil {
		fatalf("create output directory: %v", err)
	}
	content := append([]byte(phase13MetadataPrefix), encoded...)
	content = append(content, '\n')
	content = append(content, normalizeBenchmarkOutput(output)...)
	if err := os.WriteFile(absoluteOut, content, 0o644); err != nil {
		fatalf("write %s: %v", absoluteOut, err)
	}
	fmt.Printf("wrote %s (%s suite, harness %s)\n", absoluteOut, *suiteName, metadata.HarnessSHA256)
}

func suiteFor(name string) benchmarkSuite {
	switch name {
	case "text":
		return benchmarkSuite{
			packages: []string{"./internal/vt", "./internal/core", "./internal/render"},
			pattern:  "BenchmarkPhase13(TextOnly|Disabled)",
			harness: []string{
				"internal/core/attr_test.go",
				"internal/vt/parser_benchmark_test.go",
				"internal/render/snapshot_test.go",
			},
		}
	case "glfw":
		return benchmarkSuite{
			packages:  []string{"./internal/frontend/glfwgl"},
			pattern:   "^BenchmarkPhase13DisabledDraw$",
			buildTags: "glfw",
			harness:   []string{"internal/frontend/glfwgl/phase13_benchmark_test.go"},
		}
	case "control":
		return benchmarkSuite{
			packages: []string{"./internal/vt"},
			pattern:  "^BenchmarkPhase13ControlString(Discard|Overflow)$",
			harness: []string{
				"internal/vt/parser_control_string_benchmark_test.go",
			},
			measured: []string{
				"internal/vt/parser.go",
				"internal/vt/parser_esc.go",
				"internal/vt/parser_control_string.go",
			},
		}
	case "store":
		return benchmarkSuite{
			packages: []string{"./internal/termimage"},
			pattern:  "^Benchmark(ProcessBudget|Store)",
			harness: []string{
				"internal/termimage/store_benchmark_test.go",
			},
			measured: []string{
				"internal/termimage/types.go",
				"internal/termimage/limits.go",
				"internal/termimage/budget.go",
				"internal/termimage/store.go",
			},
		}
	default:
		panic("validated suite")
	}
}

func benchmarkArgs(suite benchmarkSuite, benchTime, count string) []string {
	args := []string{"test"}
	if suite.buildTags != "" {
		args = append(args, "-tags", suite.buildTags)
	}
	args = append(args, suite.packages...)
	return append(args, "-run", "^$", "-bench", suite.pattern, "-benchmem", "-benchtime", benchTime, "-cpu", "1", "-count", count)
}

func buildMetadata(root, suiteName string, suite benchmarkSuite, cpu string, warmArgs, recordArgs []string) (phase13Metadata, error) {
	goVersionOutput, err := run(root, "go", "version")
	if err != nil {
		return phase13Metadata{}, err
	}
	commitOutput, err := run(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return phase13Metadata{}, err
	}
	statusOutput, err := run(root, "git", "status", "--porcelain")
	if err != nil {
		return phase13Metadata{}, err
	}
	// The harness identity covers benchmark code and its local helpers. Capture and
	// comparison tooling may grow new suites without invalidating prior results.
	digest, err := digestFiles(root, append([]string(nil), suite.harness...))
	if err != nil {
		return phase13Metadata{}, err
	}
	var measuredDigest string
	if len(suite.measured) != 0 {
		measuredDigest, err = digestFiles(root, append([]string(nil), suite.measured...))
		if err != nil {
			return phase13Metadata{}, err
		}
	}
	return phase13Metadata{
		Version:              1,
		Suite:                suiteName,
		GoVersion:            strings.TrimSpace(string(goVersionOutput)),
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		CPU:                  cpu,
		GOMAXPROCS:           1,
		BenchTime:            "2s",
		Samples:              10,
		ProductionCommit:     strings.TrimSpace(string(commitOutput)),
		HarnessSHA256:        digest,
		MeasuredSourceSHA256: measuredDigest,
		WorkingTreeDirty:     len(bytes.TrimSpace(statusOutput)) != 0,
		WarmCommand:          append([]string{"go"}, warmArgs...),
		RecordCommand:        append([]string{"go"}, recordArgs...),
	}, nil
}

func digestFiles(root string, paths []string) (string, error) {
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		// Git may materialize the same source as LF or CRLF. Harness identity is
		// content-based rather than checkout-platform-based.
		content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		hash.Write([]byte(filepath.ToSlash(path)))
		hash.Write([]byte{0})
		hash.Write(content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func benchmarkCPU(output []byte) (string, error) {
	var found string
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "cpu:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "cpu:"))
		if value == "" {
			return "", fmt.Errorf("empty cpu line")
		}
		if found != "" && found != value {
			return "", fmt.Errorf("mixed CPUs %q and %q", found, value)
		}
		found = value
	}
	if found == "" {
		return "", fmt.Errorf("go benchmark output contains no cpu line")
	}
	return found, nil
}

func normalizeBenchmarkOutput(output []byte) []byte {
	lines := strings.Split(string(output), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return []byte(strings.Join(lines, "\n"))
}

func run(dir, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, output)
	}
	return output, nil
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
	fmt.Fprintf(os.Stderr, "capture-phase13-benchmark: "+format+"\n", args...)
	os.Exit(1)
}
