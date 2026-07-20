package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestComposeSourceGraphUsesDeterministicSchemaPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"a.lua","b.lua"}, window={height=40}, font={ligatures=false}, shell={env={A="primary",D="primary"}}}`)
	writeGraphLua(t, dir, "a.lua", `return {config_version=2, includes={"c.lua"}, window={height=20}, font={size=11}, shell={args={"a"},env={A="a",B="a"}}, events={title=function() return "a" end}}`)
	writeGraphLua(t, dir, "b.lua", `return {config_version=2, window={width=30}, shell={args={"b"},env={B="b",C="b"}}, events={output=function() return "b" end}}`)
	writeGraphLua(t, dir, "c.lua", `return {config_version=2, window={width=10}, font={family="C Font"}, shell={args={"c"},env={A="c"}}, events={output=function() return "c" end}}`)

	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	base := Defaults()
	cfg := FromDocument(base, composition.Document)
	if cfg.Window.Width != 30 || cfg.Window.Height != 40 || cfg.Font.Family != "C Font" || cfg.Font.Size != 11 || cfg.Font.Ligatures {
		t.Fatalf("composed scalar/record config = %#v %#v", cfg.Window, cfg.Font)
	}
	if got := strings.Join(cfg.Shell.Args, ","); got != "b" {
		t.Fatalf("list replacement = %q", got)
	}
	for key, want := range map[string]string{"A": "primary", "B": "b", "C": "b", "D": "primary"} {
		if cfg.Shell.Env[key] != want {
			t.Fatalf("shell.env[%s] = %q, want %q", key, cfg.Shell.Env[key], want)
		}
	}
	events := tableField(composition.Document.Root, "events")
	if events == nil || events.RawGetString("output").Type() != lua.LTFunction || events.RawGetString("title").Type() != lua.LTFunction {
		t.Fatalf("merged events = %v", events)
	}
	assertProvenanceLayers(t, composition.Provenance, "window.width", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerInclude})
	assertProvenanceLayers(t, composition.Provenance, "window.height", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerPrimary})
	assertProvenanceLayers(t, composition.Provenance, "shell.args", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerInclude, LayerInclude})
	assertProvenanceLayers(t, composition.Provenance, `shell.env["A"]`, []ProvenanceLayer{LayerInclude, LayerInclude, LayerPrimary})
	if record, ok := composition.Provenance.Lookup("events.output"); !ok || record.Winner.RequestedSource != "b.lua" || record.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "b.lua")) {
		t.Fatalf("events.output provenance = %#v, ok=%v", record, ok)
	}
	assertProvenanceLayers(t, composition.Provenance, "events.output", []ProvenanceLayer{LayerInclude, LayerInclude})
	if _, ok := composition.Provenance.Lookup("events.scroll"); ok {
		t.Fatal("absent event callback received fabricated default provenance")
	}
	if _, ok := composition.Provenance.Lookup("keys"); ok {
		t.Fatal("absent key list received fabricated default provenance")
	}
}

func TestComposeSourceGraphUnsetRestoresDefaultsAndCanBeOverridden(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"low.lua","reset.lua","higher.lua"}, shell={env={TOKEN="new"}}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2, window={width=10}, font={family="Low",size=9}, shell={args={"low"},env={TOKEN="secret",OLD="old"}}, events={output=function() end}}`)
	writeGraphLua(t, dir, "reset.lua", `return {config_version=2, window={width=unset}, font=unset, shell={args=unset,env={TOKEN=unset,KEEP="kept"}}, events={output=unset}}`)
	writeGraphLua(t, dir, "higher.lua", `return {config_version=2, window={width=30}, font={size=20}}`)

	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	base := Defaults()
	base.Font.Family = "Default Font"
	base.Font.Size = 12
	base.Shell.Args = []string{"default"}
	cfg := FromDocument(base, composition.Document)
	if cfg.Font.Family != "Default Font" || cfg.Font.Size != 20 || strings.Join(cfg.Shell.Args, ",") != "default" {
		t.Fatalf("unset/default config font=%#v args=%#v", cfg.Font, cfg.Shell.Args)
	}
	if cfg.Shell.Env["TOKEN"] != "new" || cfg.Shell.Env["KEEP"] != "kept" || cfg.Shell.Env["OLD"] != "old" {
		t.Fatalf("map merge/unset = %#v", cfg.Shell.Env)
	}
	events := tableField(composition.Document.Root, "events")
	if events == nil || events.RawGetString("output") != lua.LNil {
		t.Fatalf("unset event remains: %v", events)
	}
	family, _ := composition.Provenance.Lookup("font.family")
	if !family.Tombstone || family.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "reset.lua")) {
		t.Fatalf("font.family tombstone = %#v", family)
	}
	if got := provenanceLayers(composition.Provenance, "font.size"); !equalLayers(got, []ProvenanceLayer{LayerDefaults, LayerInclude, LayerInclude, LayerInclude}) {
		t.Fatalf("font.size layers = %v", got)
	}
	assertProvenanceLayers(t, composition.Provenance, "window.width", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerInclude, LayerInclude})
	token, _ := composition.Provenance.Lookup(`shell.env["TOKEN"]`)
	if !token.Sensitive || token.Tombstone || token.Winner.Layer != LayerPrimary {
		t.Fatalf("TOKEN provenance = %#v", token)
	}
	encoded, err := json.Marshal(composition.Provenance.Records())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "new\"") {
		t.Fatalf("provenance leaked values: %s", encoded)
	}
}

func TestComposeSourceGraphWholeMapAndEventsUnset(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","reset.lua","higher.lua"}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,shell={env={A="low",B="low"}},events={output=function() end,title=function() end}}`)
	writeGraphLua(t, dir, "reset.lua", `return {config_version=2,shell={env=unset},events=unset}`)
	writeGraphLua(t, dir, "higher.lua", `return {config_version=2,shell={env={A="higher"}},events={title=function() end}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Shell.Env["A"] != "higher" {
		t.Fatalf("higher map key = %#v", cfg.Shell.Env)
	}
	if _, ok := cfg.Shell.Env["B"]; ok {
		t.Fatalf("whole-map unset retained B: %#v", cfg.Shell.Env)
	}
	events := tableField(composition.Document.Root, "events")
	if events == nil || events.RawGetString("output") != lua.LNil || events.RawGetString("title").Type() != lua.LTFunction {
		t.Fatalf("whole-events unset/higher merge = %v", events)
	}
	removed, _ := composition.Provenance.Lookup(`shell.env["B"]`)
	if !removed.Tombstone || removed.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "reset.lua")) {
		t.Fatalf("removed map provenance = %#v", removed)
	}
	output, _ := composition.Provenance.Lookup("events.output")
	if !output.Tombstone || output.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "reset.lua")) {
		t.Fatalf("removed event provenance = %#v", output)
	}
}

func TestComposeSourceGraphMigratesV1PartialWithoutMaterializingDefaults(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2, includes={"legacy.lua"}, font={size=14}}`)
	writeGraphLua(t, dir, "legacy.lua", `return {font={family="Legacy",size="wrong"},shell={args={"pwsh",3,"-NoLogo"},env={GOOD="yes",BAD=9}},unknown=true}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	base := Defaults()
	cfg := FromDocument(base, composition.Document)
	if cfg.Font.Family != "Legacy" || cfg.Font.Size != 14 {
		t.Fatalf("migrated v1 font = %#v", cfg.Font)
	}
	if got := strings.Join(cfg.Shell.Args, ","); got != "pwsh,-NoLogo" || cfg.Shell.Env["GOOD"] != "yes" {
		t.Fatalf("migrated v1 collections args=%q env=%#v", got, cfg.Shell.Env)
	}
	if _, exists := cfg.Shell.Env["BAD"]; exists {
		t.Fatalf("invalid v1 map entry survived: %#v", cfg.Shell.Env)
	}
	record, _ := composition.Provenance.Lookup("font.family")
	if record.Winner.AuthoredVersion != 1 || record.Winner.Version != 2 {
		t.Fatalf("migration provenance = %#v", record)
	}
}

func TestComposeSourceGraphEnforcesNodeLimitAndStateOwnership(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2, shell={args={"1","2","3","4","5"}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	atLimit, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 6})
	if err != nil || atLimit.NodeCount != 6 {
		t.Fatalf("exact limit composition nodes=%d err=%v", atLimit.NodeCount, err)
	}
	repeated, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 6})
	if err != nil || repeated.NodeCount != atLimit.NodeCount || len(repeated.Provenance.Records()) != len(atLimit.Provenance.Records()) {
		t.Fatalf("repeated deterministic composition nodes=%d records=%d err=%v", repeated.NodeCount, len(repeated.Provenance.Records()), err)
	}
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 5}); err == nil || !strings.Contains(err.Error(), "node/list-entry limit 5") {
		t.Fatalf("limit error = %v", err)
	}
	other := lua.NewState()
	defer other.Close()
	if _, err := ComposeSourceGraph(other, graph, CompositionOptions{}); err == nil || !strings.Contains(err.Error(), "different Lua candidate state") {
		t.Fatalf("state ownership error = %v", err)
	}
}

func TestSingleSourceDecodeRejectsUnsetWhileGraphAcceptsIt(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	root := state.NewTable()
	root.RawSetString("config_version", lua.LNumber(2))
	font := state.NewTable()
	font.RawSetString("size", NewUnsetValue(state))
	root.RawSetString("font", font)
	if _, err := DecodeDocument("single.lua", root); err == nil || !strings.Contains(err.Error(), "not available in single-source loading") {
		t.Fatalf("single-source unset error = %v", err)
	}

	dir := t.TempDir()
	legacyPrimary := writeGraphLua(t, dir, "legacy-primary.lua", `return {config_version=2,includes={"legacy.lua"}}`)
	writeGraphLua(t, dir, "legacy.lua", `return {font={size=unset}}`)
	legacyState := lua.NewState()
	legacyState.SetGlobal("unset", NewUnsetValue(legacyState))
	if _, err := BuildSourceGraph(legacyState, legacyPrimary, DefaultSourceGraphOptions()); err == nil || !strings.Contains(err.Error(), "requires config_version = 2") {
		legacyState.Close()
		t.Fatalf("v1 include unset error = %v", err)
	}
	legacyState.Close()

	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,font={size=unset}}`)
	candidate := lua.NewState()
	candidate.SetGlobal("unset", NewUnsetValue(candidate))
	defer candidate.Close()
	graph, err := BuildSourceGraph(candidate, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	composition, err := ComposeSourceGraph(candidate, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if record, ok := composition.Provenance.Lookup("font.size"); !ok || !record.Tombstone {
		t.Fatalf("candidate unset provenance = %#v, ok=%v", record, ok)
	}
}

func canonicalTestSource(t *testing.T, path string) string {
	t.Helper()
	canonical, _, err := canonicalLocalFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func buildComposition(t *testing.T, primary string, options CompositionOptions) (*lua.LState, *SourceGraph, Composition) {
	t.Helper()
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	composition, err := ComposeSourceGraph(state, graph, options)
	if err != nil {
		graph.Close()
		state.Close()
		t.Fatal(err)
	}
	return state, graph, composition
}

func assertProvenanceLayers(t *testing.T, provenance Provenance, path string, want []ProvenanceLayer) {
	t.Helper()
	if got := provenanceLayers(provenance, path); !equalLayers(got, want) {
		t.Fatalf("%s layers = %v, want %v", path, got, want)
	}
}

func provenanceLayers(provenance Provenance, path string) []ProvenanceLayer {
	record, ok := provenance.Lookup(path)
	if !ok {
		return nil
	}
	out := make([]ProvenanceLayer, 0, len(record.Overwritten)+1)
	for _, previous := range record.Overwritten {
		out = append(out, previous.Layer)
	}
	return append(out, record.Winner.Layer)
}

func equalLayers(a, b []ProvenanceLayer) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
