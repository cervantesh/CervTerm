//go:build ignore

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
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
	slice31Base             = "9fe0bd0287ed402fe90b58a5fba7d6413897c1af"
	slice31T                = "4a9c2800ac2bbd943eb93e892f45abe01a4328bd"
	slice31A                = "cf834b7870e34689c789f38aeaf6de4914b34675"
	slice31W                = "27070f32395be7803b6fc11388a08a5c32d3d3a1"
	slice31GSubj            = "refactor(mux): guard executable owner capability"
	slice31Branch           = "arch/l3-02-owner-thread-capability-rebuild"
	slice31Doc              = "docs/validation/architecture-maturity-slice-3.1.md"
	evidenceDir             = "docs/validation/architecture-maturity-slice-3.1"
	redesignPathManifest    = "docs/validation/architecture-maturity-slice-3.1/redesign-paths.txt"
	redesignPathManifestSHA = "9c7d0f77f0f3dd7a950b3c50b82c0533a5394b4b64cfa1d13e0a1bf7fcc779b7"
)

var slice31Stages = []struct{ class, commit, parent, subject string }{
	{"T", slice31T, slice31Base, "test(mux): characterize owner mutation boundary"},
	{"A", slice31A, slice31T, "refactor(mux): add owner capability seam"},
	{"W", slice31W, slice31A, "refactor(mux): wire executable owner capability"},
}

var slice31GPaths = []string{
	".architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0017-establish-explicit-process-and-mux-owner-capabilities.md",
	"docs/architecture-maturity/accepted-findings.md", "docs/architecture-maturity/implementation-plan.md", "docs/architecture.md",
	"docs/validation/architecture-maturity-slice-3.1.md", "docs/validation/architecture-maturity-slice-3.1/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-3.1/benchmarks-candidate.txt", "docs/validation/architecture-maturity-slice-3.1/gates.txt",
	"docs/validation/architecture-maturity-slice-3.1/redesign-paths.txt", "docs/validation/architecture-maturity-slice-3.1/scope-and-commits.txt",
	"docs/validation/architecture-maturity-slice-3.1/source-binary-hashes.txt",
	"internal/frontend/glfwgl/process_capability_boundary_test.go", "internal/frontend/glfwgl/window_controller_native_guard_test.go",
	"internal/mux/owner_characterization_test.go", "internal/mux/owner_guard_test.go", "internal/mux/owner_recursive_guard_test.go", "internal/mux/owner_types_analysis_test.go", "internal/mux/owner_types_guard_test.go",
	"internal/termimage/prepared_owner_scope_test.go",
	"scripts/capture-slice31-evidence.go", "scripts/check-maturity-gates.go", "scripts/check-slice31-owner.go",
}

func main() {
	var failures []string
	failures = append(failures, selfTest()...)
	failures = append(failures, checkDocument()...)
	failures = append(failures, checkGit()...)
	failures = append(failures, checkEvidence()...)
	if out, err := run("go", "test", "./internal/mux", "-run", "^(TestL302.*|TestEveryStructurallyDerivedOwnerMutationRejectsWrongThreadReentrantStaleAndClosed)$", "-count=1"); err != nil {
		failures = append(failures, "recursive source/API/control-flow guard failed: "+strings.TrimSpace(out))
	}
	if out, err := run("go", "test", "./internal/termimage", "-run", "^TestPreparedStore(State|Close)", "-count=1"); err != nil {
		failures = append(failures, "termimage prepared ownership guard failed: "+strings.TrimSpace(out))
	}
	if runtime.GOOS == "windows" {
		if out, err := run("go", "test", "-tags", "glfw", "./internal/frontend/glfwgl", "-run", "^Test(FrontendRetainsOnlyNarrowMuxCapabilities|ProductionWindowAttestation|WindowControllerNative|Child|ControllerMintsChild)", "-count=1"); err != nil {
			failures = append(failures, "frontend capability/origin/thread/router guard failed: "+strings.TrimSpace(out))
		}
	}
	if len(failures) != 0 {
		fmt.Fprintln(os.Stderr, "Slice 3.1 owner guard failures:")
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "FAIL", failure)
		}
		os.Exit(1)
	}
	fmt.Println("Slice 3.1 owner guards ok")
}

func checkDocument() []string {
	data, err := os.ReadFile(slice31Doc)
	if err != nil {
		return []string{slice31Doc + ": " + err.Error()}
	}
	text := normalizedLF(data)
	manifestPaths, manifestFailures := readRedesignPathManifest()
	required := []string{slice31Base, slice31T, slice31A, slice31W, slice31GSubj, "G is derived", "shared `internal/ownerthread` leaf", "exact OS-thread identity", "loop epoch", "ephemeral process and window mutation scopes", "window incarnation", "typed message router", "prepared scopes", "L3-02 is **closed only when**", "ten physical", "immutable pre-slice baseline", "W is rejected as a benchmark baseline", "full-history", fmt.Sprintf("sorted %d-path two-dot allowlist", len(manifestPaths))}
	failures := append([]string(nil), manifestFailures...)
	for _, value := range required {
		if !strings.Contains(text, value) {
			failures = append(failures, slice31Doc+": missing truthful contract "+strconv.Quote(value))
		}
	}
	return failures
}

func checkGit() []string {
	branch, _ := git("symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := git("rev-parse", "HEAD")
	shallow, _ := git("rev-parse", "--is-shallow-repository")
	porcelain, _ := gitRaw("status", "--porcelain=v1", "--untracked-files=all")
	if shallow == "true" {
		return []string{"git:Slice 3.1 requires a full-history checkout"}
	}
	var failures []string
	for _, stage := range slice31Stages {
		identity, err := git("show", "-s", "--format=%H%x00%P%x00%s", stage.commit)
		want := stage.commit + "\x00" + stage.parent + "\x00" + stage.subject
		if err != nil || identity != want {
			failures = append(failures, "git:"+stage.class+" exact SHA/sole-parent/subject mismatch")
		}
	}
	gCommit, gFailures := discoverGCommit()
	failures = append(failures, gFailures...)
	if branch == slice31Branch && head == slice31W {
		failures = append(failures, exactSet(porcelainPaths(porcelain), slice31GPaths, "pre-G path")...)
		if gCommit != "" {
			failures = append(failures, "git:pre-G state already has a reachable exact-subject G")
		}
		return failures
	}
	if gCommit == "" {
		failures = append(failures, "git:G exact subject commit missing")
		return failures
	}
	failures = append(failures, checkRollbackSequence(gCommit)...)
	gPaths, gPathErr := git("diff", "--name-only", committedRangeSpec(slice31W, gCommit))
	if gPathErr != nil {
		failures = append(failures, "git:G-range "+gPathErr.Error())
	} else {
		failures = append(failures, exactSet(nonemptyLines(gPaths), slice31GPaths, "G-range path")...)
	}
	paths, err := git("diff", "--name-only", committedRangeSpec(slice31Base, gCommit))
	if err != nil {
		failures = append(failures, "git:committed-range "+err.Error())
	} else {
		expected, expectedErr := slice31ExpectedAllowed()
		if expectedErr != nil {
			failures = append(failures, "git:expected allowlist "+expectedErr.Error())
		} else {
			failures = append(failures, exactSet(nonemptyLines(paths), expected, "committed-range path")...)
		}
	}
	_, manifestFailures := readRedesignPathManifest()
	failures = append(failures, manifestFailures...)
	actualPaths, noiseFailures := stripExactLocalNoise(porcelainPaths(porcelain))
	failures = append(failures, noiseFailures...)
	if branch == slice31Branch {
		if head != gCommit {
			failures = append(failures, "git:active branch HEAD must be the unique exact-subject G")
		}
		failures = append(failures, exactSet(actualPaths, nil, "clean G tree")...)
	} else {
		if _, err := git("merge-base", "--is-ancestor", gCommit, head); err != nil {
			failures = append(failures, "git:merged/full-history tree does not contain G")
		}
		failures = append(failures, exactSet(actualPaths, nil, "merged clean tree")...)
	}
	return failures
}

func committedRangeSpec(base, head string) string { return base + ".." + head }

func slice31ExpectedAllowed() ([]string, error) {
	committed, err := git("diff", "--name-only", committedRangeSpec(slice31Base, slice31W))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, path := range append(nonemptyLines(committed), slice31GPaths...) {
		seen[path] = true
	}
	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func checkRollbackSequence(gCommit string) []string {
	path, err := os.MkdirTemp("", "cervterm-slice31-rollback-")
	if err != nil {
		return []string{"rollback:create temp: " + err.Error()}
	}
	_ = os.Remove(path)
	runGit := func(args ...string) error {
		command := exec.Command("git", args...)
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), commandErr, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if err := runGit("worktree", "add", "--detach", path, gCommit); err != nil {
		return []string{"rollback:add worktree: " + err.Error()}
	}
	defer func() {
		_ = runGit("worktree", "remove", "--force", path)
		_ = os.RemoveAll(path)
	}()
	for _, commit := range []string{gCommit, slice31W, slice31A} {
		if err := runGit("-C", path, "revert", "--no-commit", commit); err != nil {
			return []string{"rollback:reverse G/W/A: " + err.Error()}
		}
	}
	if err := runGit("-C", path, "diff", "--quiet", slice31T, "--"); err != nil {
		return []string{"rollback:reversed tree does not equal retained T: " + err.Error()}
	}
	return nil
}

func readRedesignPathManifest() ([]string, []string) {
	data, err := os.ReadFile(redesignPathManifest)
	if err != nil {
		return nil, []string{"git:redesign path manifest unreadable: " + err.Error()}
	}
	normalized := normalizedLF(data)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(normalized))); got != redesignPathManifestSHA {
		return nil, []string{fmt.Sprintf("git:redesign path manifest hash=%s want=%s", got, redesignPathManifestSHA)}
	}
	paths := nonemptyLines(normalized)
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	if strings.Join(paths, "\n") != strings.Join(sorted, "\n") {
		return nil, []string{"git:redesign path manifest must remain sorted"}
	}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			return nil, []string{"git:redesign path manifest has blank/duplicate path " + path}
		}
		seen[path] = true
	}
	expected, expectedErr := slice31ExpectedAllowed()
	if expectedErr != nil {
		return nil, []string{"git:expected manifest allowlist: " + expectedErr.Error()}
	}
	if failures := exactSet(paths, expected, "git:redesign path manifest"); len(failures) != 0 {
		return nil, failures
	}
	return paths, nil
}

func stripExactLocalNoise(paths []string) ([]string, []string) {
	const localNoise = ".agent/memory/episodic/AGENT_LEARNINGS.jsonl"
	var result []string
	count := 0
	for _, path := range paths {
		if path == localNoise {
			count++
			continue
		}
		result = append(result, path)
	}
	if count > 1 {
		return result, []string{fmt.Sprintf("git:local Agentic Stack memory noise count=%d want <=1", count)}
	}
	return result, nil
}

func discoverGCommit() (string, []string) {
	matches, _ := gitRaw("log", "HEAD", "--format=%H%x00%P%x00%s")
	var commits []string
	var failures []string
	for _, line := range nonemptyLines(matches) {
		parts := strings.Split(line, "\x00")
		if len(parts) != 3 || parts[2] != slice31GSubj {
			continue
		}
		commits = append(commits, parts[0])
		if parts[1] != slice31W {
			failures = append(failures, "git:G exact-subject commit has wrong sole parent")
		}
	}
	if len(commits) > 1 {
		failures = append(failures, fmt.Sprintf("git:G exact subject count=%d want unique 1", len(commits)))
	}
	if len(commits) == 1 {
		return commits[0], failures
	}
	return "", failures
}

func commitSourceHash(commit string) (string, error) {
	listed, err := git("ls-tree", "-r", "--name-only", commit, "--", "internal/mux", "internal/core", "internal/termimage", "internal/frontend/glfwgl", "internal/ownerthread")
	if err != nil {
		return "", err
	}
	paths := nonemptyLines(listed)
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, showErr := exec.Command("git", "show", commit+":"+path).Output()
		if showErr != nil {
			return "", showErr
		}
		fmt.Fprintf(hash, "%s\x00%s\n", path, normalizedLF(data))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

type benchSample struct {
	round, sequence                                       int
	benchmark, function, side, nonce, binary, commandHash string
	started, ended                                        time.Time
	ns, bytes, allocs                                     float64
}

type benchEvidence struct {
	headers map[string]string
	samples []benchSample
}

func parseEvidence(path string, content []byte) (benchEvidence, []string) {
	result := benchEvidence{headers: make(map[string]string)}
	var failures []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "sample ") {
			fields := parseFields(strings.TrimPrefix(line, "sample "))
			sample := benchSample{benchmark: fields["benchmark"], function: fields["function"], side: fields["side"], nonce: fields["nonce"], binary: fields["binary_sha256"], commandHash: fields["command_sha256"]}
			var err error
			sample.round, err = strconv.Atoi(fields["round"])
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid round", path, lineNumber))
			}
			sample.sequence, err = strconv.Atoi(fields["sequence"])
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid sequence", path, lineNumber))
			}
			sample.started, err = time.Parse(time.RFC3339Nano, fields["started"])
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid started timestamp", path, lineNumber))
			}
			sample.ended, err = time.Parse(time.RFC3339Nano, fields["ended"])
			if err != nil || sample.ended.Before(sample.started) {
				failures = append(failures, fmt.Sprintf("%s:%d invalid ended timestamp", path, lineNumber))
			}
			sample.ns, err = strconv.ParseFloat(fields["ns_op"], 64)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid ns_op", path, lineNumber))
			}
			sample.bytes, err = strconv.ParseFloat(fields["bytes_op"], 64)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid bytes_op", path, lineNumber))
			}
			sample.allocs, err = strconv.ParseFloat(fields["allocs_op"], 64)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d invalid allocs_op", path, lineNumber))
			}
			result.samples = append(result.samples, sample)
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			if _, duplicate := result.headers[key]; duplicate {
				failures = append(failures, fmt.Sprintf("%s:%d duplicate header %s", path, lineNumber, key))
			}
			result.headers[key] = value
		} else {
			failures = append(failures, fmt.Sprintf("%s:%d malformed evidence line", path, lineNumber))
		}
	}
	if err := scanner.Err(); err != nil {
		failures = append(failures, path+": "+err.Error())
	}
	return result, failures
}

func checkEvidence() []string {
	hashData, err := os.ReadFile(filepath.Join(evidenceDir, "source-binary-hashes.txt"))
	if err != nil {
		return []string{err.Error()}
	}
	hashes := parseFields(strings.ReplaceAll(normalizedLF(hashData), "\n", " "))
	candidate, err := commitSourceHash(slice31W)
	if err != nil {
		return []string{err.Error()}
	}
	base, baseErr := commitSourceHash(slice31Base)
	if baseErr != nil {
		return []string{baseErr.Error()}
	}
	var harnessData []byte
	if gCommit, _ := discoverGCommit(); gCommit != "" {
		command := exec.Command("git", "show", gCommit+":internal/mux/owner_characterization_test.go")
		harnessData, err = command.Output()
		if err != nil {
			return []string{err.Error()}
		}
	} else {
		harnessData, err = os.ReadFile("internal/mux/owner_characterization_test.go")
		if err != nil {
			return []string{err.Error()}
		}
	}
	candidateHarness := fmt.Sprintf("%x", sha256.Sum256([]byte(normalizedLF(harnessData))))
	var failures []string
	hashKeys := []string{"base_binary_sha256", "base_command", "base_command_sha256", "base_harness_sha256", "base_identity", "base_normalized_lf_sha256", "candidate_binary_sha256", "candidate_command", "candidate_command_sha256", "candidate_harness_sha256", "candidate_head", "candidate_identity", "candidate_normalized_lf_sha256", "schema"}
	failures = append(failures, exactSet(mapKeys(hashes), hashKeys, "evidence:source/binary manifest key")...)
	for key, want := range map[string]string{
		"schema": "slice31-source-binary-manifest-v4", "base_identity": slice31Base, "candidate_identity": "detached-W+harness-overlay", "candidate_head": slice31W,
		"candidate_normalized_lf_sha256": candidate, "base_normalized_lf_sha256": base, "candidate_harness_sha256": candidateHarness,
	} {
		if hashes[key] != want {
			failures = append(failures, fmt.Sprintf("evidence:source/binary manifest %s=%q want %q", key, hashes[key], want))
		}
	}
	for _, side := range []string{"base", "candidate"} {
		command := hashes[side+"_command"]
		wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(command)))
		if command == "" || hashes[side+"_command_sha256"] != wantHash {
			failures = append(failures, "evidence:"+side+" logical command/hash binding mismatch")
		}
		if hashes[side+"_binary_sha256"] == "" || len(hashes[side+"_binary_sha256"]) != 64 {
			failures = append(failures, "evidence:"+side+" binary hash missing/malformed")
		}
	}
	basePath := filepath.Join(evidenceDir, "benchmarks-base.txt")
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return append(failures, err.Error())
	}
	baseMetadata := parseFields(strings.ReplaceAll(normalizedLF(baseData), "\n", " "))
	failures = append(failures, exactSet(mapKeys(baseMetadata), []string{"base_harness_sha256", "binary_sha256", "command_sha256", "identity", "logical_command", "normalized_lf_source_sha256", "platform_compile", "platform_runtime", "schema"}, "evidence:base metadata key")...)
	for key, want := range map[string]string{
		"schema": "slice31-source-bound-base-v3", "identity": slice31Base, "normalized_lf_source_sha256": base,
		"binary_sha256": hashes["base_binary_sha256"], "base_harness_sha256": hashes["base_harness_sha256"],
		"logical_command": hashes["base_command"], "command_sha256": hashes["base_command_sha256"],
	} {
		if baseMetadata[key] != want {
			failures = append(failures, fmt.Sprintf("evidence:base metadata %s=%q want %q", key, baseMetadata[key], want))
		}
	}
	path := filepath.Join(evidenceDir, "benchmarks-candidate.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return append(failures, err.Error())
	}
	evidence, parseFailures := parseEvidence(path, data)
	failures = append(failures, parseFailures...)
	headerKeys := []string{"base_binary_sha256", "base_command", "base_command_sha256", "base_harness_sha256", "base_identity", "base_source_sha256", "candidate_binary_sha256", "candidate_command", "candidate_command_sha256", "candidate_harness_sha256", "candidate_head", "candidate_identity", "candidate_source_sha256", "capture_finished", "capture_started", "order", "physical_processes", "platform_runtime", "samples_per_side", "schema", "threshold_percent", "warmup_benchtime", "warmup_order", "warmup_per_side", "warmup_processes"}
	failures = append(failures, exactSet(mapKeys(evidence.headers), headerKeys, "evidence:benchmark header key")...)
	for key, want := range map[string]string{
		"schema": "slice31-abba-v4", "samples_per_side": "10", "physical_processes": "120", "threshold_percent": "3.000000", "order": "ABBAx5",
		"warmup_processes": "12", "warmup_per_side": "1", "warmup_benchtime": "2s", "warmup_order": "base,candidate",
		"base_identity": slice31Base, "candidate_identity": "detached-W+harness-overlay", "candidate_head": slice31W,
		"candidate_source_sha256": candidate, "base_source_sha256": base,
	} {
		if evidence.headers[key] != want {
			failures = append(failures, fmt.Sprintf("evidence:%s=%q want %q", key, evidence.headers[key], want))
		}
	}
	for _, side := range []string{"base", "candidate"} {
		for _, suffix := range []string{"binary_sha256", "harness_sha256", "command", "command_sha256"} {
			if evidence.headers[side+"_"+suffix] != hashes[side+"_"+suffix] {
				failures = append(failures, fmt.Sprintf("evidence:%s %s binding mismatch", side, suffix))
			}
		}
	}
	failures = append(failures, validateSamples(evidence)...)
	return failures
}

func validateSamples(evidence benchEvidence) []string {
	var failures []string
	groups := make(map[string]map[string][]benchSample)
	nonces := make(map[string]bool)
	captureStarted, startErr := time.Parse(time.RFC3339Nano, evidence.headers["capture_started"])
	captureFinished, finishErr := time.Parse(time.RFC3339Nano, evidence.headers["capture_finished"])
	if startErr != nil || finishErr != nil || captureFinished.Before(captureStarted) {
		failures = append(failures, "evidence:invalid capture start/finish timestamps")
	}
	if len(evidence.samples) != 120 {
		failures = append(failures, fmt.Sprintf("evidence:physical sample processes=%d want exact 120", len(evidence.samples)))
	}
	lastSequence := 0
	var previousEnd time.Time
	wantFunctions := map[string]map[string]string{
		"ProcessMutation": {"base": "BenchmarkL302EvidenceProcessMutation", "candidate": "BenchmarkL302ProcessMutationCandidate"},
		"WindowMutation":  {"base": "BenchmarkL302EvidenceWindowMutation", "candidate": "BenchmarkL302WindowMutationCandidate"},
		"PaneMutation":    {"base": "BenchmarkL302EvidencePaneMutation", "candidate": "BenchmarkL302PaneMutationCandidate"},
		"ImageMutation":   {"base": "BenchmarkL302EvidenceImageMutation", "candidate": "BenchmarkL302ImageMutationCandidate"},
		"StartupProxy":    {"base": "BenchmarkL302EvidenceStartupProxy", "candidate": "BenchmarkL302StartupProxyCandidate"},
		"HeadlessProxy":   {"base": "BenchmarkL302EvidenceHeadlessProxy", "candidate": "BenchmarkL302HeadlessFrameCandidate"},
	}
	for _, sample := range evidence.samples {
		if sample.sequence != lastSequence+1 {
			failures = append(failures, fmt.Sprintf("evidence:physical sequence=%d after %d", sample.sequence, lastSequence))
		}
		lastSequence = sample.sequence
		if sample.side != "base" && sample.side != "candidate" {
			failures = append(failures, "evidence:invalid sample side "+sample.side)
		}
		if want := wantFunctions[sample.benchmark][sample.side]; want == "" || sample.function != want {
			failures = append(failures, fmt.Sprintf("evidence:%s/%s function=%q want %q", sample.benchmark, sample.side, sample.function, want))
		}
		if sample.nonce == "" || nonces[sample.nonce] {
			failures = append(failures, "evidence:missing/duplicate nonce "+sample.nonce)
		}
		nonces[sample.nonce] = true
		if sample.started.Before(captureStarted) || sample.ended.After(captureFinished) || (!previousEnd.IsZero() && sample.started.Before(previousEnd)) {
			failures = append(failures, fmt.Sprintf("evidence:sample sequence=%d timestamp is outside capture or not physically monotonic", sample.sequence))
		}
		previousEnd = sample.ended
		if sample.ns <= 0 || sample.bytes < 0 || sample.allocs < 0 || math.IsNaN(sample.ns) || math.IsNaN(sample.bytes) || math.IsNaN(sample.allocs) || math.IsInf(sample.ns, 0) || math.IsInf(sample.bytes, 0) || math.IsInf(sample.allocs, 0) {
			failures = append(failures, fmt.Sprintf("evidence:sample sequence=%d has invalid benchmark metrics", sample.sequence))
		}
		if sample.binary != evidence.headers[sample.side+"_binary_sha256"] {
			failures = append(failures, "evidence:sample binary hash mismatch "+sample.benchmark+"/"+sample.side)
		}
		if sample.commandHash != evidence.headers[sample.side+"_command_sha256"] {
			failures = append(failures, "evidence:sample command hash mismatch "+sample.benchmark+"/"+sample.side)
		}
		if groups[sample.benchmark] == nil {
			groups[sample.benchmark] = make(map[string][]benchSample)
		}
		groups[sample.benchmark][sample.side] = append(groups[sample.benchmark][sample.side], sample)
	}
	wantBenchmarks := []string{"HeadlessProxy", "ImageMutation", "PaneMutation", "ProcessMutation", "StartupProxy", "WindowMutation"}
	actualBenchmarks := make([]string, 0, len(groups))
	for name := range groups {
		actualBenchmarks = append(actualBenchmarks, name)
	}
	sort.Strings(actualBenchmarks)
	if strings.Join(actualBenchmarks, ",") != strings.Join(wantBenchmarks, ",") {
		failures = append(failures, fmt.Sprintf("evidence:benchmark inventory=%v want=%v", actualBenchmarks, wantBenchmarks))
	}
	for _, benchmark := range wantBenchmarks {
		base, candidate := groups[benchmark]["base"], groups[benchmark]["candidate"]
		if len(base) != 10 || len(candidate) != 10 {
			failures = append(failures, fmt.Sprintf("evidence:%s counts base=%d candidate=%d want 10/10", benchmark, len(base), len(candidate)))
			continue
		}
		for round := 1; round <= 10; round++ {
			firstSide := "base"
			if round%2 == 0 {
				firstSide = "candidate"
			}
			positions := []string{}
			for _, sample := range evidence.samples {
				if sample.benchmark == benchmark && sample.round == round {
					positions = append(positions, sample.side)
				}
			}
			secondSide := "candidate"
			if firstSide == "candidate" {
				secondSide = "base"
			}
			if strings.Join(positions, ",") != firstSide+","+secondSide {
				failures = append(failures, fmt.Sprintf("evidence:%s round=%d order=%v violates ABBA", benchmark, round, positions))
			}
		}
		baseNS, candidateNS := sampleMetric(base, func(s benchSample) float64 { return s.ns }), sampleMetric(candidate, func(s benchSample) float64 { return s.ns })
		delta := (candidateNS/baseNS - 1) * 100
		if delta > 3.0 {
			failures = append(failures, fmt.Sprintf("evidence:%s median delta=%.6f%% exceeds 3%%", benchmark, delta))
		}
		baseBytes, candidateBytes := sampleMetric(base, func(s benchSample) float64 { return s.bytes }), sampleMetric(candidate, func(s benchSample) float64 { return s.bytes })
		if candidateBytes > baseBytes {
			failures = append(failures, fmt.Sprintf("evidence:%s bytes median %.3f -> %.3f", benchmark, baseBytes, candidateBytes))
		}
		baseAllocs, candidateAllocs := sampleMetric(base, func(s benchSample) float64 { return s.allocs }), sampleMetric(candidate, func(s benchSample) float64 { return s.allocs })
		if candidateAllocs > baseAllocs {
			failures = append(failures, fmt.Sprintf("evidence:%s allocation median %.3f -> %.3f", benchmark, baseAllocs, candidateAllocs))
		}
	}
	return failures
}

func sampleMetric(samples []benchSample, metric func(benchSample) float64) float64 {
	values := make([]float64, len(samples))
	for index, sample := range samples {
		values[index] = metric(sample)
	}
	sort.Float64s(values)
	return (values[4] + values[5]) / 2
}

func selfTest() []string {
	var failures []string
	valid := append([]string(nil), slice31GPaths...)
	if got := exactSet(valid, slice31GPaths, "synthetic"); len(got) != 0 {
		failures = append(failures, "synthetic exact pre-G set rejected")
	}
	if got := exactSet(valid[1:], slice31GPaths, "synthetic"); len(got) != 1 {
		failures = append(failures, "synthetic missing path not rejected")
	}
	if got := exactSet(append(valid, "outside.go"), slice31GPaths, "synthetic"); len(got) != 1 {
		failures = append(failures, "synthetic outside path not rejected")
	}
	manifestPaths, manifestFailures := readRedesignPathManifest()
	if len(manifestFailures) != 0 || len(manifestPaths) == 0 || len(exactSet(manifestPaths, manifestPaths, "synthetic redesign")) != 0 {
		failures = append(failures, "synthetic exact dirty redesign manifest rejected")
	} else {
		if got := exactSet(manifestPaths[1:], manifestPaths, "synthetic redesign"); len(got) != 1 {
			failures = append(failures, "synthetic dirty redesign missing path not rejected")
		}
		if got := exactSet(append(append([]string(nil), manifestPaths...), "outside.go"), manifestPaths, "synthetic redesign"); len(got) != 1 {
			failures = append(failures, "synthetic dirty redesign outside path not rejected")
		}
	}
	if got := committedRangeSpec(slice31Base, strings.Repeat("b", 40)); got != slice31Base+".."+strings.Repeat("b", 40) || strings.Contains(got, "...") {
		failures = append(failures, "synthetic committed-range diff is not exact two-dot semantics")
	}
	fixture := []byte("schema=slice31-abba-v3\nsample round=1 sequence=1 benchmark=x side=base nonce=n started=2026-01-01T00:00:00Z ended=2026-01-01T00:00:01Z ns_op=1 bytes_op=0 allocs_op=0 binary_sha256=b command_sha256=c\n")
	parsed, got := parseEvidence("fixture", fixture)
	if len(got) != 0 || len(parsed.samples) != 1 {
		failures = append(failures, "synthetic evidence fixture rejected")
	}
	for name, mutated := range map[string][]byte{
		"benchmark tamper": bytes.Replace(fixture, []byte("ns_op=1"), []byte("ns_op=x"), 1),
		"nonce tamper":     append(fixture, []byte("sample round=1 sequence=2 benchmark=x side=base nonce=n started=2026-01-01T00:00:00Z ended=2026-01-01T00:00:01Z ns_op=1 bytes_op=0 allocs_op=0 binary_sha256=b command_sha256=c\n")...),
	} {
		value, parseFailures := parseEvidence("fixture", mutated)
		if name == "benchmark tamper" && len(parseFailures) == 0 {
			failures = append(failures, "synthetic benchmark tamper not rejected")
		}
		if name == "nonce tamper" && len(validateSamples(value)) == 0 {
			failures = append(failures, "synthetic nonce tamper not rejected")
		}
	}
	return failures
}

func parseFields(text string) map[string]string {
	result := make(map[string]string)
	for _, field := range strings.Fields(text) {
		if key, value, ok := strings.Cut(field, "="); ok {
			result[key] = value
		}
	}
	return result
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizedLF(data []byte) string { return strings.ReplaceAll(string(data), "\r\n", "\n") }

func porcelainPaths(text string) []string {
	var result []string
	for _, line := range nonemptyLines(text) {
		if len(line) < 4 || line[2] != ' ' {
			result = append(result, "<malformed:"+line+">")
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.Contains(path, " -> ") {
			path = strings.TrimSpace(path[strings.LastIndex(path, " -> ")+4:])
		}
		if strings.HasPrefix(path, `"`) {
			if unquoted, err := strconv.Unquote(path); err == nil {
				path = unquoted
			}
		}
		result = append(result, filepath.ToSlash(path))
	}
	return result
}

func exactSet(actual, want []string, label string) []string {
	a, w := map[string]bool{}, map[string]bool{}
	for _, path := range actual {
		a[filepath.ToSlash(path)] = true
	}
	for _, path := range want {
		w[filepath.ToSlash(path)] = true
	}
	var failures []string
	for path := range w {
		if !a[path] {
			failures = append(failures, label+" missing "+path)
		}
	}
	for path := range a {
		if !w[path] {
			failures = append(failures, label+" outside allowlist "+path)
		}
	}
	sort.Strings(failures)
	return failures
}

func nonemptyLines(text string) []string {
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func git(args ...string) (string, error)    { return run("git", args...) }
func gitRaw(args ...string) (string, error) { return runRaw("git", args...) }
func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func runRaw(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	return strings.TrimRight(output.String(), "\r\n"), err
}
