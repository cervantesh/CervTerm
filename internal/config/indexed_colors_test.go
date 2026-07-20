package config

import (
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestIndexedColorsStrictDecodeAndValidation(t *testing.T) {
	valid := writeLuaDocument(t, `return {config_version=2,colors={indexed_colors={[16]="#102030",[196]="#FF1010",[255]="#F0F0F0"}}}`)
	cfg, err := LoadLua(valid, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range map[uint8]string{16: "#102030", 196: "#FF1010", 255: "#F0F0F0"} {
		if got := cfg.Colors.IndexedColors.Lookup(index); got != want {
			t.Fatalf("indexed color %d = %q, want %q", index, got, want)
		}
	}
	if got := cfg.Colors.IndexedColors.Lookup(15); got != "" {
		t.Fatalf("ANSI overlap lookup = %q", got)
	}

	tests := []struct{ name, table, want string }{
		{"ansi overlap", `{[15]="#010203"}`, "between 16 and 255"},
		{"too high", `{[256]="#010203"}`, "between 16 and 255"},
		{"negative", `{[-1]="#010203"}`, "between 16 and 255"},
		{"fractional", `{[16.5]="#010203"}`, "between 16 and 255"},
		{"string key", `{["196"]="#010203"}`, "between 16 and 255"},
		{"alpha", `{[196]="#01020380"}`, "must be #RRGGBB"},
		{"wrong value", `{[196]=true}`, "must be string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLuaDocument(t, `return {config_version=2,colors={indexed_colors=`+tt.table+`}}`)
			if _, err := LoadLua(path, Defaults()); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestIndexedColorsComposePerKeyUnsetAndProvenance(t *testing.T) {
	state, graph, composition := buildIndexedUnsetComposition(t, false)
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Colors.IndexedColors.Lookup(16) != "#101010" || cfg.Colors.IndexedColors.Lookup(196) != "" || cfg.Colors.IndexedColors.Lookup(255) != "#FFFFFF" {
		t.Fatalf("composed indexed colors = %#v", cfg.Colors.IndexedColors)
	}
	record, ok := composition.Provenance.Lookup("colors.indexed_colors[196]")
	if !ok || !record.Tombstone || record.Winner.Layer != LayerPrimary || len(record.Overwritten) != 1 || record.Overwritten[0].Layer != LayerInclude {
		t.Fatalf("indexed unset provenance = %#v, ok=%v", record, ok)
	}
	diagnostic, err := DiagnoseConfig(cfg, composition.Provenance, []string{"colors.indexed_colors"})
	if err != nil || len(diagnostic.Fields) != 1 || diagnostic.Fields[0].Value != `{"16":"#101010","255":"#FFFFFF"}` {
		t.Fatalf("indexed diagnostic = %#v err=%v", diagnostic, err)
	}
	if len(diagnostic.Fields[0].Provenance) != 3 {
		t.Fatalf("indexed diagnostic provenance = %#v", diagnostic.Fields[0].Provenance)
	}
}

func TestIndexedColorsWholeFieldUnsetRestoresFallback(t *testing.T) {
	state, graph, composition := buildIndexedUnsetComposition(t, true)
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Colors.IndexedColors.Lookup(16) != "" || cfg.Colors.IndexedColors.Lookup(196) != "" {
		t.Fatalf("whole unset indexed colors = %#v", cfg.Colors.IndexedColors)
	}
	for _, path := range []string{"colors.indexed_colors[16]", "colors.indexed_colors[196]"} {
		record, ok := composition.Provenance.Lookup(path)
		if !ok || !record.Tombstone || record.Winner.Layer != LayerPrimary {
			t.Fatalf("whole unset provenance %s = %#v, ok=%v", path, record, ok)
		}
	}
}

func TestIndexedColorsCapabilitiesAndComparableDiff(t *testing.T) {
	fields, err := SchemaFields(CurrentSchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	var metadata FieldMetadata
	for _, field := range fields {
		if field.Path == "colors.indexed_colors" {
			metadata = field
			break
		}
	}
	if metadata.Kind != KindIndexedColorMap || metadata.ApplyScope != ApplyLive || metadata.CLIOverride || metadata.RuntimeOverride {
		t.Fatalf("indexed color metadata = %#v", metadata)
	}
	base := Defaults()
	next := base
	if err := next.Colors.IndexedColors.Set(196, "#123456"); err != nil {
		t.Fatal(err)
	}
	changes := DiffConfig(next, base)
	found := false
	for _, change := range changes {
		if change.Path == "colors.indexed_colors" && change.Scope == ApplyLive {
			found = true
		}
	}
	if !found {
		t.Fatalf("indexed diff = %#v", changes)
	}
}

func buildIndexedUnsetComposition(t *testing.T, whole bool) (*lua.LState, *SourceGraph, Composition) {
	t.Helper()
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,colors={indexed_colors={[16]="#101010",[196]="#111111"}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},colors={indexed_colors={[196]="#222222",[255]="#FFFFFF"}}}`)
	state := lua.NewState()
	graph, err := BuildSourceGraph(state, primary, SourceGraphOptions{})
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	node, ok := graph.PrimaryNode()
	if !ok {
		graph.Close()
		state.Close()
		t.Fatal("missing primary node")
	}
	colors := node.Document.Root.RawGetString("colors").(*lua.LTable)
	if whole {
		colors.RawSetString("indexed_colors", NewUnsetValue(state))
	} else {
		indexed := colors.RawGetString("indexed_colors").(*lua.LTable)
		indexed.RawSetInt(196, NewUnsetValue(state))
	}
	composition, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		graph.Close()
		state.Close()
		t.Fatal(err)
	}
	return state, graph, composition
}
