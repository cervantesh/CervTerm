package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestNamedColorSchemeStrictValidationIncludesUnselectedSchemes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "invalid unselected scalar",
			source: `return {config_version=2,color_scheme="valid",color_schemes={valid={foreground="#FFFFFF"},unused={background="purple"}}}`,
			want:   `color_schemes["unused"].background: must be #RRGGBB or #RRGGBBAA`,
		},
		{
			name:   "invalid unselected ANSI",
			source: `return {config_version=2,color_scheme="valid",color_schemes={valid={},unused={ansi={"#000000"}}}}`,
			want:   `color_schemes["unused"].ansi: must contain exactly 16 entries`,
		},
		{
			name:   "unknown palette field",
			source: `return {config_version=2,color_schemes={valid={foreground="#FFFFFF",typo="#000000"}}}`,
			want:   `color_schemes["valid"].typo: unknown field`,
		},
		{
			name:   "empty scheme name",
			source: `return {config_version=2,color_schemes={[""]={foreground="#FFFFFF"}}}`,
			want:   `color_schemes: name must not be empty`,
		},
		{
			name:   "non-string scheme name",
			source: `return {config_version=2,color_schemes={[1]={foreground="#FFFFFF"}}}`,
			want:   `color_schemes: field names must be strings`,
		},
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
}

func TestNamedColorSchemeShadesOfPurpleExactValues(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {
config_version=2,
color_scheme="Shades of Purple",
color_schemes={["Shades of Purple"]={
foreground="#FFFFFF", background="#2D2B55", cursor="#FAD000", selection_background="#B362FF",
ansi={"#000000","#D90429","#3AD900","#FFE700","#6943FF","#FF2C70","#80FCFF","#C7C7C7","#686868","#F9555F","#5CFF45","#FFFF85","#6871FF","#FF77FF","#79E8FB","#FFFFFF"}
}}
}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()

	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.ColorScheme != "Shades of Purple" || cfg.Colors.Foreground != "#FFFFFF" || cfg.Colors.Background != "#2D2B55" || cfg.Colors.Cursor != "#FAD000" || cfg.Colors.SelectionBackground != "#B362FF" {
		t.Fatalf("selected Shades of Purple = scheme %q colors %#v", cfg.ColorScheme, cfg.Colors)
	}
	wantANSI := [16]string{"#000000", "#D90429", "#3AD900", "#FFE700", "#6943FF", "#FF2C70", "#80FCFF", "#C7C7C7", "#686868", "#F9555F", "#5CFF45", "#FFFF85", "#6871FF", "#FF77FF", "#79E8FB", "#FFFFFF"}
	if cfg.Colors.ANSI != wantANSI {
		t.Fatalf("Shades of Purple ANSI = %#v, want %#v", cfg.Colors.ANSI, wantANSI)
	}
}

func TestNamedColorSchemeDuplicateDeclarationsMergeAcrossIncludeAndPrimary(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,color_schemes={shared={foreground="#111111",background="#222222",indexed_colors={[16]="#161616",[17]="#171717"}}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},color_scheme="shared",color_schemes={shared={background="#333333",cursor="#444444",indexed_colors={[17]="#272727",[18]="#181818"}}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()

	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Colors.Foreground != "#111111" || cfg.Colors.Background != "#333333" || cfg.Colors.Cursor != "#444444" {
		t.Fatalf("merged scheme colors = %#v", cfg.Colors)
	}
	for index, want := range map[uint8]string{16: "#161616", 17: "#272727", 18: "#181818"} {
		if got := cfg.Colors.IndexedColors.Lookup(index); got != want {
			t.Fatalf("indexed color %d = %q, want %q", index, got, want)
		}
	}
}

func TestNamedColorSchemeSelectorPrecedenceEnvironmentProfileCLI(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {
config_version=2, color_scheme="base", default_environment="windows", default_profile="work",
color_schemes={base={background="#111111"},environment={background="#222222"},profile={background="#333333"},cli={background="#444444"}},
environments={windows={color_scheme="environment"}}, profiles={work={color_scheme="profile"}}
}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()

	profileComposition, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	profileConfig := FromDocument(Defaults(), profileComposition.Document)
	if profileConfig.ColorScheme != "profile" || profileConfig.Colors.Background != "#333333" {
		t.Fatalf("profile selector precedence = scheme %q background %q", profileConfig.ColorScheme, profileConfig.Colors.Background)
	}

	cliComposition, err := ComposeSourceGraph(state, graph, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 7, Path: "color_scheme", Value: "cli"}}})
	if err != nil {
		t.Fatal(err)
	}
	cliConfig := FromDocument(Defaults(), cliComposition.Document)
	if cliConfig.ColorScheme != "cli" || cliConfig.Colors.Background != "#444444" {
		t.Fatalf("CLI selector precedence = scheme %q background %q", cliConfig.ColorScheme, cliConfig.Colors.Background)
	}
	assertProvenanceLayers(t, cliComposition.Provenance, "color_scheme", []ProvenanceLayer{LayerDefaults, LayerPrimary, LayerEnvironment, LayerProfile, LayerCLI})
}

func TestNamedColorSchemeExplicitPaletteAndANSIOverrideScheme(t *testing.T) {
	dir := t.TempDir()
	schemeANSI := `{"#000000","#010101","#020202","#030303","#040404","#050505","#060606","#070707","#080808","#090909","#0A0A0A","#0B0B0B","#0C0C0C","#0D0D0D","#0E0E0E","#0F0F0F"}`
	explicitANSI := `{"#101010","#111111","#121212","#131313","#141414","#151515","#161616","#171717","#181818","#191919","#1A1A1A","#1B1B1B","#1C1C1C","#1D1D1D","#1E1E1E","#1F1F1F"}`
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,color_scheme="selected",color_schemes={selected={foreground="#AAAAAA",background="#BBBBBB",cursor="#CCCCCC",ansi=`+schemeANSI+`}},colors={background="#DDDDDD",ansi=`+explicitANSI+`}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 9, Path: "colors.cursor", Value: "#EEEEEE"}}})
	defer state.Close()
	defer graph.Close()

	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Colors.Foreground != "#AAAAAA" || cfg.Colors.Background != "#DDDDDD" || cfg.Colors.Cursor != "#EEEEEE" {
		t.Fatalf("scheme and explicit color precedence = %#v", cfg.Colors)
	}
	wantANSI := [16]string{"#101010", "#111111", "#121212", "#131313", "#141414", "#151515", "#161616", "#171717", "#181818", "#191919", "#1A1A1A", "#1B1B1B", "#1C1C1C", "#1D1D1D", "#1E1E1E", "#1F1F1F"}
	if cfg.Colors.ANSI != wantANSI {
		t.Fatalf("explicit ANSI did not replace scheme ANSI: got %#v want %#v", cfg.Colors.ANSI, wantANSI)
	}
}

func TestNamedColorSchemeIndexedMergeAndUnsets(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,color_schemes={per_key={indexed_colors={[16]="#161616",[17]="#171717"}},whole={indexed_colors={[16]="#161616",[17]="#171717"}}}}`)
	writeGraphLua(t, dir, "reset.lua", `return {config_version=2,color_schemes={per_key={indexed_colors={[16]=unset,[18]="#181818"}},whole={indexed_colors=unset}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","reset.lua"},color_scheme="per_key",color_schemes={per_key={indexed_colors={[19]="#191919"}},whole={indexed_colors={[18]="#282828"}}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()

	perKey, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	perKeyColors := FromDocument(Defaults(), perKey.Document).Colors.IndexedColors
	for index, want := range map[uint8]string{16: "", 17: "#171717", 18: "#181818", 19: "#191919"} {
		if got := perKeyColors.Lookup(index); got != want {
			t.Fatalf("per-key indexed color %d = %q, want %q", index, got, want)
		}
	}

	whole, err := ComposeSourceGraph(state, graph, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 1, Path: "color_scheme", Value: "whole"}}})
	if err != nil {
		t.Fatal(err)
	}
	wholeColors := FromDocument(Defaults(), whole.Document).Colors.IndexedColors
	for index, want := range map[uint8]string{16: "", 17: "", 18: "#282828"} {
		if got := wholeColors.Lookup(index); got != want {
			t.Fatalf("whole-unset indexed color %d = %q, want %q", index, got, want)
		}
	}
}

func TestNamedColorSchemeUnknownSelectionAndAbsentSelectorCompatibility(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,color_schemes={known={background="#010203"}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()

	composition, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := FromDocument(Defaults(), composition.Document)
	defaults := Defaults()
	if cfg.ColorScheme != "" || !reflect.DeepEqual(cfg.Colors, defaults.Colors) {
		t.Fatalf("absent selector changed compatibility: scheme %q colors %#v", cfg.ColorScheme, cfg.Colors)
	}

	_, err = ComposeSourceGraph(state, graph, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 2, Path: "color_scheme", Value: "missing"}}})
	if err == nil || !strings.Contains(err.Error(), `selected color scheme "missing" is not declared`) {
		t.Fatalf("unknown scheme selection error = %v", err)
	}
}

func TestNamedColorSchemeProvenanceDefaultsSchemeExplicit(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,color_scheme="selected",color_schemes={selected={background="#111111",indexed_colors={[16]="#161616"}}},colors={background="#222222",indexed_colors={[16]="#262626"}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()

	assertProvenanceLayers(t, composition.Provenance, "colors.background", []ProvenanceLayer{LayerDefaults, LayerColorScheme, LayerPrimary})
	background, ok := composition.Provenance.Lookup("colors.background")
	if !ok || background.Overwritten[len(background.Overwritten)-1].Name != "selected" || background.Overwritten[len(background.Overwritten)-1].Layer != LayerColorScheme {
		t.Fatalf("background scheme provenance = %#v, ok=%v", background, ok)
	}
	indexedPath := indexedColorEntryPath("colors.indexed_colors", 16)
	assertProvenanceLayers(t, composition.Provenance, indexedPath, []ProvenanceLayer{LayerColorScheme, LayerPrimary})
	indexed, ok := composition.Provenance.Lookup(indexedPath)
	if !ok || indexed.Overwritten[0].Name != "selected" || indexed.Overwritten[0].CanonicalSource != canonicalTestSource(t, filepath.Join(dir, "primary.lua")) {
		t.Fatalf("indexed scheme provenance = %#v, ok=%v", indexed, ok)
	}
}

func TestNamedColorSchemeExplicitUnsetsRevealBuiltInDefaults(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,color_scheme="selected",color_schemes={selected={background="#111111",ansi={"#101010","#111111","#121212","#131313","#141414","#151515","#161616","#171717","#181818","#191919","#1A1A1A","#1B1B1B","#1C1C1C","#1D1D1D","#1E1E1E","#1F1F1F"},indexed_colors={[16]="#161616"}}},colors={background=unset,indexed_colors={[16]=unset}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	composition, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Colors.Background != Defaults().Colors.Background || cfg.Colors.IndexedColors.Lookup(16) != "" {
		t.Fatalf("explicit unsets did not reveal defaults: %#v", cfg.Colors)
	}
	for _, path := range []string{"colors.background", "colors.indexed_colors[16]"} {
		record, ok := composition.Provenance.Lookup(path)
		if !ok || !record.Tombstone || record.Winner.Layer != LayerPrimary {
			t.Fatalf("unset provenance %s = %#v, ok=%v", path, record, ok)
		}
	}
}

func TestNamedColorSchemeDuplicateDeclarationProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,color_schemes={selected={background="#111111"}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},color_scheme="selected",color_schemes={selected={background="#222222"}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	record, ok := composition.Provenance.Lookup("colors.background")
	if !ok || record.Winner.Layer != LayerColorScheme || record.Winner.CanonicalSource != canonicalTestSource(t, primary) || len(record.Overwritten) != 2 || record.Overwritten[1].Layer != LayerColorScheme || !strings.HasSuffix(filepath.ToSlash(record.Overwritten[1].CanonicalSource), "/base.lua") {
		t.Fatalf("duplicate scheme provenance = %#v, ok=%v", record, ok)
	}
}

func TestNamedColorSchemeEmptySelectorAndNodeLimit(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,color_scheme="",color_schemes={one={},two={}}}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	if _, err = ComposeSourceGraph(state, graph, CompositionOptions{}); err == nil || !strings.Contains(err.Error(), `selected color scheme "" is not declared`) || !strings.Contains(err.Error(), "selected by primary") {
		t.Fatalf("empty selector error = %v", err)
	}
	if _, err = ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 1}); err == nil || !strings.Contains(err.Error(), "limit 1 exceeded at color_schemes") {
		t.Fatalf("catalog node limit error = %v", err)
	}
}

func TestLoadLuaNamedSchemesAndV1Compatibility(t *testing.T) {
	dir := t.TempDir()
	v2 := writeGraphLua(t, dir, "v2.lua", `return {config_version=2,color_scheme="selected",color_schemes={selected={background="#123456"}}}`)
	cfg, err := LoadLua(v2, Defaults())
	if err != nil || cfg.ColorScheme != "selected" || cfg.Colors.Background != "#123456" {
		t.Fatalf("v2 named scheme load cfg=%#v err=%v", cfg, err)
	}
	missing := writeGraphLua(t, dir, "missing.lua", `return {config_version=2,color_scheme="missing",color_schemes={selected={background="#123456"}}}`)
	if _, err := LoadLua(missing, Defaults()); err == nil || !strings.Contains(err.Error(), `selected color scheme "missing" is not declared`) {
		t.Fatalf("missing direct-load scheme error = %v", err)
	}
	v1 := writeGraphLua(t, dir, "v1.lua", `return {color_scheme="legacy-ignored",colors={background="#654321"}}`)
	cfg, err = LoadLua(v1, Defaults())
	if err != nil || cfg.ColorScheme != "" || cfg.Colors.Background != "#654321" {
		t.Fatalf("v1 selector compatibility cfg=%#v err=%v", cfg, err)
	}
	state := lua.NewState()
	defer state.Close()
	if err := state.DoFile(v1); err != nil {
		t.Fatal(err)
	}
	document, err := DecodeDocument(v1, state.Get(-1).(*lua.LTable))
	if err != nil || document.Has("color_scheme") {
		t.Fatalf("v1 selector presence document=%#v err=%v", document.Present, err)
	}
	fields, err := SchemaFields(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if field.Path == "color_scheme" {
			t.Fatalf("v1 schema unexpectedly advertises color_scheme: %#v", field)
		}
	}
}

func TestNamedColorSchemeV1IncludeIgnoresSelectorAndMalformedColors(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "legacy.lua", `return {color_scheme="legacy-ignored",colors={foreground=true,ansi="bad",indexed_colors="bad"}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"legacy.lua"},color_scheme="selected",color_schemes={selected={foreground="#ABCDEF",background="#123456",indexed_colors={[16]="#161616"}}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.ColorScheme != "selected" || cfg.Colors.Foreground != "#ABCDEF" || cfg.Colors.Background != "#123456" || cfg.Colors.IndexedColors.Lookup(16) != "#161616" {
		t.Fatalf("v1 include affected named scheme: %#v", cfg)
	}
}
