package config

import (
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestComposeSourceGraphAppliesEnvironmentThenProfile(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {
config_version=2, includes={"a.lua","b.lua"}, default_environment="windows", default_profile="work",
window={opacity=0.70},
environments={windows={font={ligatures=false}}},
profiles={work={font={family="Primary Work"}}},
}`)
	writeGraphLua(t, dir, "a.lua", `return {config_version=2,includes={"c.lua"},environments={windows={window={opacity=0.85},font={size=12}},linux={colors={background="#linux"}}},profiles={work={font={family="A Work"}}}}`)
	writeGraphLua(t, dir, "b.lua", `return {config_version=2,environments={windows={window={opacity=0.90}}},profiles={work={window={opacity=0.95}},personal={font={family="Personal"}}}}`)
	writeGraphLua(t, dir, "c.lua", `return {config_version=2,environments={windows={window={opacity=0.80},font={family="C Env"}}},profiles={work={font={size=10}}}}`)

	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Window.Opacity != 0.95 || cfg.Font.Family != "Primary Work" || cfg.Font.Size != 10 || cfg.Font.Ligatures {
		t.Fatalf("selected config window=%#v font=%#v", cfg.Window, cfg.Font)
	}
	if cfg.Colors.Background == "#linux" {
		t.Fatal("unselected environment affected composition")
	}
	if composition.Selection.Environment == nil || composition.Selection.Environment.Name != "windows" || composition.Selection.Environment.Basis != SelectionDefault {
		t.Fatalf("environment selection = %#v", composition.Selection.Environment)
	}
	if composition.Selection.Profile == nil || composition.Selection.Profile.Name != "work" || composition.Selection.Profile.Basis != SelectionDefault {
		t.Fatalf("profile selection = %#v", composition.Selection.Profile)
	}
	if origin := composition.Selection.Environment.DefaultOrigin; origin == nil || origin.Layer != LayerPrimary || origin.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "primary.lua")) {
		t.Fatalf("environment default origin = %#v", origin)
	}
	assertProvenanceLayers(t, composition.Provenance, "window.opacity", []ProvenanceLayer{
		LayerDefaults, LayerPrimary, LayerEnvironment, LayerEnvironment, LayerEnvironment, LayerProfile,
	})
	opacity, _ := composition.Provenance.Lookup("window.opacity")
	prior := opacity.Overwritten[len(opacity.Overwritten)-1]
	if prior.Layer != LayerEnvironment || prior.Name != "windows" || prior.RequestedSource != "b.lua" || prior.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "b.lua")) || prior.AuthoredVersion != 2 || prior.Version != 2 {
		t.Fatalf("environment provenance = %#v", prior)
	}
	family, _ := composition.Provenance.Lookup("font.family")
	if family.Winner.Layer != LayerProfile || family.Winner.Name != "work" || family.Winner.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "primary.lua")) || family.Winner.AuthoredVersion != 2 || family.Winner.Version != 2 {
		t.Fatalf("font.family provenance = %#v", family)
	}
}

func TestSelectionPrecedenceAndMissingRules(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,default_environment="windows",default_profile="work",environments={windows={font={family="Windows"}},linux={font={family="Linux"}}},profiles={work={font={size=11}},personal={font={size=13}}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	windows, linux, work, personal := "windows", "linux", "work", "personal"
	tests := []struct {
		name                string
		options             SelectionOptions
		wantEnvironment     string
		wantEnvironmentFrom SelectionBasis
		wantProfile         string
		wantProfileFrom     SelectionBasis
	}{
		{name: "explicit", options: SelectionOptions{EnvironmentOverride: &linux, EnvironmentVariableValue: &windows, ProfileOverride: &personal, ProfileVariableValue: &work}, wantEnvironment: "linux", wantEnvironmentFrom: SelectionExplicit, wantProfile: "personal", wantProfileFrom: SelectionExplicit},
		{name: "environment variables", options: SelectionOptions{EnvironmentVariableValue: &linux, ProfileVariableValue: &personal}, wantEnvironment: "linux", wantEnvironmentFrom: SelectionEnvironmentVariable, wantProfile: "personal", wantProfileFrom: SelectionEnvironmentVariable},
		{name: "configured defaults", options: SelectionOptions{GOOS: "linux"}, wantEnvironment: "windows", wantEnvironmentFrom: SelectionDefault, wantProfile: "work", wantProfileFrom: SelectionDefault},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			composition, err := ComposeSourceGraph(state, graph, CompositionOptions{Selection: test.options})
			if err != nil {
				t.Fatal(err)
			}
			if composition.Selection.Environment.Name != test.wantEnvironment || composition.Selection.Environment.Basis != test.wantEnvironmentFrom {
				t.Fatalf("environment = %#v", composition.Selection.Environment)
			}
			if composition.Selection.Profile.Name != test.wantProfile || composition.Selection.Profile.Basis != test.wantProfileFrom {
				t.Fatalf("profile = %#v", composition.Selection.Profile)
			}
		})
	}
	missing := "missing"
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{Selection: SelectionOptions{EnvironmentOverride: &missing}}); err == nil || !strings.Contains(err.Error(), `selected environment "missing" from explicit is not declared`) {
		t.Fatalf("missing explicit environment error = %v", err)
	}
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{Selection: SelectionOptions{ProfileVariableValue: &missing}}); err == nil || !strings.Contains(err.Error(), `selected profile "missing" from environment_variable is not declared`) {
		t.Fatalf("missing profile variable error = %v", err)
	}
}

func TestSelectionGOOSFallbackAndNoSelection(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,environments={windows={font={family="Windows"}}},profiles={work={font={size=10}}}}`)
	state, graph, windowsComposition := buildComposition(t, primary, CompositionOptions{Selection: SelectionOptions{GOOS: "windows"}})
	defer state.Close()
	defer graph.Close()
	if windowsComposition.Selection.Environment == nil || windowsComposition.Selection.Environment.Basis != SelectionGOOS {
		t.Fatalf("GOOS selection = %#v", windowsComposition.Selection.Environment)
	}
	if windowsComposition.Selection.Profile != nil {
		t.Fatalf("unexpected profile = %#v", windowsComposition.Selection.Profile)
	}
	unknown, err := ComposeSourceGraph(state, graph, CompositionOptions{Selection: SelectionOptions{GOOS: "plan9"}})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Selection.Environment != nil || unknown.Selection.Profile != nil {
		t.Fatalf("unknown GOOS selection = %#v", unknown.Selection)
	}
	if cfg := FromDocument(Defaults(), unknown.Document); cfg.Font.Family == "Windows" || cfg.Font.Size == 10 {
		t.Fatalf("unselected declarations applied: %#v", cfg.Font)
	}
}

func TestNamedLayerUnsetAndHigherProfile(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,default_environment="windows",default_profile="work",font={family="Base"},environments={windows={font={family=unset},shell={env={TOKEN="env"}}}},profiles={work={font={family="Profile"},shell={env={TOKEN=unset}}}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Font.Family != "Profile" {
		t.Fatalf("profile did not override environment tombstone: %#v", cfg.Font)
	}
	if _, ok := cfg.Shell.Env["TOKEN"]; ok {
		t.Fatalf("profile map tombstone did not remove environment value: %#v", cfg.Shell.Env)
	}
	assertProvenanceLayers(t, composition.Provenance, "font.family", []ProvenanceLayer{LayerDefaults, LayerPrimary, LayerEnvironment, LayerProfile})
	token, _ := composition.Provenance.Lookup(`shell.env["TOKEN"]`)
	if !token.Tombstone || token.Winner.Layer != LayerProfile || token.Winner.Name != "work" {
		t.Fatalf("selected map tombstone provenance = %#v", token)
	}
}

func TestSelectedLayerLimitRepeatAndIncludeDefaultOrigin(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,default_environment="windows",environments={windows={shell={args={"1","2","3","4","5"}}}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"}}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 5}); err == nil || !strings.Contains(err.Error(), "node/list-entry limit 5") {
		t.Fatalf("selected-layer limit error = %v", err)
	}
	first, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 6})
	if err != nil || first.NodeCount != 6 {
		t.Fatalf("selected-layer exact limit nodes=%d err=%v", first.NodeCount, err)
	}
	second, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 6})
	if err != nil || second.NodeCount != first.NodeCount || len(second.Provenance.Records()) != len(first.Provenance.Records()) {
		t.Fatalf("selected-layer repeated composition nodes=%d records=%d err=%v", second.NodeCount, len(second.Provenance.Records()), err)
	}
	choice := first.Selection.Environment
	if choice == nil || choice.DefaultOrigin == nil || choice.DefaultOrigin.Layer != LayerInclude || choice.DefaultOrigin.RequestedSource != "base.lua" || choice.DefaultOrigin.CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "base.lua")) || choice.DefaultOrigin.AuthoredVersion != 2 || choice.DefaultOrigin.Version != 2 {
		t.Fatalf("include default origin = %#v", choice)
	}
}

func TestConfiguredMissingSelectionFails(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,default_environment="missing",default_profile="absent",environments={windows={}},profiles={work={}}}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{}); err == nil || !strings.Contains(err.Error(), `selected environment "missing" from default is not declared`) {
		t.Fatalf("missing configured default error = %v", err)
	}
}

func TestNamedLayerStructuralValidationAndPublicBoundary(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{name: "value table", source: `return {config_version=2,environments={windows=3}}`, want: "environments.windows"},
		{name: "empty name", source: `return {config_version=2,profiles={[""]={font={size=12}}}}`, want: "name must not be empty"},
		{name: "nested includes", source: `return {config_version=2,profiles={work={includes={"x.lua"}}}}`, want: "profiles.work.includes: unknown field"},
		{name: "nested version", source: `return {config_version=2,environments={windows={config_version=2}}}`, want: "environments.windows.config_version: unknown field"},
		{name: "unknown partial", source: `return {config_version=2,profiles={work={font={typo=true}}}}`, want: "profiles.work.font.typo: unknown field"},
		{name: "default unset", source: `return {config_version=2,default_profile=unset,profiles={work={}}}`, want: "cervterm.config.unset is not allowed"},
		{name: "non-string name", source: `return {config_version=2,environments={[1]={}}}`, want: "field names must be strings"},
		{name: "nested default", source: `return {config_version=2,profiles={work={default_profile="other"}}}`, want: "profiles.work.default_profile: unknown field"},
		{name: "nested profiles", source: `return {config_version=2,environments={windows={profiles={work={}}}}}`, want: "environments.windows.profiles: unknown field"},
		{name: "malformed key action", source: `return {config_version=2,profiles={work={keys={{key="a",action=42}}}}}`, want: "profiles.work.keys[1].action"},
		{name: "malformed event", source: `return {config_version=2,environments={windows={events={bell=true}}}}`, want: "environments.windows.events.bell"},
		{name: "v1 metadata", source: `return {profiles={work={font={size=12}}}}`, want: "requires config_version = 2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeGraphLua(t, dir, "primary.lua", test.source)
			state := lua.NewState()
			state.SetGlobal("unset", NewUnsetValue(state))
			defer state.Close()
			_, err := BuildSourceGraph(state, path, DefaultSourceGraphOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	publicCases := []struct {
		name, source, want string
	}{
		{name: "v2 default environment", source: `return {config_version=2,default_environment="windows"}`, want: "reserved for a later Phase 2 slice"},
		{name: "v2 default profile", source: `return {config_version=2,default_profile="work"}`, want: "reserved for a later Phase 2 slice"},
		{name: "v2 environments", source: `return {config_version=2,environments={windows={}}}`, want: "reserved for a later Phase 2 slice"},
		{name: "v2 profiles", source: `return {config_version=2,profiles={work={}}}`, want: "reserved for a later Phase 2 slice"},
		{name: "v1 default environment", source: `return {config_version=1,default_environment="windows"}`, want: "requires config_version = 2"},
		{name: "v1 default profile", source: `return {default_profile="work"}`, want: "requires config_version = 2"},
		{name: "v1 environments", source: `return {environments={windows={}}}`, want: "requires config_version = 2"},
		{name: "v1 profiles", source: `return {profiles={work={}}}`, want: "requires config_version = 2"},
	}
	for _, test := range publicCases {
		t.Run("public "+test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeGraphLua(t, dir, "single.lua", test.source)
			if _, err := LoadLua(path, Defaults()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("public error = %v, want %q", err, test.want)
			}
		})
	}
	state := lua.NewState()
	defer state.Close()
	root := state.NewTable()
	root.RawSetString("config_version", lua.LNumber(2))
	profiles := state.NewTable()
	work := state.NewTable()
	font := state.NewTable()
	font.RawSetString("size", NewUnsetValue(state))
	work.RawSetString("font", font)
	profiles.RawSetString("work", work)
	root.RawSetString("profiles", profiles)
	if _, err := DecodeDocument("single.lua", root); err == nil || !strings.Contains(err.Error(), "reserved for a later Phase 2 slice") {
		t.Fatalf("public nested unset selection error = %v", err)
	}
}
