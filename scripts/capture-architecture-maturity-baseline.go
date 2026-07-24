//go:build ignore

// capture-architecture-maturity-baseline writes the reproducible Phase 0
// architecture evidence set. Run it from the repository root on clean main.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultOut = "docs/validation/architecture-maturity-phase0"

type command struct {
	label string
	args  []string
}

func main() {
	out := flag.String("out", defaultOut, "evidence output directory")
	count := flag.Int("count", 10, "performance sample count")
	flag.Parse()
	must(os.MkdirAll(*out, 0o755))

	must(capture(filepath.Join(*out, "common-gate.txt"), []command{
		{"maturity gates", []string{"go", "run", "./scripts/check-maturity-gates.go"}},
		{"go test", []string{"go", "test", "./..."}},
		{"vet", []string{"go", "vet", "-unsafeptr=false", "./..."}},
		{"glfw", []string{"go", "test", "-tags", "glfw", "./internal/frontend/glfwgl", "./cmd/cervterm", "-count=1"}},
		{"recovery race", []string{"go", "run", "./scripts/check-phase15-recovery.go", "-race"}},
	}))
	must(capture(filepath.Join(*out, "focused-evidence.txt"), []command{
		{"multi-window geometry/lifecycle trace", []string{"go", "test", "-tags", "glfw", "./internal/frontend/glfwgl", "-run", "^(TestWindowControllerCreateFocusCloseLoopsOwnIndependentBundles|TestCrossWindowActionsUseStableOriginAndPreserveSessions|TestMovePaneToWindowUsesPerPaneMetricsAndStaleSourceIsAtomic|TestRunProjectionCycleClosesExactWindowAndFramesSurvivingSiblings|TestWindowControllerRuntimeCreatePublishActivateAndCloseOrdering)$", "-count=1", "-v"}},
		{"accessibility goldens", []string{"go", "test", "./internal/accessibility", "-count=1"}},
		{"public-output goldens", []string{"go", "test", "./internal/vt", "./internal/mux", "-run", "Public|Projection|Output", "-count=1"}},
	}))
	must(writePackageGraph(filepath.Join(*out, "package-graph.txt")))
	must(writeScoredFiles(filepath.Join(*out, "scored-files.txt")))
	must(run([]string{"go", "run", "./scripts/capture-parity-baseline.go", "-count", fmt.Sprint(*count), "-out", filepath.Join(*out, "performance.txt")}, io.Discard))

	for _, name := range []string{"common-gate.txt", "focused-evidence.txt", "package-graph.txt", "scored-files.txt", "performance.txt"} {
		path := filepath.Join(*out, name)
		data, err := os.ReadFile(path)
		must(err)
		sum := sha256.Sum256(data)
		fmt.Printf("%s  %s\n", hex.EncodeToString(sum[:]), filepath.ToSlash(path))
	}
}

func capture(path string, commands []command) error {
	var out bytes.Buffer
	for _, item := range commands {
		fmt.Fprintf(&out, "## %s\n", item.label)
		if err := run(item.args, &out); err != nil {
			return fmt.Errorf("%s: %w", item.label, err)
		}
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func run(args []string, out io.Writer) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	return cmd.Run()
}

func scoredFiles() ([]string, error) {
	raw, err := exec.Command("git", "ls-files", "-z", "--", "cmd", "internal").Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range bytes.Split(raw, []byte{0}) {
		file := filepath.ToSlash(string(name))
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			return nil, err
		}
		prefix := data
		if len(prefix) > 2048 {
			prefix = prefix[:2048]
		}
		if bytes.Contains(prefix, []byte("Code generated")) && bytes.Contains(prefix, []byte("DO NOT EDIT")) {
			continue
		}
		files = append(files, file)
	}
	sort.Strings(files)
	return files, nil
}

func writeScoredFiles(path string) error {
	files, err := scoredFiles()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(files, "\n")+"\n"), 0o644)
}

func writePackageGraph(path string) error {
	files, err := scoredFiles()
	if err != nil {
		return err
	}
	moduleBytes, err := exec.Command("go", "list", "-m", "-f", "{{.Path}}").Output()
	if err != nil {
		return err
	}
	module := strings.TrimSpace(string(moduleBytes))
	members := make(map[string]bool)
	type edge struct{ from, to string }
	var parsedEdges []edge
	fileset := token.NewFileSet()
	for _, file := range files {
		from := module + "/" + filepath.ToSlash(filepath.Dir(file))
		members[from] = true
		parsed, err := parser.ParseFile(fileset, filepath.FromSlash(file), nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", file, err)
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("import in %s: %w", file, err)
			}
			if imported == module || strings.HasPrefix(imported, module+"/") {
				parsedEdges = append(parsedEdges, edge{from: from, to: imported})
			}
		}
	}
	adj := make(map[string][]string)
	var edges []string
	for _, edge := range parsedEdges {
		if !members[edge.to] {
			return fmt.Errorf("local import %s from %s has no scored production package", edge.to, edge.from)
		}
		adj[edge.from] = append(adj[edge.from], edge.to)
		edges = append(edges, edge.from+" -> "+edge.to)
	}
	sort.Strings(edges)
	edges = compact(edges)
	cycles := stronglyConnectedCycles(members, adj)
	var out strings.Builder
	fmt.Fprintf(&out, "scope=all-tracked-production-go packages=%d edges=%d cycles=%d\n", len(members), len(edges), cycles)
	for _, edge := range edges {
		fmt.Fprintln(&out, edge)
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}

func compact(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func stronglyConnectedCycles(nodes map[string]bool, adj map[string][]string) int {
	index := 0
	indices := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	cycles := 0
	var visit func(string)
	visit = func(node string) {
		indices[node], low[node] = index, index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adj[node] {
			if _, seen := indices[next]; !seen {
				visit(next)
				low[node] = min(low[node], low[next])
			} else if onStack[next] {
				low[node] = min(low[node], indices[next])
			}
		}
		if low[node] != indices[node] {
			return
		}
		size := 0
		self := false
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			size++
			if last == node {
				for _, next := range adj[last] {
					self = self || next == last
				}
				break
			}
		}
		if size > 1 || self {
			cycles++
		}
	}
	var ordered []string
	for node := range nodes {
		ordered = append(ordered, node)
	}
	sort.Strings(ordered)
	for _, node := range ordered {
		if _, seen := indices[node]; !seen {
			visit(node)
		}
	}
	return cycles
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
