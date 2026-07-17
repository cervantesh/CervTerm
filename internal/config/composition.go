package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

const DefaultMaxMergedNodes = 100_000

var eventFieldNames = []string{"bell", "cwd", "focus", "output", "resize", "scroll", "title"}

type CompositionOptions struct {
	MaxMergedNodes int
	Selection      SelectionOptions
	CLIOverrides   []CLIOverride
}

type Composition struct {
	Document   Document
	Provenance Provenance
	NodeCount  int
	Selection  SelectionResult
}

type compositionBuilder struct {
	state      *lua.LState
	root       *lua.LTable
	provenance Provenance
	maxNodes   int
	nodes      int
	document   Document
	origin     ProvenanceOrigin
}

// ComposeSourceGraph applies schema-driven low-to-high merge semantics to a
// completed candidate graph. It does not mutate active configuration or publish
// Teal outputs; callers retain ownership of the graph and candidate Lua state.
func ComposeSourceGraph(state *lua.LState, graph *SourceGraph, options CompositionOptions) (Composition, error) {
	if state == nil || graph == nil {
		return Composition{}, fmt.Errorf("compose config source graph: state and graph are required")
	}
	if graph.state != state {
		return Composition{}, fmt.Errorf("compose config source graph: graph belongs to a different Lua candidate state")
	}
	maxNodes := options.MaxMergedNodes
	if maxNodes <= 0 {
		maxNodes = DefaultMaxMergedNodes
	}
	catalog := buildSelectionCatalog(graph)
	selection, err := resolveSelection(catalog, options.Selection)
	if err != nil {
		return Composition{}, err
	}
	builder := &compositionBuilder{state: state, root: state.NewTable(), provenance: newProvenance(), maxNodes: maxNodes}
	builder.root.RawSetString("config_version", lua.LNumber(CurrentSchemaVersion))
	builder.seedDefaults(rootSchema, "")
	for _, source := range graph.Sources {
		builder.document = source.Document
		builder.origin = sourceLayerOrigin(graph, source, LayerInclude, source.RequestedPath)
		if err := builder.mergeRecord(builder.root, source.Document.Root, rootSchema, ""); err != nil {
			return Composition{}, err
		}
	}
	if selection.Environment != nil {
		if err := builder.applyNamedLayer(graph, "environments", selection.Environment.Name, LayerEnvironment); err != nil {
			return Composition{}, err
		}
	}
	if selection.Profile != nil {
		if err := builder.applyNamedLayer(graph, "profiles", selection.Profile.Name, LayerProfile); err != nil {
			return Composition{}, err
		}
	}
	if err := builder.applyCLIOverrides(options.CLIOverrides); err != nil {
		return Composition{}, err
	}
	present := make(map[string]struct{})
	collectPresence(builder.root, rootSchema, "", present)
	present["config_version"] = struct{}{}
	document := Document{
		Source: graph.Primary, AuthoredVersion: CurrentSchemaVersion, Version: CurrentSchemaVersion,
		Root: builder.root, Present: present,
	}
	return Composition{Document: document, Provenance: builder.provenance, NodeCount: builder.nodes, Selection: selection}, nil
}

func (b *compositionBuilder) applyNamedLayer(graph *SourceGraph, field, name string, layer ProvenanceLayer) error {
	for _, source := range graph.Sources {
		declarations, ok := source.Document.Root.RawGetString(field).(*lua.LTable)
		if !ok {
			continue
		}
		partial, ok := declarations.RawGetString(name).(*lua.LTable)
		if !ok {
			continue
		}
		b.document = source.Document
		b.origin = sourceLayerOrigin(graph, source, layer, name)
		if err := b.mergeRecord(b.root, partial, rootSchema, ""); err != nil {
			return err
		}
	}
	return nil
}

func (b *compositionBuilder) mergeRecord(dst, src *lua.LTable, schema fieldSchema, prefix string) error {
	for _, child := range schema.children {
		value := src.RawGetString(child.name)
		if value == lua.LNil {
			continue
		}
		path := joinPath(prefix, child.name)
		if isUnsetValue(value) {
			if b.document.AuthoredVersion != 2 {
				continue
			}
			if err := b.consume(b.unsetCost(child, path), path); err != nil {
				return err
			}
			b.unset(dst, child, path)
			continue
		}
		if b.document.AuthoredVersion == 1 && !legacyValueCompatible(value, child.kind) {
			continue
		}
		switch child.kind {
		case KindTable:
			sourceTable, ok := value.(*lua.LTable)
			if !ok {
				continue
			}
			target, ok := dst.RawGetString(child.name).(*lua.LTable)
			if !ok {
				target = b.state.NewTable()
				dst.RawSetString(child.name, target)
			}
			if err := b.mergeRecord(target, sourceTable, child, path); err != nil {
				return err
			}
		case KindStringMap:
			if err := b.mergeStringMap(dst, child, path, value); err != nil {
				return err
			}
		case KindIndexedColorMap:
			if err := b.mergeIndexedColorMap(dst, child, path, value); err != nil {
				return err
			}
		case KindEvents:
			if err := b.mergeEvents(dst, child, path, value); err != nil {
				return err
			}
		default:
			cost := 1
			if table, ok := value.(*lua.LTable); ok {
				cost += table.Len()
			}
			if err := b.consume(cost, path); err != nil {
				return err
			}
			dst.RawSetString(child.name, value)
			b.provenance.set(path, b.origin, false, child.sensitive)
		}
	}
	return nil
}

func (b *compositionBuilder) mergeStringMap(dst *lua.LTable, schema fieldSchema, path string, value lua.LValue) error {
	source, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	target, ok := dst.RawGetString(schema.name).(*lua.LTable)
	if !ok {
		target = b.state.NewTable()
		dst.RawSetString(schema.name, target)
	}
	keys := make([]string, 0, source.Len())
	source.ForEach(func(key, _ lua.LValue) {
		if parsed, ok := key.(lua.LString); ok {
			keys = append(keys, string(parsed))
		}
	})
	sort.Strings(keys)
	for _, key := range keys {
		entry := source.RawGetString(key)
		entryPath := mapEntryPath(path, key)
		if isUnsetValue(entry) && b.document.AuthoredVersion == 2 {
			if err := b.consume(1, entryPath); err != nil {
				return err
			}
			target.RawSetString(key, lua.LNil)
			b.provenance.set(entryPath, b.origin, true, schema.sensitive)
			continue
		}
		if _, ok := entry.(lua.LString); !ok {
			continue
		}
		if err := b.consume(1, entryPath); err != nil {
			return err
		}
		target.RawSetString(key, entry)
		b.provenance.set(entryPath, b.origin, false, schema.sensitive)
	}
	return nil
}

func (b *compositionBuilder) mergeIndexedColorMap(dst *lua.LTable, schema fieldSchema, path string, value lua.LValue) error {
	source, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	target, ok := dst.RawGetString(schema.name).(*lua.LTable)
	if !ok {
		target = b.state.NewTable()
		dst.RawSetString(schema.name, target)
	}
	keys := indexedColorKeys(source)
	for _, key := range keys {
		entry := source.RawGetInt(key)
		entryPath := indexedColorEntryPath(path, key)
		if isUnsetValue(entry) && b.document.AuthoredVersion == 2 {
			if err := b.consume(1, entryPath); err != nil {
				return err
			}
			target.RawSetInt(key, lua.LNil)
			b.provenance.set(entryPath, b.origin, true, schema.sensitive)
			continue
		}
		if _, ok := entry.(lua.LString); !ok {
			continue
		}
		if err := b.consume(1, entryPath); err != nil {
			return err
		}
		target.RawSetInt(key, entry)
		b.provenance.set(entryPath, b.origin, false, schema.sensitive)
	}
	return nil
}

func indexedColorKeys(table *lua.LTable) []int {
	keys := make([]int, 0, table.Len())
	table.ForEach(func(key, _ lua.LValue) {
		if number, ok := key.(lua.LNumber); ok {
			keys = append(keys, int(number))
		}
	})
	sort.Ints(keys)
	return keys
}

func (b *compositionBuilder) mergeEvents(dst *lua.LTable, schema fieldSchema, path string, value lua.LValue) error {
	source, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	target, ok := dst.RawGetString(schema.name).(*lua.LTable)
	if !ok {
		target = b.state.NewTable()
		dst.RawSetString(schema.name, target)
	}
	for _, name := range eventFieldNames {
		entry := source.RawGetString(name)
		if entry == lua.LNil {
			continue
		}
		entryPath := joinPath(path, name)
		if isUnsetValue(entry) && b.document.AuthoredVersion == 2 {
			if err := b.consume(1, entryPath); err != nil {
				return err
			}
			target.RawSetString(name, lua.LNil)
			b.provenance.set(entryPath, b.origin, true, schema.sensitive)
			continue
		}
		if _, ok := entry.(*lua.LFunction); !ok {
			continue
		}
		if err := b.consume(1, entryPath); err != nil {
			return err
		}
		target.RawSetString(name, entry)
		b.provenance.set(entryPath, b.origin, false, schema.sensitive)
	}
	return nil
}

func (b *compositionBuilder) unset(dst *lua.LTable, schema fieldSchema, path string) {
	dst.RawSetString(schema.name, lua.LNil)
	leaves := fixedLeafPaths(schema, path)
	exclude := make(map[string]struct{}, len(leaves))
	for _, leaf := range leaves {
		exclude[leaf.path] = struct{}{}
	}
	b.provenance.tombstonePrefixExcept(path, b.origin, schema.sensitive, exclude)
	for _, leaf := range leaves {
		b.provenance.set(leaf.path, b.origin, true, schema.sensitive || leaf.sensitive)
	}
}

func (b *compositionBuilder) unsetCost(schema fieldSchema, path string) int {
	leaves := fixedLeafPaths(schema, path)
	seen := make(map[string]struct{}, len(leaves))
	for _, leaf := range leaves {
		seen[leaf.path] = struct{}{}
	}
	prefixes := []string{path + ".", path + "["}
	for existing := range b.provenance.records {
		if _, ok := seen[existing]; ok {
			continue
		}
		if existing == path || strings.HasPrefix(existing, prefixes[0]) || strings.HasPrefix(existing, prefixes[1]) {
			seen[existing] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return 1
	}
	return len(seen)
}

func (b *compositionBuilder) consume(count int, path string) error {
	if count < 0 || b.nodes > b.maxNodes-count {
		return fmt.Errorf("config composed node/list-entry limit %d exceeded at %s from %q", b.maxNodes, path, b.document.Source)
	}
	b.nodes += count
	return nil
}

func (b *compositionBuilder) seedDefaults(schema fieldSchema, prefix string) {
	origin := ProvenanceOrigin{Layer: LayerDefaults, Name: "built-in defaults"}
	var seed func(fieldSchema, string)
	seed = func(field fieldSchema, path string) {
		switch field.kind {
		case KindTable:
			for _, child := range field.children {
				seed(child, joinPath(path, child.name))
			}
		case KindStringMap, KindIndexedColorMap, KindKeyList, KindEvents:
			// These surfaces have no fixed built-in winner: map provenance begins
			// at concrete keys, while bindings and callbacks are absent by default.
		default:
			b.provenance.set(path, origin, false, field.sensitive)
		}
	}
	for _, child := range schema.children {
		seed(child, joinPath(prefix, child.name))
	}
}

type schemaLeaf struct {
	path      string
	sensitive bool
}

func fixedLeafPaths(schema fieldSchema, path string) []schemaLeaf {
	switch schema.kind {
	case KindTable:
		var out []schemaLeaf
		for _, child := range schema.children {
			for _, leaf := range fixedLeafPaths(child, joinPath(path, child.name)) {
				leaf.sensitive = leaf.sensitive || schema.sensitive
				out = append(out, leaf)
			}
		}
		return out
	case KindEvents:
		out := make([]schemaLeaf, 0, len(eventFieldNames))
		for _, name := range eventFieldNames {
			out = append(out, schemaLeaf{path: joinPath(path, name), sensitive: schema.sensitive})
		}
		return out
	default:
		return []schemaLeaf{{path: path, sensitive: schema.sensitive}}
	}
}

func legacyValueCompatible(value lua.LValue, kind ValueKind) bool {
	switch kind {
	case KindTable, KindStringList, KindStringMap, KindIndexedColorMap, KindKeyList, KindEvents:
		_, ok := value.(*lua.LTable)
		return ok
	case KindString:
		_, ok := value.(lua.LString)
		return ok
	case KindNumber, KindInteger:
		_, ok := value.(lua.LNumber)
		return ok
	case KindBoolean:
		_, ok := value.(lua.LBool)
		return ok
	default:
		return false
	}
}

func mapEntryPath(path, key string) string {
	return path + "[" + strconv.Quote(key) + "]"
}

func indexedColorEntryPath(path string, key int) string {
	return path + "[" + strconv.Itoa(key) + "]"
}
