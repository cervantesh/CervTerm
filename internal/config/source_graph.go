package config

import (
	"bytes"
	"crypto/sha256"
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
	SelectedPath  string
	SelectedPaths []string
	Hash          [sha256.Size]byte
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

// PrimaryNode returns the evaluated primary source document.
func (g *SourceGraph) PrimaryNode() (SourceNode, bool) {
	if g == nil {
		return SourceNode{}, false
	}
	for _, source := range g.Sources {
		if source.CanonicalPath == g.Primary {
			return source, true
		}
	}
	return SourceNode{}, false
}

type sourceRecord struct {
	node   SourceNode
	info   os.FileInfo
	active bool
	done   bool
}

type sourceGraphBuilder struct {
	state              *lua.LState
	options            SourceGraphOptions
	graph              *SourceGraph
	capture            *dependencyCapture
	records            map[string]*sourceRecord
	stack              []string
	aggregate          int64
	strict             bool
	allowLegacyPrimary bool
	explicit           map[string]string
	reservedLua        map[string]string
}

var sourceGraphConsumedKey = &lua.LUserData{Value: "cervterm.config.source_graph_consumed"}

var graphDocumentFields = map[string]fieldSchema{
	"includes":            {name: "includes", kind: KindStringList},
	"default_environment": {name: "default_environment", kind: KindString},
	"default_profile":     {name: "default_profile", kind: KindString},
	"environments":        {name: "environments", kind: KindDocumentMap},
	"profiles":            {name: "profiles", kind: KindDocumentMap},
}

// BuildSourceGraph consumes a fresh caller-owned Lua candidate state to evaluate
// the primary and all declarative includes exactly once. The state must be discarded
// if this function returns an error and must not be passed to BuildSourceGraph again.
// The graph builds order/dependency evidence only; it does not merge documents or
// mutate active application configuration.
func BuildSourceGraph(state *lua.LState, primary string, options SourceGraphOptions) (*SourceGraph, error) {
	return buildSourceGraph(state, primary, options, false)
}

// BuildVersionedSourceGraph admits an authored-v1 primary solely so the
// application loader can dispatch after one evaluation. Includes remain v2-only.
func BuildVersionedSourceGraph(state *lua.LState, primary string, options SourceGraphOptions) (*SourceGraph, error) {
	return buildSourceGraph(state, primary, options, true)
}

func buildSourceGraph(state *lua.LState, primary string, options SourceGraphOptions, allowLegacyPrimary bool) (*SourceGraph, error) {
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
		state: state, options: options, graph: graph, capture: capture, allowLegacyPrimary: allowLegacyPrimary,
		records: make(map[string]*sourceRecord), explicit: make(map[string]string), reservedLua: make(map[string]string),
	}
	legacyCapture := false
	defer func() {
		if legacyCapture {
			capture.restoreLegacy()
		} else {
			capture.restore()
		}
	}()
	canonical, err := builder.build(primary, "", 0, true)
	if err != nil {
		_ = graph.Close()
		return nil, err
	}
	graph.Primary = canonical
	if primaryNode, ok := graph.PrimaryNode(); ok {
		legacyCapture = primaryNode.Document.AuthoredVersion == 1
	}
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
	selectedPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve config source %q: %w", resolved, err)
	}
	selectedPath = filepath.Clean(selectedPath)
	loadSourcePath := canonical
	if primary && b.allowLegacyPrimary {
		loadSourcePath = selectedPath
	}
	extension := strings.ToLower(filepath.Ext(loadSourcePath))
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
		record.node.SelectedPaths = appendUniquePath(record.node.SelectedPaths, selectedPath)
		if record.done {
			for index := range b.graph.Sources {
				if b.graph.Sources[index].CanonicalPath == record.node.CanonicalPath {
					b.graph.Sources[index].SelectedPaths = append([]string(nil), record.node.SelectedPaths...)
					break
				}
			}
		}
		if record.active {
			return "", b.cycleError(record.node.CanonicalPath)
		}
		return record.node.CanonicalPath, nil
	}
	if len(b.records) >= b.options.MaxDeclarativeFiles {
		return "", fmt.Errorf("config source count exceeds limit %d at %q", b.options.MaxDeclarativeFiles, canonical)
	}
	deferPrimaryLimits := primary && b.allowLegacyPrimary
	if !deferPrimaryLimits && info.Size() > b.options.MaxSourceBytes {
		return "", fmt.Errorf("config source %q size %d exceeds limit %d", canonical, info.Size(), b.options.MaxSourceBytes)
	}
	if !deferPrimaryLimits && b.aggregate+info.Size() > b.options.MaxAggregateBytes {
		return "", fmt.Errorf("config aggregate source bytes exceed limit %d at %q", b.options.MaxAggregateBytes, canonical)
	}
	b.aggregate += info.Size()
	b.explicit[identity] = canonical
	sourceContent, err := os.ReadFile(loadSourcePath)
	if err != nil {
		return "", fmt.Errorf("read config source %q: %w", loadSourcePath, err)
	}
	record := &sourceRecord{node: SourceNode{RequestedPath: requested, CanonicalPath: canonical, SelectedPath: selectedPath, SelectedPaths: []string{selectedPath}, Hash: SourceWatchHash(canonical, sourceContent), Size: info.Size()}, info: info, active: true}
	b.records[identity] = record
	b.stack = append(b.stack, identity)
	if parent != "" {
		b.graph.Edges = append(b.graph.Edges, SourceEdge{From: parent, To: canonical, Requested: requested})
	}
	evaluationPath := loadSourcePath
	evaluationContent := sourceContent
	if extension == ".tl" {
		staged, err := stageTealSource(loadSourcePath, b.graph.stageRoot)
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
		evaluationContent, err = os.ReadFile(evaluationPath)
		if err != nil {
			return "", fmt.Errorf("read staged Teal output %q: %w", evaluationPath, err)
		}
	}
	restoreGuard := setDeclarativeIncludeGuard(b.state, !primary)
	displayPath := evaluationPath
	if record.node.Teal != nil {
		displayPath = record.node.Teal.PublishedLua
	}
	sourceLabel := canonical
	if primary && b.allowLegacyPrimary {
		sourceLabel = loadSourcePath
	}
	root, err := evaluateGraphSource(b.state, sourceLabel, evaluationPath, displayPath, evaluationContent)
	restoreGuard()
	if err != nil {
		return "", err
	}
	document, err := decodeCompositionDocument(sourceLabel, root, graphDocumentFields)
	if err != nil {
		return "", err
	}
	record.node.Document = document
	if primary {
		if document.AuthoredVersion != 2 && !b.allowLegacyPrimary {
			return "", fmt.Errorf("config source graph requires config_version = 2 in primary %q", canonical)
		}
		b.strict = document.AuthoredVersion == 2
		if document.AuthoredVersion == 2 {
			if info.Size() > b.options.MaxSourceBytes {
				return "", fmt.Errorf("config source %q size %d exceeds limit %d", canonical, info.Size(), b.options.MaxSourceBytes)
			}
			if b.aggregate > b.options.MaxAggregateBytes {
				return "", fmt.Errorf("config aggregate source bytes exceed limit %d at %q", b.options.MaxAggregateBytes, canonical)
			}
		}
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

func evaluateGraphSource(state *lua.LState, sourcePath, evaluationPath, displayPath string, content []byte) (*lua.LTable, error) {
	function, err := state.Load(bytes.NewReader(content), "@"+displayPath)
	if err != nil {
		return nil, fmt.Errorf("evaluate config source %q via %q: %w", sourcePath, evaluationPath, err)
	}
	stackBase := state.GetTop()
	if err := state.CallByParam(lua.P{Fn: function, NRet: lua.MultRet, Protect: true}); err != nil {
		return nil, fmt.Errorf("evaluate config source %q via %q: %w", sourcePath, evaluationPath, err)
	}
	returns := state.GetTop() - stackBase
	value := lua.LValue(lua.LNil)
	if returns > 0 {
		value = state.Get(-1)
		state.Pop(returns)
	}
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

func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}
