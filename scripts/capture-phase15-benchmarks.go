//go:build ignore

// capture-phase15-benchmarks captures the inherited ten-sample comparables and
// candidate-only integration budgets without rewriting any historical baseline.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	phase13MetadataPrefix   = "# phase13-meta "
	phase0BaselineCommit    = "7d64cc9"
	phase14ProductionCommit = "b1eec1335aaad55cffbec9b15c610e3aebbc24dd"
	phase14EvidenceCommit   = "353cdcfe5260c11d9eee87c6a96663e5291e8125"
	phase15HarnessCommit    = "0673a5aa2d127080087a38446c3a6a16664562a9"
)

var inheritedHarnessSHA256 = map[string]string{
	"internal/core/attr_test.go":                               "634ddf5ce6a037a3988c1f868eeda0c87f5d89b760bac54cd4521626460dfa88",
	"internal/vt/parser_benchmark_test.go":                     "bbd86345783ec6834154024d2d4edba6d816113633f096f3d391a8dd84b0d858",
	"internal/render/snapshot_test.go":                         "fcd957628d2651797b5ea3c080cff087618fb150493d696f4f7691341229fd99",
	"internal/frontend/glfwgl/phase13_benchmark_test.go":       "c378c7689a80fd71f680a7fd083c4f314ec1cd1844f7de0615cb95f35d05addd",
	"internal/frontend/glfwgl/phase13_frame_benchmark_test.go": "d9fa696e322370d96caab2bcab1a9cbd61bb453243f40a998a795e790494e050",
	"internal/vt/parser_control_string_benchmark_test.go":      "d830b72a97e62038685992ee9a22c8ea472959efc1dd70d91982540161082487",
	"internal/termimage/store_benchmark_test.go":               "87f7f52c135789755d947e8a3db10b41c3799eb202b060ad5b26e0dd3559e0e9",
}

var phase0HarnessSHA256 = map[string]string{
	"internal/vt/parser_benchmark_test.go": inheritedHarnessSHA256["internal/vt/parser_benchmark_test.go"],
	"internal/render/snapshot_test.go":     inheritedHarnessSHA256["internal/render/snapshot_test.go"],
}

type inheritedMetadata struct {
	Version          int    `json:"version"`
	Suite            string `json:"suite"`
	GoVersion        string `json:"go_version"`
	GOOS             string `json:"goos"`
	GOARCH           string `json:"goarch"`
	CPU              string `json:"cpu"`
	GOMAXPROCS       int    `json:"gomaxprocs"`
	BenchTime        string `json:"benchtime"`
	Samples          int    `json:"samples"`
	ProductionCommit string `json:"production_commit"`
	HarnessSHA256    string `json:"harness_sha256"`
	WorkingTreeDirty bool   `json:"working_tree_dirty"`
}

type benchmarkSample struct {
	Nanoseconds float64
	Bytes       float64
	Allocations float64
}

type benchmarkSummary struct {
	Name              string  `json:"name"`
	Samples           int     `json:"samples"`
	MedianNanoseconds float64 `json:"median_ns_per_op"`
	MedianBytes       float64 `json:"median_bytes_per_op"`
	MedianAllocations float64 `json:"median_allocations_per_op"`
}

type inheritedComparison struct {
	Suite                string  `json:"suite"`
	Benchmark            string  `json:"benchmark"`
	BaselineNanoseconds  float64 `json:"baseline_median_ns_per_op"`
	CandidateNanoseconds float64 `json:"candidate_median_ns_per_op"`
	DeltaPercent         float64 `json:"delta_percent"`
	BaselineBytes        float64 `json:"baseline_median_bytes_per_op"`
	CandidateBytes       float64 `json:"candidate_median_bytes_per_op"`
	BaselineAllocations  float64 `json:"baseline_median_allocations_per_op"`
	CandidateAllocations float64 `json:"candidate_median_allocations_per_op"`
	ThresholdPercent     float64 `json:"threshold_percent"`
	Waiver               string  `json:"waiver,omitempty"`
}

type phase15PerformanceReport struct {
	SchemaVersion   int                   `json:"schema_version"`
	CapturedUTC     string                `json:"captured_utc"`
	Commit          string                `json:"commit"`
	GoVersion       string                `json:"go_version"`
	GoEnvironment   map[string]string     `json:"go_environment"`
	GOOS            string                `json:"goos"`
	GOARCH          string                `json:"goarch"`
	CPU             string                `json:"cpu"`
	Samples         int                   `json:"samples"`
	BaselineRefs    map[string]string     `json:"baseline_refs"`
	BaselineDigests map[string]string     `json:"baseline_sha256"`
	Inherited       []inheritedComparison `json:"inherited"`
	CandidateOnly   []benchmarkSummary    `json:"candidate_only"`
}

type absoluteBudget struct {
	MaxNanoseconds float64
	MaxBytes       float64
	MaxAllocations float64
}

func main() {
	outDir := flag.String("out-dir", "dist/phase-15-performance", "repository-relative output directory")
	count := flag.Int("count", 10, "sample count; inherited Phase 13/14 rows require exactly 10")
	waiver := flag.String("waiver", "", "explicit performance waiver ID; only P15-W01 is recognized when applicable")
	flag.Parse()
	if *count != 10 {
		fatalf("count must be exactly 10 for inherited Phase 13/14 comparables")
	}
	root, err := repositoryRoot()
	if err != nil {
		fatalf("repository root: %v", err)
	}
	if dirty, err := run(root, "git", "status", "--porcelain"); err != nil || len(bytes.TrimSpace(dirty)) != 0 {
		fatalf("capture requires a clean working tree")
	}
	outputRoot, err := insideRepository(root, *outDir)
	if err != nil {
		fatalf("output directory: %v", err)
	}
	outputRelative, err := filepath.Rel(root, outputRoot)
	if err != nil || outputRelative == "dist" || !strings.HasPrefix(outputRelative, "dist"+string(filepath.Separator)) {
		fatalf("output directory must be a child of dist/")
	}
	if _, err := os.Lstat(outputRoot); !os.IsNotExist(err) {
		fatalf("output directory must not already exist")
	}
	if err := os.MkdirAll(filepath.Dir(outputRoot), 0o755); err != nil {
		fatalf("create output parent: %v", err)
	}
	if err := resolvedInside(root, filepath.Dir(outputRoot)); err != nil {
		fatalf("resolved output parent: %v", err)
	}
	if err := rejectSymlinkComponents(root, filepath.Dir(outputRoot)); err != nil {
		fatalf("output parent: %v", err)
	}
	if err := os.Mkdir(outputRoot, 0o755); err != nil {
		fatalf("create exclusive output directory: %v", err)
	}
	if err := resolvedInside(root, outputRoot); err != nil {
		fatalf("resolved output directory: %v", err)
	}

	report := phase15PerformanceReport{SchemaVersion: 1, CapturedUTC: time.Now().UTC().Format(time.RFC3339), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Samples: *count, BaselineRefs: map[string]string{"phase0": phase0BaselineCommit, "phase14-production": phase14ProductionCommit, "phase14-evidence": phase14EvidenceCommit, "approved-harness": phase15HarnessCommit}, BaselineDigests: make(map[string]string), GoEnvironment: make(map[string]string)}
	report.Commit = strings.TrimSpace(string(mustRun(root, "git", "rev-parse", "HEAD")))
	report.GoVersion = strings.TrimSpace(string(mustRun(root, "go", "version")))
	for _, key := range []string{"GOAMD64", "GOEXPERIMENT", "GOFLAGS", "CGO_ENABLED"} {
		report.GoEnvironment[key] = strings.TrimSpace(string(mustRun(root, "go", "env", key)))
	}
	report.BaselineDigests["git:"+phase0BaselineCommit+"^{tree}"] = strings.TrimSpace(string(mustRun(root, "git", "rev-parse", phase0BaselineCommit+"^{tree}")))
	for relative, digest := range phase0HarnessSHA256 {
		report.BaselineDigests["phase0-harness:"+relative] = digest
	}

	phase0Comparisons, phase0CPU, err := capturePhase0(root, outputRoot, *waiver)
	if err != nil {
		fatalf("Phase 0 historical comparison: %v", err)
	}
	report.CPU = phase0CPU
	report.Inherited = append(report.Inherited, phase0Comparisons...)

	phase14Root, cleanupPhase14, err := preparePinnedHarnessWorktree(root, phase14ProductionCommit, inheritedHarnessSHA256)
	if err != nil {
		fatalf("prepare Phase 14 production baseline: %v", err)
	}
	defer cleanupPhase14()
	for relative, digest := range inheritedHarnessSHA256 {
		report.BaselineDigests["inherited-harness:"+relative] = digest
	}

	for _, suite := range []string{"text", "control", "store", "glfw", "glfwframe"} {
		baselineRelative := filepath.ToSlash("docs/validation/phase-14-" + suite + "-baseline.txt")
		_, digest, err := immutableBaseline(root, baselineRelative)
		if err != nil {
			fatalf("verify %s evidence baseline: %v", suite, err)
		}
		report.BaselineDigests[baselineRelative] = digest
		baselineOutput, candidateOutput, cpu, err := captureInterleavedSuite(phase14Root, root, suite)
		if err != nil {
			fatalf("capture inherited suite %s: %v", suite, err)
		}
		if err := os.WriteFile(filepath.Join(outputRoot, "baseline-"+suite+".txt"), baselineOutput, 0o644); err != nil {
			fatalf("write %s baseline: %v", suite, err)
		}
		if err := os.WriteFile(filepath.Join(outputRoot, "candidate-"+suite+".txt"), candidateOutput, 0o644); err != nil {
			fatalf("write %s candidate: %v", suite, err)
		}
		if err := rejectSymlinkComponents(root, outputRoot); err != nil {
			fatalf("output confinement changed: %v", err)
		}
		baselineSamples, err := parseBenchmarkSamples(baselineOutput)
		if err != nil {
			fatalf("read %s baseline: %v", suite, err)
		}
		candidateSamples, err := parseBenchmarkSamples(candidateOutput)
		if err != nil {
			fatalf("read %s candidate: %v", suite, err)
		}
		if report.CPU == "" {
			report.CPU = cpu
		}
		comparisons, err := compareInherited(suite, baselineSamples, candidateSamples, 3, 10, *waiver, false)
		if err != nil {
			fatalf("%s comparison: %v", suite, err)
		}
		report.Inherited = append(report.Inherited, comparisons...)
	}

	absoluteArgs := []string{
		"test", "./internal/input", "./internal/core", "./internal/fontdesc", "./internal/mux", "./internal/accessibility", "./internal/termimage", "./internal/workscheduler", "./internal/sixel", "./internal/itermimage",
		"-run", "^$", "-bench", `^(BenchmarkPhase15|BenchmarkAccessibility|Benchmark(ProcessBudget|Store)|Benchmark(Scheduler|Sixel|ITerm)|BenchmarkMuxAllDisabledImageIdle)`, "-benchmem", "-benchtime", "500ms", "-cpu", "1", "-count", strconv.Itoa(*count),
	}
	absoluteOutput := mustRun(root, "go", absoluteArgs...)
	if err := rejectSymlinkComponents(root, outputRoot); err != nil {
		fatalf("output confinement changed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "candidate-integration.txt"), absoluteOutput, 0o644); err != nil {
		fatalf("write candidate integration output: %v", err)
	}
	absoluteSamples, err := parseBenchmarkSamples(absoluteOutput)
	if err != nil {
		fatalf("parse candidate integration benchmarks: %v", err)
	}
	report.CandidateOnly = summarizeSamples(absoluteSamples)
	if err := enforceAbsoluteBudgets(report.CandidateOnly); err != nil {
		fatalf("candidate absolute budget: %v", err)
	}
	if err := assertCleanAtRevision(root, report.Commit); err != nil {
		fatalf("candidate changed during capture: %v", err)
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := rejectSymlinkComponents(root, outputRoot); err != nil {
		fatalf("output confinement changed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "phase-15-performance.json"), encoded, 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("Phase 15 performance capture passed: %s\n", filepath.Join(outputRoot, "phase-15-performance.json"))
}

func approvedHarnessBlob(root, relative, expectedDigest string) ([]byte, error) {
	path := filepath.ToSlash(relative)
	pinned, err := run(root, "git", "show", phase15HarnessCommit+":"+path)
	if err != nil {
		return nil, err
	}
	current, err := run(root, "git", "show", "HEAD:"+path)
	if err != nil {
		return nil, err
	}
	for label, content := range map[string][]byte{"pinned": pinned, "current": current} {
		if actual := fmt.Sprintf("%x", sha256.Sum256(content)); actual != expectedDigest {
			return nil, fmt.Errorf("%s harness %s digest is %s, expected %s", label, relative, actual, expectedDigest)
		}
	}
	if !bytes.Equal(pinned, current) {
		return nil, fmt.Errorf("current harness %s differs from pinned commit %s", relative, phase15HarnessCommit)
	}
	return pinned, nil
}

func capturePhase0(root, outputRoot, waiver string) ([]inheritedComparison, string, error) {
	parent, err := os.MkdirTemp("", "cervterm-phase15-phase0-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(parent)
	baselineRoot := filepath.Join(parent, "baseline")
	if output, err := run(root, "git", "worktree", "add", "--detach", baselineRoot, phase0BaselineCommit); err != nil {
		return nil, "", fmt.Errorf("create detached baseline worktree: %w: %s", err, output)
	}
	defer func() { _, _ = run(root, "git", "worktree", "remove", "--force", baselineRoot) }()

	for relative, expectedDigest := range phase0HarnessSHA256 {
		content, readErr := approvedHarnessBlob(root, relative, expectedDigest)
		if readErr != nil {
			return nil, "", readErr
		}
		destination := filepath.Join(baselineRoot, filepath.FromSlash(relative))
		if err := rejectSymlinkComponents(baselineRoot, filepath.Dir(destination)); err != nil {
			return nil, "", err
		}
		if writeErr := os.WriteFile(destination, content, 0o644); writeErr != nil {
			return nil, "", writeErr
		}
	}
	args := []string{"test", "./internal/vt", "./internal/render", "-run", "^$", "-bench", `^Benchmark(ParserThroughput|CoreReuseVsNew|CaptureReuse)$`, "-benchmem", "-benchtime", "2s", "-cpu", "1"}
	for _, directory := range []string{baselineRoot, root} {
		if _, err := run(directory, "go", append(args, "-count", "1")...); err != nil {
			return nil, "", fmt.Errorf("warm Phase 0 benchmark in %s: %w", directory, err)
		}
	}
	baselineOutput, err := run(baselineRoot, "go", append(args, "-count", "3")...)
	if err != nil {
		return nil, "", fmt.Errorf("baseline Phase 0 benchmark: %w\n%s", err, baselineOutput)
	}
	candidateOutput, err := run(root, "go", append(args, "-count", "3")...)
	if err != nil {
		return nil, "", fmt.Errorf("candidate Phase 0 benchmark: %w\n%s", err, candidateOutput)
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "baseline-phase0.txt"), baselineOutput, 0o644); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "candidate-phase0.txt"), candidateOutput, 0o644); err != nil {
		return nil, "", err
	}
	baselineSamples, err := parseBenchmarkSamples(baselineOutput)
	if err != nil {
		return nil, "", err
	}
	candidateSamples, err := parseBenchmarkSamples(candidateOutput)
	if err != nil {
		return nil, "", err
	}
	comparisons, err := compareInherited("phase0", baselineSamples, candidateSamples, 15, 3, waiver, true)
	return comparisons, benchmarkCPU(candidateOutput), err
}

func preparePinnedHarnessWorktree(root, revision string, harness map[string]string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "cervterm-phase15-inherited-")
	if err != nil {
		return "", nil, err
	}
	worktree := filepath.Join(parent, "baseline")
	cleanup := func() {
		_, _ = run(root, "git", "worktree", "remove", "--force", worktree)
		_ = os.RemoveAll(parent)
	}
	if output, err := run(root, "git", "worktree", "add", "--detach", worktree, revision); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create detached worktree: %w: %s", err, output)
	}
	for relative, expectedDigest := range harness {
		content, err := approvedHarnessBlob(root, relative, expectedDigest)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		destination := filepath.Join(worktree, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := rejectSymlinkComponents(worktree, filepath.Dir(destination)); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(destination, content, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	status, err := run(worktree, "git", "status", "--porcelain")
	if err != nil {
		cleanup()
		return "", nil, err
	}
	for _, line := range strings.Split(string(status), "\n") {
		if line == "" {
			continue
		}
		if len(line) < 4 {
			cleanup()
			return "", nil, fmt.Errorf("unexpected baseline status row %q", line)
		}
		path := filepath.ToSlash(strings.TrimSpace(line[3:]))
		if _, allowed := harness[path]; !allowed {
			cleanup()
			return "", nil, fmt.Errorf("unexpected baseline mutation %s", path)
		}
	}
	return worktree, cleanup, nil
}

type inheritedSuiteSpec struct {
	packages []string
	pattern  string
	tags     string
}

func captureInterleavedSuite(baselineRoot, candidateRoot, suite string) ([]byte, []byte, string, error) {
	spec, err := inheritedSuite(suite)
	if err != nil {
		return nil, nil, "", err
	}
	args := func(benchTime string) []string {
		result := []string{"test"}
		if spec.tags != "" {
			result = append(result, "-tags", spec.tags)
		}
		result = append(result, spec.packages...)
		return append(result, "-run", "^$", "-bench", spec.pattern, "-benchmem", "-benchtime", benchTime, "-cpu", "1", "-count", "1")
	}
	for _, directory := range []string{baselineRoot, candidateRoot} {
		if output, err := run(directory, "go", args("5s")...); err != nil {
			return nil, nil, "", fmt.Errorf("warm %s: %w\n%s", directory, err, output)
		}
	}
	var baselineOutput, candidateOutput bytes.Buffer
	cpu := ""
	for sample := 0; sample < 10; sample++ {
		runs := []struct {
			directory string
			output    *bytes.Buffer
		}{{baselineRoot, &baselineOutput}, {candidateRoot, &candidateOutput}}
		if sample%2 != 0 {
			runs[0], runs[1] = runs[1], runs[0]
		}
		for _, item := range runs {
			output, err := run(item.directory, "go", args("2s")...)
			if err != nil {
				return nil, nil, "", fmt.Errorf("sample %d in %s: %w\n%s", sample+1, item.directory, err, output)
			}
			measuredCPU := benchmarkCPU(output)
			if measuredCPU == "" {
				return nil, nil, "", fmt.Errorf("sample %d has no CPU metadata", sample+1)
			}
			if cpu == "" {
				cpu = measuredCPU
			} else if cpu != measuredCPU {
				return nil, nil, "", fmt.Errorf("CPU changed from %q to %q", cpu, measuredCPU)
			}
			item.output.Write(output)
			item.output.WriteByte('\n')
		}
	}
	return baselineOutput.Bytes(), candidateOutput.Bytes(), cpu, nil
}

func inheritedSuite(name string) (inheritedSuiteSpec, error) {
	switch name {
	case "text":
		return inheritedSuiteSpec{packages: []string{"./internal/vt", "./internal/core", "./internal/render"}, pattern: "BenchmarkPhase13(TextOnly|Disabled)"}, nil
	case "control":
		return inheritedSuiteSpec{packages: []string{"./internal/vt"}, pattern: "^BenchmarkPhase13ControlString(Discard|Overflow)$"}, nil
	case "store":
		return inheritedSuiteSpec{packages: []string{"./internal/termimage"}, pattern: "^Benchmark(ProcessBudget|Store)"}, nil
	case "glfw":
		return inheritedSuiteSpec{packages: []string{"./internal/frontend/glfwgl"}, pattern: "^BenchmarkPhase13DisabledDraw$", tags: "glfw"}, nil
	case "glfwframe":
		return inheritedSuiteSpec{packages: []string{"./internal/frontend/glfwgl"}, pattern: "^BenchmarkPhase13DisabledFrame$", tags: "glfw"}, nil
	default:
		return inheritedSuiteSpec{}, fmt.Errorf("unknown inherited suite %q", name)
	}
}

func benchmarkCPU(output []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if value, found := strings.CutPrefix(scanner.Text(), "cpu: "); found {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func immutableBaseline(root, relative string) ([]byte, string, error) {
	if _, err := insideRepository(root, relative); err != nil {
		return nil, "", err
	}
	pinned, err := run(root, "git", "show", phase14EvidenceCommit+":"+filepath.ToSlash(relative))
	if err != nil {
		return nil, "", fmt.Errorf("read pinned baseline blob: %w", err)
	}
	digest := sha256.Sum256(pinned)
	return pinned, fmt.Sprintf("%x", digest), nil
}

func assertCleanAtRevision(root, revision string) error {
	current := strings.TrimSpace(string(mustRun(root, "git", "rev-parse", "HEAD")))
	if current != revision {
		return fmt.Errorf("HEAD changed from %s to %s", revision, current)
	}
	status, err := run(root, "git", "status", "--porcelain")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(status)) != 0 {
		return fmt.Errorf("working tree became dirty")
	}
	return nil
}

func readInheritedReport(path string) (inheritedMetadata, map[string][]benchmarkSample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inheritedMetadata{}, nil, err
	}
	return readInheritedData(data)
}

func readInheritedData(data []byte) (inheritedMetadata, map[string][]benchmarkSample, error) {
	line, rest, found := bytes.Cut(data, []byte{'\n'})
	if !found || !bytes.HasPrefix(line, []byte(phase13MetadataPrefix)) {
		return inheritedMetadata{}, nil, fmt.Errorf("missing metadata header")
	}
	var metadata inheritedMetadata
	if err := json.Unmarshal(bytes.TrimPrefix(line, []byte(phase13MetadataPrefix)), &metadata); err != nil {
		return inheritedMetadata{}, nil, err
	}
	samples, err := parseBenchmarkSamples(rest)
	return metadata, samples, err
}

func comparableMetadata(baseline, candidate inheritedMetadata) error {
	if baseline.Version != candidate.Version || baseline.Suite != candidate.Suite || baseline.GoVersion != candidate.GoVersion || baseline.GOOS != candidate.GOOS || baseline.GOARCH != candidate.GOARCH || baseline.CPU != candidate.CPU || baseline.GOMAXPROCS != candidate.GOMAXPROCS || baseline.BenchTime != candidate.BenchTime || baseline.Samples != 10 || candidate.Samples != 10 || baseline.HarnessSHA256 != candidate.HarnessSHA256 {
		return fmt.Errorf("host/toolchain/suite/harness/sample identity mismatch")
	}
	if baseline.WorkingTreeDirty || candidate.WorkingTreeDirty {
		return fmt.Errorf("dirty baseline or candidate")
	}
	return nil
}

func parseBenchmarkSamples(data []byte) (map[string][]benchmarkSample, error) {
	result := make(map[string][]benchmarkSample)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		metric := func(label string) (float64, error) {
			for index := 1; index < len(fields); index++ {
				if fields[index] == label && index > 0 {
					return strconv.ParseFloat(fields[index-1], 64)
				}
			}
			return 0, fmt.Errorf("missing %s", label)
		}
		nanoseconds, errNS := metric("ns/op")
		bytesPerOp, errBytes := metric("B/op")
		allocations, errAllocs := metric("allocs/op")
		if errNS != nil || errBytes != nil || errAllocs != nil {
			return nil, fmt.Errorf("invalid benchmark row %q: ns=%v bytes=%v allocs=%v", scanner.Text(), errNS, errBytes, errAllocs)
		}
		for label, value := range map[string]float64{"ns/op": nanoseconds, "B/op": bytesPerOp, "allocs/op": allocations} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return nil, fmt.Errorf("invalid non-finite or negative %s in benchmark row %q", label, scanner.Text())
			}
		}
		name := strings.TrimSuffix(fields[0], "-1")
		result[name] = append(result[name], benchmarkSample{Nanoseconds: nanoseconds, Bytes: bytesPerOp, Allocations: allocations})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no benchmark samples")
	}
	return result, nil
}

func compareInherited(suite string, baseline, candidate map[string][]benchmarkSample, threshold float64, expectedSamples int, approvedWaiver string, requireWaiverUse bool) ([]inheritedComparison, error) {
	if approvedWaiver != "" && approvedWaiver != "P15-W01" {
		return nil, fmt.Errorf("unknown waiver %q", approvedWaiver)
	}
	if len(baseline) != len(candidate) {
		return nil, fmt.Errorf("benchmark set size differs")
	}
	names := make([]string, 0, len(baseline))
	for name := range baseline {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]inheritedComparison, 0, len(names))
	waiverApplied := false
	for _, name := range names {
		candidateRows, exists := candidate[name]
		if !exists {
			return nil, fmt.Errorf("candidate is missing benchmark %s", name)
		}
		if len(baseline[name]) != expectedSamples || len(candidateRows) != expectedSamples {
			return nil, fmt.Errorf("%s does not contain exactly %d samples", name, expectedSamples)
		}
		left, right := summarize(name, baseline[name]), summarize(name, candidateRows)
		waiver := ""
		if right.MedianAllocations > left.MedianAllocations {
			return nil, fmt.Errorf("%s allocation-count regression from %.0f to %.0f allocs", name, left.MedianAllocations, right.MedianAllocations)
		}
		if right.MedianBytes > left.MedianBytes {
			allowed := false
			if suite == "phase0" && name == "BenchmarkCoreReuseVsNew/new-terminal" && approvedWaiver == "P15-W01" && left.MedianBytes > 0 {
				increase := (right.MedianBytes - left.MedianBytes) / left.MedianBytes * 100
				allowed = increase <= 2
			}
			if suite == "phase0" && name == "BenchmarkCaptureReuse" && approvedWaiver == "P15-W01" {
				allowed = right.MedianBytes <= 8 && right.MedianAllocations == 0
			}
			if !allowed {
				return nil, fmt.Errorf("%s allocation-byte regression from %.0f to %.0f B/op", name, left.MedianBytes, right.MedianBytes)
			}
			waiver = "P15-W01: bounded setup/metadata bytes; allocation count unchanged"
			waiverApplied = true
		}
		delta := (right.MedianNanoseconds - left.MedianNanoseconds) / left.MedianNanoseconds * 100
		effectiveThreshold := threshold
		if delta > threshold && suite == "store" && name == "BenchmarkStoreAcquireMiss" && approvedWaiver == "P15-W01" {
			effectiveThreshold = 10
			waiver = "P15-W01: sub-nanosecond miss-path measurement floor; 10 ns absolute candidate budget retained"
			waiverApplied = true
		}
		if delta > threshold && suite == "control" && name == "BenchmarkPhase13ControlStringOverflow" && approvedWaiver == "P15-W01" {
			effectiveThreshold = 15
			waiver = "P15-W01: selected-DCS preamble and bounded adapter routing; allocation-free streaming retained"
			waiverApplied = true
		}
		if delta > threshold && suite == "phase0" && name == "BenchmarkCaptureReuse" && approvedWaiver == "P15-W01" {
			effectiveThreshold = 400
			waiver = "P15-W01: detached snapshot metadata growth; zero steady-state allocations retained"
			waiverApplied = true
		}
		if delta > effectiveThreshold {
			return nil, fmt.Errorf("%s median regressed %.2f%% above %.2f%%", name, delta, effectiveThreshold)
		}
		result = append(result, inheritedComparison{Suite: suite, Benchmark: name, BaselineNanoseconds: left.MedianNanoseconds, CandidateNanoseconds: right.MedianNanoseconds, DeltaPercent: delta, BaselineBytes: left.MedianBytes, CandidateBytes: right.MedianBytes, BaselineAllocations: left.MedianAllocations, CandidateAllocations: right.MedianAllocations, ThresholdPercent: effectiveThreshold, Waiver: waiver})
	}
	for name := range candidate {
		if _, exists := baseline[name]; !exists {
			return nil, fmt.Errorf("candidate added unpaired benchmark %s", name)
		}
	}
	if requireWaiverUse && approvedWaiver != "" && !waiverApplied {
		return nil, fmt.Errorf("waiver %s was not applicable", approvedWaiver)
	}
	return result, nil
}

func summarizeSamples(samples map[string][]benchmarkSample) []benchmarkSummary {
	names := make([]string, 0, len(samples))
	for name := range samples {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]benchmarkSummary, 0, len(names))
	for _, name := range names {
		result = append(result, summarize(name, samples[name]))
	}
	return result
}

func summarize(name string, samples []benchmarkSample) benchmarkSummary {
	ns, bytesPerOp, allocations := make([]float64, len(samples)), make([]float64, len(samples)), make([]float64, len(samples))
	for index, sample := range samples {
		ns[index], bytesPerOp[index], allocations[index] = sample.Nanoseconds, sample.Bytes, sample.Allocations
	}
	return benchmarkSummary{Name: name, Samples: len(samples), MedianNanoseconds: median(ns), MedianBytes: median(bytesPerOp), MedianAllocations: median(allocations)}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[middle-1] + sorted[middle]) / 2
	}
	return sorted[middle]
}

func enforceAbsoluteBudgets(summaries []benchmarkSummary) error {
	budgets := map[string]absoluteBudget{
		"BenchmarkPhase15InputEncode":             {MaxNanoseconds: 200, MaxBytes: 64, MaxAllocations: 4},
		"BenchmarkPhase15TerminalStartupMemory":   {MaxNanoseconds: 100000, MaxBytes: 256 << 10, MaxAllocations: 16},
		"BenchmarkPhase15ResizeReflow":            {MaxNanoseconds: 6000000, MaxBytes: 30 << 20, MaxAllocations: 70000},
		"BenchmarkPhase15SemanticProjection":      {MaxNanoseconds: 10000, MaxBytes: 0, MaxAllocations: 0},
		"BenchmarkPhase15FontEnvironmentRebuild":  {MaxNanoseconds: 15000, MaxBytes: 16 << 10, MaxAllocations: 128},
		"BenchmarkPhase15ManyTabsWindowsSnapshot": {MaxNanoseconds: 10000, MaxBytes: 12 << 10, MaxAllocations: 200},
		"BenchmarkMuxAllDisabledImageIdle":        {MaxNanoseconds: 100, MaxBytes: 0, MaxAllocations: 0},
		"BenchmarkAccessibilitySemanticCapture":   {MaxNanoseconds: 300000, MaxBytes: 600 << 10, MaxAllocations: 700},
		"BenchmarkAccessibilityEventCoalescing":   {MaxNanoseconds: 250000, MaxBytes: 8 << 10, MaxAllocations: 150},
		"BenchmarkProcessBudgetReserveRelease":    {MaxNanoseconds: 100, MaxBytes: 128, MaxAllocations: 2},
		"BenchmarkStoreBeginTransferCancel":       {MaxNanoseconds: 300, MaxBytes: 512, MaxAllocations: 4},
		"BenchmarkStoreAcquireMiss":               {MaxNanoseconds: 10, MaxBytes: 0, MaxAllocations: 0},
		"BenchmarkSchedulerSubmitComplete":        {MaxNanoseconds: 1000, MaxBytes: 512, MaxAllocations: 8},
		"BenchmarkSixelTokenizer256KiB":           {MaxNanoseconds: 1500000, MaxBytes: 0, MaxAllocations: 0},
		"BenchmarkSixelAdapterSeal256KiB":         {MaxNanoseconds: 2000000, MaxBytes: 400 << 10, MaxAllocations: 100},
		"BenchmarkSixelDecodeWorker256x64":        {MaxNanoseconds: 500000, MaxBytes: 128 << 10, MaxAllocations: 64},
		"BenchmarkITermScanner256KiB":             {MaxNanoseconds: 2000000, MaxBytes: 0, MaxAllocations: 0},
		"BenchmarkITermAdapterSeal256KiB":         {MaxNanoseconds: 2500000, MaxBytes: 400 << 10, MaxAllocations: 100},
		"BenchmarkITermDecodeWorker256x64":        {MaxNanoseconds: 1000000, MaxBytes: 400 << 10, MaxAllocations: 20000},
	}
	seen := make(map[string]struct{}, len(budgets))
	for _, summary := range summaries {
		budget, exists := budgets[summary.Name]
		if !exists {
			continue
		}
		seen[summary.Name] = struct{}{}
		if summary.Samples != 10 || summary.MedianNanoseconds > budget.MaxNanoseconds || summary.MedianBytes > budget.MaxBytes || summary.MedianAllocations > budget.MaxAllocations {
			return fmt.Errorf("%s is %.0f ns/op %.0f B/op %.0f allocs/op across %d samples; budget %.0f ns/op %.0f B/op %.0f allocs/op across 10 samples", summary.Name, summary.MedianNanoseconds, summary.MedianBytes, summary.MedianAllocations, summary.Samples, budget.MaxNanoseconds, budget.MaxBytes, budget.MaxAllocations)
		}
	}
	for name := range budgets {
		if _, exists := seen[name]; !exists {
			return fmt.Errorf("required candidate benchmark %s did not run", name)
		}
	}
	return nil
}

func repositoryRoot() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func insideRepository(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("path must be non-empty and repository-relative")
	}
	absolute := filepath.Join(root, filepath.Clean(filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, absolute)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository")
	}
	return absolute, nil
}

func resolvedInside(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("resolved path escapes repository")
	}
	return nil
}

func rejectSymlinkComponents(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes confinement root")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink or junction component is not allowed: %s", current)
		}
	}
	return nil
}

func mustRun(dir, name string, args ...string) []byte {
	output, err := run(dir, name, args...)
	if err != nil {
		fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return output
}

func run(dir, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	return command.CombinedOutput()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "capture-phase15-benchmarks: "+format+"\n", args...)
	os.Exit(1)
}
