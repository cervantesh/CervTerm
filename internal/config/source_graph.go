package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const (
	DefaultMaxIncludeDepth     = 16
	DefaultMaxDeclarativeFiles = 64
	DefaultMaxSourceBytes      = 1 << 20
	DefaultMaxAggregateBytes   = 8 << 20
)

type SourceGraphOptions struct {
	MaxIncludeDepth     int
	MaxDeclarativeFiles int
	MaxSourceBytes      int64
	MaxAggregateBytes   int64
	StageDirectory      string
}

func DefaultSourceGraphOptions() SourceGraphOptions {
	return SourceGraphOptions{
		MaxIncludeDepth: DefaultMaxIncludeDepth, MaxDeclarativeFiles: DefaultMaxDeclarativeFiles,
		MaxSourceBytes: DefaultMaxSourceBytes, MaxAggregateBytes: DefaultMaxAggregateBytes,
	}
}

type SourceNode struct {
	RequestedPath string
	CanonicalPath string
	Size          int64
	Document      Document
	Teal          *StagedTeal
}

type SourceEdge struct {
	From      string
	To        string
	Requested string
}

type SourceGraph struct {
	Primary      string
	Sources      []SourceNode // deterministic depth-first post-order
	Edges        []SourceEdge
	Dependencies []SourceDependency
	StagedTeal   []StagedTeal
	stageRoot    string
	ownsStage    bool
	state        *lua.LState
}

func (g *SourceGraph) Close() error {
	if g == nil || !g.ownsStage || g.stageRoot == "" {
		return nil
	}
	err := os.RemoveAll(g.stageRoot)
	g.stageRoot, g.ownsStage = "", false
	return err
}

type sourceRecord struct {
	node   SourceNode
	info   os.FileInfo
	active bool
	done   bool
}

type sourceGraphBuilder struct {
	state       *lua.LState
	options     SourceGraphOptions
	graph       *SourceGraph
	capture     *dependencyCapture
	records     map[string]*sourceRecord
	stack       []string
	aggregate   int64
	strict      bool
	explicit    map[string]string
	reservedLua map[string]string
}

var sourceGraphConsumedKey = &lua.LUserData{Value: "cervterm.config.source_graph_consumed"}

var graphDocumentFields = map[string]fieldSchema{
	"includes": {name: "includes", kind: KindStringList},
}

// BuildSourceGraph consumes a fresh caller-owned Lua candidate state to evaluate
// the primary and all declarative includes exactly once. The state must be discarded
// if this function returns an error and must not be passed to BuildSourceGraph again.
// The graph builds order/dependency evidence only; it does not merge documents or
// mutate active application configuration.
func BuildSourceGraph(state *lua.LState, primary string, options SourceGraphOptions) (*SourceGraph, error) {
	if state == nil {
		return nil, fmt.Errorf("build config source graph: nil Lua state")
	}
	if state.G.Registry.RawGet(sourceGraphConsumedKey) != lua.LNil {
		return nil, fmt.Errorf("build config source graph: Lua candidate state was already consumed")
	}
	state.G.Registry.RawSet(sourceGraphConsumedKey, lua.LTrue)
	options = normalizeSourceGraphOptions(options)
	stageRoot, ownsStage, err := sourceGraphStageRoot(options.StageDirectory)
	if err != nil {
		return nil, err
	}
	graph := &SourceGraph{stageRoot: stageRoot, ownsStage: ownsStage, state: state}
	capture, err := installDependencyCapture(state)
	if err != nil {
		_ = graph.Close()
		return nil, err
	}
	builder := &sourceGraphBuilder{
		state: state, options: options, graph: graph, capture: capture,
		records: make(map[string]*sourceRecord), explicit: make(map[string]string), reservedLua: make(map[string]string),
	}
	defer capture.restore()
	canonical, err := builder.build(primary, "", 0, true)
	if err != nil {
		_ = graph.Close()
		return nil, err
	}
	graph.Primary = canonical
	graph.Dependencies = capture.list()
	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].From == graph.Edges[j].From {
			if graph.Edges[i].To == graph.Edges[j].To {
				return graph.Edges[i].Requested < graph.Edges[j].Requested
			}
			return graph.Edges[i].To < graph.Edges[j].To
		}
		return graph.Edges[i].From < graph.Edges[j].From
	})
	return graph, nil
}

func (b *sourceGraphBuilder) build(requested, parent string, depth int, primary bool) (string, error) {
	if depth > b.options.MaxIncludeDepth {
		return "", fmt.Errorf("config include depth %d exceeds limit %d at %q", depth, b.options.MaxIncludeDepth, requested)
	}
	if looksRemotePath(requested) {
		return "", fmt.Errorf("remote config include %q is not supported", requested)
	}
	resolved := requested
	if parent != "" && !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(parent), resolved)
	}
	canonical, info, err := canonicalLocalFile(resolved)
	if err != nil {
		if parent != "" {
			return "", fmt.Errorf("config include %q declared by %q: %w", requested, parent, err)
		}
		return "", err
	}
	extension := strings.ToLower(filepath.Ext(canonical))
	if extension != ".lua" && extension != ".tl" {
		return "", fmt.Errorf("config source %q must use .lua or .tl", canonical)
	}
	identity := b.identityFor(canonical, info)
	if owner, reserved := b.reservedLua[identity]; reserved && canonicalIdentity(owner) != canonicalIdentity(canonical) {
		return "", fmt.Errorf("config source %q collides with generated Teal output reserved by %q", canonical, owner)
	}
	if record := b.records[identity]; record != nil {
		if parent != "" {
			b.graph.Edges = append(b.graph.Edges, SourceEdge{From: parent, To: record.node.CanonicalPath, Requested: requested})
		}
		if record.active {
			return "", b.cycleError(record.node.CanonicalPath)
		}
		return record.node.CanonicalPath, nil
	}
	if len(b.records) >= b.options.MaxDeclarativeFiles {
		return "", fmt.Errorf("config source count exceeds limit %d at %q", b.options.MaxDeclarativeFiles, canonical)
	}
	if info.Size() > b.options.MaxSourceBytes {
		return "", fmt.Errorf("config source %q size %d exceeds limit %d", canonical, info.Size(), b.options.MaxSourceBytes)
	}
	if b.aggregate+info.Size() > b.options.MaxAggregateBytes {
		return "", fmt.Errorf("config aggregate source bytes exceed limit %d at %q", b.options.MaxAggregateBytes, canonical)
	}
	b.aggregate += info.Size()
	b.explicit[identity] = canonical
	record := &sourceRecord{node: SourceNode{RequestedPath: requested, CanonicalPath: canonical, Size: info.Size()}, info: info, active: true}
	b.records[identity] = record
	b.stack = append(b.stack, identity)
	if parent != "" {
		b.graph.Edges = append(b.graph.Edges, SourceEdge{From: parent, To: canonical, Requested: requested})
	}
	evaluationPath := canonical
	if extension == ".tl" {
		staged, err := stageTealSource(canonical, b.graph.stageRoot)
		if err != nil {
			return "", err
		}
		publishedIdentity := canonicalIdentity(staged.PublishedLua)
		if explicit, exists := b.explicit[publishedIdentity]; exists && canonicalIdentity(explicit) != canonicalIdentity(canonical) {
			return "", fmt.Errorf("Teal source %q generated output collides with explicit source %q", canonical, explicit)
		}
		b.reservedLua[publishedIdentity] = canonical
		record.node.Teal = &staged
		b.graph.StagedTeal = append(b.graph.StagedTeal, staged)
		evaluationPath = staged.EvaluationLua
	}
	restoreGuard := setDeclarativeIncludeGuard(b.state, !primary)
	root, err := evaluateGraphSource(b.state, canonical, evaluationPath)
	restoreGuard()
	if err != nil {
		return "", err
	}
	document, err := decodeCompositionDocument(canonical, root, graphDocumentFields)
	if err != nil {
		return "", err
	}
	record.node.Document = document
	if primary {
		if document.AuthoredVersion != 2 {
			return "", fmt.Errorf("config source graph requires config_version = 2 in primary %q", canonical)
		}
		b.strict = true
	}
	if b.strict {
		if err := b.capture.verifyStrict(); err != nil {
			return "", fmt.Errorf("%s: %w", canonical, err)
		}
	}
	includes := stringListField(root, "includes", nil)
	for _, include := range includes {
		if _, err := b.build(include, canonical, depth+1, false); err != nil {
			return "", err
		}
	}
	record.active, record.done = false, true
	b.stack = b.stack[:len(b.stack)-1]
	b.graph.Sources = append(b.graph.Sources, record.node)
	return canonical, nil
}

func (b *sourceGraphBuilder) identityFor(canonical string, info os.FileInfo) string {
	for identity, record := range b.records {
		if os.SameFile(info, record.info) {
			return identity
		}
	}
	return canonicalIdentity(canonical)
}

func (b *sourceGraphBuilder) cycleError(canonical string) error {
	start := 0
	identity := canonicalIdentity(canonical)
	for i, item := range b.stack {
		if item == identity {
			start = i
			break
		}
	}
	paths := make([]string, 0, len(b.stack)-start+1)
	for _, item := range b.stack[start:] {
		if record := b.records[item]; record != nil {
			paths = append(paths, record.node.CanonicalPath)
		}
	}
	paths = append(paths, canonical)
	return fmt.Errorf("config include cycle: %s", strings.Join(paths, " -> "))
}

func evaluateGraphSource(state *lua.LState, sourcePath, evaluationPath string) (*lua.LTable, error) {
	function, err := state.LoadFile(evaluationPath)
	if err != nil {
		return nil, fmt.Errorf("evaluate config source %q via %q: %w", sourcePath, evaluationPath, err)
	}
	if err := state.CallByParam(lua.P{Fn: function, NRet: 1, Protect: true}); err != nil {
		return nil, fmt.Errorf("evaluate config source %q via %q: %w", sourcePath, evaluationPath, err)
	}
	value := state.Get(-1)
	state.Pop(1)
	root, ok := value.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("config source %q must return a table, got %s", sourcePath, value.Type().String())
	}
	return root, nil
}

func canonicalLocalFile(path string) (string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve config source %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("resolve config source %q: %w", path, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("resolve canonical config source %q: %w", path, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("stat config source %q: %w", canonical, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("config source %q is not a regular file", canonical)
	}
	return filepath.Clean(canonical), info, nil
}

func canonicalIdentity(path string) string {
	identity := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	return identity
}

func looksRemotePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.Contains(lower, "://") || strings.HasPrefix(lower, "file:")
}

func normalizeSourceGraphOptions(options SourceGraphOptions) SourceGraphOptions {
	defaults := DefaultSourceGraphOptions()
	if options.MaxIncludeDepth <= 0 {
		options.MaxIncludeDepth = defaults.MaxIncludeDepth
	}
	if options.MaxDeclarativeFiles <= 0 {
		options.MaxDeclarativeFiles = defaults.MaxDeclarativeFiles
	}
	if options.MaxSourceBytes <= 0 {
		options.MaxSourceBytes = defaults.MaxSourceBytes
	}
	if options.MaxAggregateBytes <= 0 {
		options.MaxAggregateBytes = defaults.MaxAggregateBytes
	}
	return options
}

func sourceGraphStageRoot(configured string) (string, bool, error) {
	parent := ""
	if configured != "" {
		if err := os.MkdirAll(configured, 0o700); err != nil {
			return "", false, fmt.Errorf("create config staging directory: %w", err)
		}
		parent = configured
	}
	root, err := os.MkdirTemp(parent, "cervterm-config-candidate-*")
	if err != nil {
		return "", false, fmt.Errorf("create config staging directory: %w", err)
	}
	return root, true, nil
}
