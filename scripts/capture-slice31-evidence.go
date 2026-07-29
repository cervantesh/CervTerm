//go:build ignore

package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	baseCommit        = "9fe0bd0287ed402fe90b58a5fba7d6413897c1af"
	candidateCommit   = "27070f32395be7803b6fc11388a08a5c32d3d3a1"
	evidenceDirectory = "docs/validation/architecture-maturity-slice-3.1"
	benchTime         = "2s"
)

var baseHarness = `package mux

import (
    "errors"
    "testing"
    "cervterm/internal/termimage"
)

var l302EvidenceEvents []Event
var l302EvidenceLayout Layout
var l302EvidenceFrameChecksum uint64
var l302EvidenceSpawnErr = errors.New("evidence spawn unavailable")

func consumeL302EvidencePaneView(view PaneView) uint64 {
    sum := uint64(view.Snapshot.Cols + view.Snapshot.Rows + view.ScrollbackLines)
    for _, cell := range view.Snapshot.Cells { sum += uint64(cell.Rune) + uint64(cell.HyperlinkID) }
    return sum
}

func newL302EvidenceMux(b *testing.B) *Mux {
    b.Helper()
    m := New(&fakeFactory{err: l302EvidenceSpawnErr}, Options{IngressCapacity: 2048})
    if _, pane, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(err, l302EvidenceSpawnErr) || pane != 1 { b.Fatalf("bootstrap pane=%d err=%v", pane, err) }
    b.Cleanup(func(){ _ = m.Shutdown() })
    return m
}

func expandL302EvidenceRegistry(b *testing.B, m *Mux) {
	b.Helper(); pane,ok:=m.sessions.lookup(1); if !ok { b.Fatal("benchmark pane unavailable") }; m.sessions.mu.Lock(); for id:=PaneID(2); id<=512; id++ { m.sessions.panes[id]=pane }; m.sessions.mu.Unlock(); b.Cleanup(func(){ m.sessions.mu.Lock(); for id:=PaneID(2); id<=512; id++ { delete(m.sessions.panes,id) }; m.sessions.mu.Unlock() })
}

func BenchmarkL302EvidenceProcessMutation(b *testing.B) {
    m := newL302EvidenceMux(b); b.ReportAllocs(); b.ResetTimer()
    for i:=0; i<b.N; i++ { for record:=0; record<1024; record++ { m.sessions.incoming <- ingressRecord{} }; l302EvidenceEvents=m.Drain(1024) }
}
func BenchmarkL302EvidenceWindowMutation(b *testing.B) {
    m := newL302EvidenceMux(b); b.ReportAllocs(); b.ResetTimer()
    for i:=0; i<b.N; i++ { events, err := m.Resize(PixelRect{Width: 800 + (i & 1), Height:480}, CellMetrics{CellWidth:8, CellHeight:16}); if err != nil { b.Fatal(err) }; l302EvidenceEvents=events }
}
func BenchmarkL302EvidencePaneMutation(b *testing.B) {
    m := newL302EvidenceMux(b); b.ReportAllocs(); b.ResetTimer()
    for i:=0; i<b.N; i++ { events, err := m.FeedFallback(1, []byte("x")); if err != nil { b.Fatal(err) }; l302EvidenceEvents=events }
}
func seedL302EvidenceStore(b *testing.B, store *termimage.Store) {
    b.Helper()
    for image:=uint32(1); image<=256; image++ { candidate, err := store.NewDecodedCandidate(termimage.ImageID(image),1,1); if err != nil { b.Fatal(err) }; prepared, _, err := store.PrepareCandidate(candidate); if err != nil { b.Fatal(err) }; store.PublishPrepared(prepared); prepared.Finalize() }
    for placement:=0; placement<1024; placement++ { if _,err:=store.ReservePlacements(1); err != nil { b.Fatal(err) } }
}
func BenchmarkL302EvidenceImageMutation(b *testing.B) {
    store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits()); b.Cleanup(store.Close); seedL302EvidenceStore(b, store); refs:=store.ResourceRefs(); b.ReportAllocs(); b.ResetTimer()
    for i:=0; i<b.N; i++ { prepared, err := store.PrepareResourceRemoval(refs); if err != nil { b.Fatal(err) }; prepared.Abort() }
}
func BenchmarkL302EvidenceStartupProxy(b *testing.B) {
    b.ReportAllocs()
    for i:=0; i<b.N; i++ { m:=New(&fakeFactory{}, Options{IngressCapacity:8}); b.StopTimer(); err:=m.Shutdown(); b.StartTimer(); if err != nil { b.Fatal(err) } }
}
func BenchmarkL302EvidenceHeadlessProxy(b *testing.B) {
    m:=newL302EvidenceMux(b); b.ReportAllocs(); b.ResetTimer()
    for i:=0; i<b.N; i++ { layout,err:=m.Layout(); if err != nil { b.Fatal(err) }; view,ok:=m.PaneView(1); if !ok { b.Fatal("pane view unavailable") }; l302EvidenceLayout=layout; l302EvidenceFrameChecksum=consumeL302EvidencePaneView(view) }
}
`

type benchmarkSpec struct{ label, base, candidate string }

var benchmarks = []benchmarkSpec{
	{"ProcessMutation", "BenchmarkL302EvidenceProcessMutation", "BenchmarkL302ProcessMutationCandidate"},
	{"WindowMutation", "BenchmarkL302EvidenceWindowMutation", "BenchmarkL302WindowMutationCandidate"},
	{"PaneMutation", "BenchmarkL302EvidencePaneMutation", "BenchmarkL302PaneMutationCandidate"},
	{"ImageMutation", "BenchmarkL302EvidenceImageMutation", "BenchmarkL302ImageMutationCandidate"},
	{"StartupProxy", "BenchmarkL302EvidenceStartupProxy", "BenchmarkL302StartupProxyCandidate"},
	{"HeadlessProxy", "BenchmarkL302EvidenceHeadlessProxy", "BenchmarkL302HeadlessFrameCandidate"},
}

var benchmarkLine = regexp.MustCompile(`(?m)^Benchmark\S+\s+\d+\s+([0-9.]+) ns/op\s+([0-9.]+) B/op\s+([0-9.]+) allocs/op\s*$`)

func main() {
	root, err := os.Getwd()
	must(err)
	captureStarted := time.Now().UTC()
	if _, err := commandOutput(root, "git", "cat-file", "-e", candidateCommit+"^{commit}"); err != nil {
		panic("candidate commit unavailable: " + err.Error())
	}
	candidateHead := candidateCommit
	temp, err := os.MkdirTemp("", "cervterm-slice31-evidence-")
	must(err)
	defer os.RemoveAll(temp)
	baseTree := filepath.Join(temp, "base")
	candidateTree := filepath.Join(temp, "candidate")
	must(runAt(root, "git", "worktree", "add", "--detach", baseTree, baseCommit))
	defer runAt(root, "git", "worktree", "remove", "--force", baseTree)
	must(runAt(root, "git", "worktree", "add", "--detach", candidateTree, candidateCommit))
	defer runAt(root, "git", "worktree", "remove", "--force", candidateTree)
	must(os.WriteFile(filepath.Join(baseTree, "internal", "mux", "l302_evidence_test.go"), []byte(baseHarness), 0o644))
	candidateHarnessData, err := os.ReadFile(filepath.Join(root, "internal", "mux", "owner_characterization_test.go"))
	must(err)
	must(os.WriteFile(filepath.Join(candidateTree, "internal", "mux", "owner_characterization_test.go"), candidateHarnessData, 0o644))

	baseBinary := filepath.Join(temp, "base-mux.test.exe")
	candidateBinary := filepath.Join(temp, "candidate-mux.test.exe")
	must(runAt(baseTree, "go", "test", "-c", "-trimpath", "-o", baseBinary, "./internal/mux"))
	must(runAt(candidateTree, "go", "test", "-c", "-trimpath", "-o", candidateBinary, "./internal/mux"))
	baseBinaryHash := fileHash(baseBinary)
	candidateBinaryHash := fileHash(candidateBinary)
	baseSourceHash, err := commitSourceHash(root, baseCommit)
	must(err)
	candidateSourceHash, err := commitSourceHash(root, candidateCommit)
	must(err)
	harnessHash := bytesHash([]byte(strings.ReplaceAll(baseHarness, "\r\n", "\n")))
	candidateHarnessHash := bytesHash([]byte(strings.ReplaceAll(string(candidateHarnessData), "\r\n", "\n")))
	baseCommand := "base-mux.test+-test.run=^$+-test.bench=^BENCHMARK$+-test.benchtime=" + benchTime + "+-test.count=1+-test.cpu=1+-test.benchmem"
	candidateCommand := "candidate-mux.test+-test.run=^$+-test.bench=^BENCHMARK$+-test.benchtime=" + benchTime + "+-test.count=1+-test.cpu=1+-test.benchmem"
	baseCommandHash, candidateCommandHash := bytesHash([]byte(baseCommand)), bytesHash([]byte(candidateCommand))

	warmupProcesses := 0
	for _, benchmark := range benchmarks {
		for _, side := range []string{"base", "candidate"} {
			binary, name := baseBinary, benchmark.base
			if side == "candidate" {
				binary, name = candidateBinary, benchmark.candidate
			}
			warmupProcesses++
			output, warmupErr := commandOutput(root, binary, "-test.run=^$", "-test.bench=^"+name+"$", "-test.benchtime="+benchTime, "-test.count=1", "-test.cpu=1", "-test.benchmem")
			if warmupErr != nil {
				panic(fmt.Sprintf("%s/%s warmup: %v\n%s", benchmark.label, side, warmupErr, output))
			}
			if match := benchmarkLine.FindStringSubmatch(strings.ReplaceAll(output, "\r\n", "\n")); len(match) != 4 {
				panic("cannot parse warmup benchmark output:\n" + output)
			}
		}
	}

	var samples strings.Builder
	sequence := 0
	for _, benchmark := range benchmarks {
		for round := 1; round <= 10; round++ {
			sides := []string{"base", "candidate"}
			if round%2 == 0 {
				sides[0], sides[1] = sides[1], sides[0]
			}
			for _, side := range sides {
				sequence++
				binary, name, binaryHash, commandHash := baseBinary, benchmark.base, baseBinaryHash, baseCommandHash
				if side == "candidate" {
					binary, name, binaryHash, commandHash = candidateBinary, benchmark.candidate, candidateBinaryHash, candidateCommandHash
				}
				nonce := randomNonce()
				started := time.Now().UTC()
				output, runErr := commandOutput(root, binary, "-test.run=^$", "-test.bench=^"+name+"$", "-test.benchtime="+benchTime, "-test.count=1", "-test.cpu=1", "-test.benchmem")
				ended := time.Now().UTC()
				if runErr != nil {
					panic(fmt.Sprintf("%s/%s round %d: %v\n%s", benchmark.label, side, round, runErr, output))
				}
				match := benchmarkLine.FindStringSubmatch(strings.ReplaceAll(output, "\r\n", "\n"))
				if len(match) != 4 {
					panic("cannot parse benchmark output:\n" + output)
				}
				fmt.Fprintf(&samples, "sample round=%02d sequence=%03d benchmark=%s function=%s side=%s nonce=%s started=%s ended=%s ns_op=%s bytes_op=%s allocs_op=%s binary_sha256=%s command_sha256=%s\n", round, sequence, benchmark.label, name, side, nonce, started.Format(time.RFC3339Nano), ended.Format(time.RFC3339Nano), match[1], match[2], match[3], binaryHash, commandHash)
			}
		}
	}
	captureFinished := time.Now().UTC()
	header := fmt.Sprintf("schema=slice31-abba-v4\nsamples_per_side=10\norder=ABBAx5\nphysical_processes=120\nthreshold_percent=3.000000\nwarmup_processes=12\nwarmup_per_side=1\nwarmup_benchtime=%s\nwarmup_order=base,candidate\nbase_identity=%s\nbase_source_sha256=%s\ncandidate_identity=detached-W+harness-overlay\ncandidate_head=%s\ncandidate_source_sha256=%s\nbase_binary_sha256=%s\ncandidate_binary_sha256=%s\nbase_harness_sha256=%s\ncandidate_harness_sha256=%s\nbase_command=%s\ncandidate_command=%s\nbase_command_sha256=%s\ncandidate_command_sha256=%s\nplatform_runtime=%s/%s\ncapture_started=%s\ncapture_finished=%s\n\n", benchTime, baseCommit, baseSourceHash, candidateHead, candidateSourceHash, baseBinaryHash, candidateBinaryHash, harnessHash, candidateHarnessHash, baseCommand, candidateCommand, baseCommandHash, candidateCommandHash, runtimeGOOS(), runtimeGOARCH(), captureStarted.Format(time.RFC3339Nano), captureFinished.Format(time.RFC3339Nano))
	must(os.WriteFile(filepath.Join(root, evidenceDirectory, "benchmarks-candidate.txt"), []byte(header+samples.String()), 0o644))
	baseMetadata := fmt.Sprintf("schema=slice31-source-bound-base-v3\nidentity=%s\nnormalized_lf_source_sha256=%s\nbinary_sha256=%s\nbase_harness_sha256=%s\nlogical_command=%s\ncommand_sha256=%s\nplatform_compile=%s/%s\nplatform_runtime=%s/%s\n", baseCommit, baseSourceHash, baseBinaryHash, harnessHash, baseCommand, baseCommandHash, runtimeGOOS(), runtimeGOARCH(), runtimeGOOS(), runtimeGOARCH())
	must(os.WriteFile(filepath.Join(root, evidenceDirectory, "benchmarks-base.txt"), []byte(baseMetadata), 0o644))
	hashes := fmt.Sprintf("schema=slice31-source-binary-manifest-v4\nbase_identity=%s\nbase_normalized_lf_sha256=%s\nbase_binary_sha256=%s\nbase_harness_sha256=%s\ncandidate_identity=detached-W+harness-overlay\ncandidate_head=%s\ncandidate_normalized_lf_sha256=%s\ncandidate_binary_sha256=%s\ncandidate_harness_sha256=%s\nbase_command=%s\nbase_command_sha256=%s\ncandidate_command=%s\ncandidate_command_sha256=%s\n", baseCommit, baseSourceHash, baseBinaryHash, harnessHash, candidateHead, candidateSourceHash, candidateBinaryHash, candidateHarnessHash, baseCommand, baseCommandHash, candidateCommand, candidateCommandHash)
	must(os.WriteFile(filepath.Join(root, evidenceDirectory, "source-binary-hashes.txt"), []byte(hashes), 0o644))
	fmt.Printf("captured %d physical benchmark processes after %d warmup processes; base=%s candidate=%s head=%s source=%s\n", sequence, warmupProcesses, baseBinaryHash, candidateBinaryHash, candidateHead, candidateSourceHash)
}

func randomNonce() string {
	data := make([]byte, 16)
	_, err := rand.Read(data)
	must(err)
	return hex.EncodeToString(data)
}
func bytesHash(data []byte) string { sum := sha256.Sum256(data); return fmt.Sprintf("%x", sum) }
func fileHash(path string) string  { data, err := os.ReadFile(path); must(err); return bytesHash(data) }
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func runAt(dir, name string, args ...string) error {
	output, err := commandOutput(dir, name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, output)
	}
	return nil
}
func commandOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func currentSourceHash(root string) (string, error) {
	var paths []string
	for _, relative := range []string{"internal/mux", "internal/core", "internal/termimage", "internal/frontend/glfwgl", "internal/ownerthread"} {
		err := filepath.WalkDir(filepath.Join(root, relative), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					return relErr
				}
				paths = append(paths, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00%s\n", path, strings.ReplaceAll(string(data), "\r\n", "\n"))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
func commitSourceHash(root, commit string) (string, error) {
	output, err := commandOutput(root, "git", "ls-tree", "-r", "--name-only", commit, "--", "internal/mux", "internal/core", "internal/termimage", "internal/frontend/glfwgl", "internal/ownerthread")
	if err != nil {
		return "", err
	}
	var paths []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, showErr := commandOutput(root, "git", "show", commit+":"+path)
		if showErr != nil {
			return "", showErr
		}
		fmt.Fprintf(hash, "%s\x00%s\n", path, strings.ReplaceAll(data, "\r\n", "\n"))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
func runtimeGOOS() string {
	output, _ := commandOutput("", "go", "env", "GOOS")
	return strings.TrimSpace(output)
}
func runtimeGOARCH() string {
	output, _ := commandOutput("", "go", "env", "GOARCH")
	return strings.TrimSpace(output)
}

var _ = strconv.IntSize
