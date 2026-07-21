//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	metaPrefix    = "# phase13-meta "
	wantSamples   = 10
	maxRegression = 3.0
)

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
	MeasuredSourceSHA256 string   `json:"measured_source_sha256,omitempty"`
	WarmCommand          []string `json:"warm_command"`
	RecordCommand        []string `json:"record_command"`
}

type sample struct{ ns, bytes, allocs float64 }
type benchmarkFile struct {
	meta       metadata
	benchmarks map[string][]sample
}
type summary struct{ medianNS, maxBytes, maxAllocs float64 }

var cpuSuffix = regexp.MustCompile(`^(.*)-(\d+)$`)

func main() {
	if len(os.Args) != 3 {
		fatalf("usage: go run ./scripts/compare-phase13-baseline.go BASELINE CANDIDATE")
	}
	base, err := parseFile(os.Args[1])
	if err != nil {
		fatalf("baseline: %v", err)
	}
	cand, err := parseFile(os.Args[2])
	if err != nil {
		fatalf("candidate: %v", err)
	}
	if err := compareMetadata(base.meta, cand.meta); err != nil {
		fatalf("metadata: %v", err)
	}
	if len(base.benchmarks) == 0 {
		fatalf("baseline contains no benchmarks")
	}
	if len(base.benchmarks) != len(cand.benchmarks) {
		fatalf("benchmark set differs: baseline=%d candidate=%d", len(base.benchmarks), len(cand.benchmarks))
	}
	names := make([]string, 0, len(base.benchmarks))
	for name := range base.benchmarks {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("%-72s %12s %12s %9s %10s %10s\n", "benchmark", "baseline", "candidate", "delta", "max B/op", "max allocs")
	var failures []string
	sourceIdentical := base.meta.MeasuredSourceSHA256 != "" && base.meta.MeasuredSourceSHA256 == cand.meta.MeasuredSourceSHA256
	for _, name := range names {
		bs := base.benchmarks[name]
		cs, ok := cand.benchmarks[name]
		if !ok {
			failures = append(failures, name+" missing from candidate")
			continue
		}
		if len(bs) != wantSamples || len(cs) != wantSamples {
			failures = append(failures, fmt.Sprintf("%s sample count baseline=%d candidate=%d want=%d", name, len(bs), len(cs), wantSamples))
			continue
		}
		b, c := summarize(bs), summarize(cs)
		delta := percentChange(b.medianNS, c.medianNS)
		fmt.Printf("%-72s %12.2f %12.2f %+8.2f%% %10.0f %10.0f\n", name, b.medianNS, c.medianNS, delta, c.maxBytes, c.maxAllocs)
		if delta > maxRegression && !sourceIdentical {
			failures = append(failures, fmt.Sprintf("%s median ns/op regression %.2f%% exceeds %.2f%%", name, delta, maxRegression))
		}
		if c.maxBytes > b.maxBytes {
			failures = append(failures, fmt.Sprintf("%s worst B/op increased %.0f -> %.0f", name, b.maxBytes, c.maxBytes))
		}
		if c.maxAllocs > b.maxAllocs {
			failures = append(failures, fmt.Sprintf("%s worst allocs/op increased %.0f -> %.0f", name, b.maxAllocs, c.maxAllocs))
		}
	}
	if sourceIdentical {
		fmt.Println("timing: measured source is identical; timing deltas are diagnostic while allocation gates remain mandatory")
	}
	for name := range cand.benchmarks {
		if _, ok := base.benchmarks[name]; !ok {
			failures = append(failures, name+" appears only in candidate")
		}
	}
	runBenchstat(os.Args[1], os.Args[2])
	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "FAIL:", failure)
		}
		os.Exit(1)
	}
	fmt.Println("phase 13 baseline gate passed")
}

func parseFile(path string) (benchmarkFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return benchmarkFile{}, err
	}
	defer file.Close()
	result := benchmarkFile{benchmarks: map[string][]sample{}}
	var haveMeta bool
	var pkg, goos, goarch, cpu string
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, metaPrefix):
			if haveMeta {
				return benchmarkFile{}, fmt.Errorf("line %d: duplicate metadata", lineNo)
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, metaPrefix)), &result.meta); err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: metadata: %w", lineNo, err)
			}
			haveMeta = true
		case strings.HasPrefix(line, "goos:"):
			if err := observe(&goos, strings.TrimSpace(strings.TrimPrefix(line, "goos:")), "goos"); err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
		case strings.HasPrefix(line, "goarch:"):
			if err := observe(&goarch, strings.TrimSpace(strings.TrimPrefix(line, "goarch:")), "goarch"); err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
		case strings.HasPrefix(line, "cpu:"):
			if err := observe(&cpu, strings.TrimSpace(strings.TrimPrefix(line, "cpu:")), "cpu"); err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
		case strings.HasPrefix(line, "pkg:"):
			pkg = strings.TrimSpace(strings.TrimPrefix(line, "pkg:"))
			if pkg == "" {
				return benchmarkFile{}, fmt.Errorf("line %d: empty package", lineNo)
			}
		case strings.HasPrefix(line, "Benchmark"):
			if !haveMeta || pkg == "" {
				return benchmarkFile{}, fmt.Errorf("line %d: benchmark lacks preceding metadata/package", lineNo)
			}
			fields := strings.Fields(line)
			parsed, err := parseSample(fields)
			if err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			name, benchCPU, err := normalizeName(fields[0], result.meta.GOMAXPROCS)
			if err != nil {
				return benchmarkFile{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			if benchCPU != result.meta.GOMAXPROCS {
				return benchmarkFile{}, fmt.Errorf("line %d: CPU suffix %d differs from metadata %d", lineNo, benchCPU, result.meta.GOMAXPROCS)
			}
			key := pkg + "::" + name
			result.benchmarks[key] = append(result.benchmarks[key], parsed)
		}
	}
	if err := scanner.Err(); err != nil {
		return benchmarkFile{}, err
	}
	if !haveMeta {
		return benchmarkFile{}, fmt.Errorf("missing metadata preamble")
	}
	if err := validateMetadata(result.meta); err != nil {
		return benchmarkFile{}, err
	}
	if goos != result.meta.GOOS || goarch != result.meta.GOARCH || cpu != result.meta.CPU {
		return benchmarkFile{}, fmt.Errorf("output identity %q/%q/%q differs from metadata %q/%q/%q", goos, goarch, cpu, result.meta.GOOS, result.meta.GOARCH, result.meta.CPU)
	}
	return result, nil
}

func parseSample(fields []string) (sample, error) {
	if len(fields) < 8 || (len(fields)-2)%2 != 0 {
		return sample{}, fmt.Errorf("malformed benchmark value/unit pairs")
	}
	iterations, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil || iterations == 0 {
		return sample{}, fmt.Errorf("invalid iteration count %q", fields[1])
	}
	var result sample
	seen := make(map[string]bool)
	for i := 2; i < len(fields); i += 2 {
		valueText, unit := fields[i], fields[i+1]
		if seen[unit] {
			return sample{}, fmt.Errorf("duplicate metric %q", unit)
		}
		seen[unit] = true
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return sample{}, fmt.Errorf("metric %q before %q must be finite and non-negative", valueText, unit)
		}
		switch unit {
		case "ns/op":
			result.ns = value
		case "B/op":
			result.bytes = value
		case "allocs/op":
			result.allocs = value
		}
	}
	if !seen["ns/op"] || !seen["B/op"] || !seen["allocs/op"] {
		return sample{}, fmt.Errorf("benchmark lacks ns/op or -benchmem metrics")
	}
	return result, nil
}

func normalizeName(name string, expectedCPU int) (string, int, error) {
	match := cpuSuffix.FindStringSubmatch(name)
	if match == nil {
		if expectedCPU != 1 {
			return "", 0, fmt.Errorf("%q omits CPU suffix", name)
		}
		return name, 1, nil
	}
	cpu, err := strconv.Atoi(match[2])
	if err != nil || cpu < 1 {
		return "", 0, fmt.Errorf("%q has invalid CPU suffix", name)
	}
	return match[1], cpu, nil
}

func validateMetadata(m metadata) error {
	if m.Version != 1 || m.Suite == "" || m.GoVersion == "" || m.GOOS == "" || m.GOARCH == "" || m.CPU == "" || m.ProductionCommit == "" || m.HarnessSHA256 == "" {
		return fmt.Errorf("metadata incomplete or unsupported")
	}
	if m.Suite == "control" && m.MeasuredSourceSHA256 == "" {
		return fmt.Errorf("control metadata lacks measured source identity")
	}
	if m.GOMAXPROCS != 1 || m.BenchTime != "2s" || m.Samples != wantSamples {
		return fmt.Errorf("method is %d/%s/%d, want 1/2s/%d", m.GOMAXPROCS, m.BenchTime, m.Samples, wantSamples)
	}
	if len(m.WarmCommand) == 0 || len(m.RecordCommand) == 0 {
		return fmt.Errorf("metadata commands missing")
	}
	return nil
}

func compareMetadata(a, b metadata) error {
	pairs := [][3]string{{"suite", a.Suite, b.Suite}, {"go_version", a.GoVersion, b.GoVersion}, {"goos", a.GOOS, b.GOOS}, {"goarch", a.GOARCH, b.GOARCH}, {"cpu", a.CPU, b.CPU}, {"benchtime", a.BenchTime, b.BenchTime}, {"harness", a.HarnessSHA256, b.HarnessSHA256}}
	for _, pair := range pairs {
		if pair[1] != pair[2] {
			return fmt.Errorf("%s differs: %q vs %q", pair[0], pair[1], pair[2])
		}
	}
	if a.GOMAXPROCS != b.GOMAXPROCS || a.Samples != b.Samples || !slices.Equal(a.WarmCommand, b.WarmCommand) || !slices.Equal(a.RecordCommand, b.RecordCommand) {
		return fmt.Errorf("method command metadata differs")
	}
	return nil
}

func observe(destination *string, value, label string) error {
	if value == "" {
		return fmt.Errorf("empty %s", label)
	}
	if *destination != "" && *destination != value {
		return fmt.Errorf("mixed %s values %q and %q", label, *destination, value)
	}
	*destination = value
	return nil
}

func summarize(samples []sample) summary {
	ns := make([]float64, len(samples))
	var result summary
	for i, value := range samples {
		ns[i] = value.ns
		result.maxBytes = math.Max(result.maxBytes, value.bytes)
		result.maxAllocs = math.Max(result.maxAllocs, value.allocs)
	}
	sort.Float64s(ns)
	result.medianNS = (ns[(len(ns)-1)/2] + ns[len(ns)/2]) / 2
	return result
}

func percentChange(base, candidate float64) float64 {
	if base == 0 {
		if candidate == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return (candidate/base - 1) * 100
}

func runBenchstat(base, candidate string) {
	path, err := exec.LookPath("benchstat")
	if err != nil {
		fmt.Println("benchstat: unavailable; self-contained gate remains authoritative")
		return
	}
	command := exec.Command(path, base, candidate)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "benchstat: supplemental command failed: %v\n", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "compare-phase13-baseline: "+format+"\n", args...)
	os.Exit(2)
}
