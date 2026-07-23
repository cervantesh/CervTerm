package config

import (
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/termimage"
	lua "github.com/yuin/gopher-lua"
)

func TestGraphicsDefaultsAreDormantHardCaps(t *testing.T) {
	got := Defaults().Graphics
	if got.Kitty.Enabled || got.Sixel.Enabled || got.ITerm.Enabled || got.Limits.EncodedBytesPerPane != termimage.HardEncodedBytesPerPane || got.Limits.DecodedBytesPerPane != termimage.HardDecodedBytesPerPane || got.Limits.ImageCountPerPane != termimage.HardImagesPerPane || got.Limits.PlacementCountPerPane != termimage.HardPlacementsPerPane || got.Limits.GPUBytesPerContext != termimage.HardGPUBytesPerContext {
		t.Fatalf("defaults=%#v", got)
	}
}

func TestGraphicsStrictV2DecodeAndRestartDiff(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`return {config_version=2,graphics={kitty={enabled=true},sixel={enabled=true},iterm={enabled=true},limits={encoded_bytes_per_pane=1024,decoded_bytes_per_pane=2048,image_count_per_pane=3,placement_count_per_pane=4,gpu_bytes_per_context=4096}}}`); err != nil {
		t.Fatal(err)
	}
	doc, err := DecodeDocument("graphics.lua", state.Get(-1).(*lua.LTable))
	if err != nil {
		t.Fatal(err)
	}
	cfg := FromDocument(Defaults(), doc)
	if err = cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	got := cfg.Graphics
	if !got.Kitty.Enabled || !got.Sixel.Enabled || !got.ITerm.Enabled || got.Limits.EncodedBytesPerPane != 1024 || got.Limits.DecodedBytesPerPane != 2048 || got.Limits.ImageCountPerPane != 3 || got.Limits.PlacementCountPerPane != 4 || got.Limits.GPUBytesPerContext != 4096 {
		t.Fatalf("decoded=%#v", got)
	}
	changes := DiffConfig(cfg, Defaults())
	if len(changes) != 8 {
		t.Fatalf("changes=%#v", changes)
	}
	for _, change := range changes {
		if change.Scope != ApplyRestart || !strings.HasPrefix(change.Path, "graphics.") {
			t.Fatalf("change=%#v", change)
		}
	}
	live := MergeLiveConfig(Defaults(), cfg)
	if live.Graphics != Defaults().Graphics {
		t.Fatalf("restart fields applied live: %#v", live.Graphics)
	}
}

func TestGraphicsStrictV2RejectsWrongProtocolFlagTypes(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{name: "sixel table", source: `return {config_version=2,graphics={sixel=true}}`, want: "graphics.sixel: must be table"},
		{name: "sixel enabled", source: `return {config_version=2,graphics={sixel={enabled="true"}}}`, want: "graphics.sixel.enabled: must be boolean"},
		{name: "iterm table", source: `return {config_version=2,graphics={iterm=false}}`, want: "graphics.iterm: must be table"},
		{name: "iterm enabled", source: `return {config_version=2,graphics={iterm={enabled=1}}}`, want: "graphics.iterm.enabled: must be boolean"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := lua.NewState()
			defer state.Close()
			if err := state.DoString(test.source); err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeDocument("wrong-type.lua", state.Get(-1).(*lua.LTable)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want=%q", err, test.want)
			}
		})
	}
}

func TestGraphicsApprovedHardCapDefaultsAreAccepted(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	source := `return {config_version=2,graphics={kitty={enabled=false},sixel={enabled=false},iterm={enabled=false},limits={encoded_bytes_per_pane=8388608,decoded_bytes_per_pane=67108864,image_count_per_pane=256,placement_count_per_pane=1024,gpu_bytes_per_context=268435456}}}`
	if err := state.DoString(source); err != nil {
		t.Fatal(err)
	}
	document, err := DecodeDocument("approved-defaults.lua", state.Get(-1).(*lua.LTable))
	if err != nil {
		t.Fatal(err)
	}
	if err = FromDocument(Defaults(), document).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGraphicsRejectsV1AndRaisedOrZeroLimits(t *testing.T) {
	for _, protocol := range []string{"kitty", "sixel", "iterm"} {
		t.Run("v1 "+protocol, func(t *testing.T) {
			state := lua.NewState()
			defer state.Close()
			if err := state.DoString(`return {graphics={` + protocol + `={enabled=true}}}`); err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeDocument("v1.lua", state.Get(-1).(*lua.LTable)); err == nil || !strings.Contains(err.Error(), "graphics: requires config_version = 2") {
				t.Fatalf("v1 error=%v", err)
			}
		})
	}
	tests := []func(*GraphicsLimitsConfig){
		func(c *GraphicsLimitsConfig) { c.EncodedBytesPerPane = 0 },
		func(c *GraphicsLimitsConfig) { c.DecodedBytesPerPane = termimage.HardDecodedBytesPerPane + 1 },
		func(c *GraphicsLimitsConfig) { c.ImageCountPerPane = 0 },
		func(c *GraphicsLimitsConfig) { c.PlacementCountPerPane = termimage.HardPlacementsPerPane + 1 },
		func(c *GraphicsLimitsConfig) { c.GPUBytesPerContext = termimage.HardGPUBytesPerContext + 1 },
	}
	for index, mutate := range tests {
		cfg := Defaults()
		mutate(&cfg.Graphics.Limits)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid limit %d accepted", index)
		}
	}
}

func TestGraphicsCompositionPrecedenceIncludesEnvironmentProfileAndCLI(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,graphics={sixel={enabled=true},iterm={enabled=true}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},default_environment="host",default_profile="work",graphics={sixel={enabled=false},iterm={enabled=false}},environments={host={graphics={sixel={enabled=true},iterm={enabled=true}}}},profiles={work={graphics={sixel={enabled=false},iterm={enabled=false}}}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 14, Path: "graphics.sixel.enabled", Value: "true"}}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if !cfg.Graphics.Sixel.Enabled || cfg.Graphics.ITerm.Enabled {
		t.Fatalf("composed flags=%#v", cfg.Graphics)
	}
	assertProvenanceLayers(t, composition.Provenance, "graphics.sixel.enabled", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerPrimary, LayerEnvironment, LayerProfile, LayerCLI})
	assertProvenanceLayers(t, composition.Provenance, "graphics.iterm.enabled", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerPrimary, LayerEnvironment, LayerProfile})
	sixel, ok := composition.Provenance.Lookup("graphics.sixel.enabled")
	if !ok || sixel.Winner.Layer != LayerCLI || !sixel.Winner.HasCLIArgumentIndex || sixel.Winner.CLIArgumentIndex != 14 {
		t.Fatalf("sixel provenance=%#v", sixel)
	}
	iterm, ok := composition.Provenance.Lookup("graphics.iterm.enabled")
	if !ok || iterm.Winner.Layer != LayerProfile || iterm.Winner.Name != "work" {
		t.Fatalf("iterm provenance=%#v", iterm)
	}
}

func TestGraphicsProtocolLeafAndTableUnsetRestoreDefaultsWithProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,graphics={sixel={enabled=true},iterm={enabled=true}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},graphics={sixel={enabled=unset},iterm=unset}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Graphics.Sixel.Enabled || cfg.Graphics.ITerm.Enabled {
		t.Fatalf("unset flags=%#v", cfg.Graphics)
	}
	for _, path := range []string{"graphics.sixel.enabled", "graphics.iterm.enabled"} {
		record, ok := composition.Provenance.Lookup(path)
		if !ok || !record.Tombstone || filepath.Base(record.Winner.CanonicalSource) != "primary.lua" {
			t.Fatalf("%s provenance=%#v ok=%v", path, record, ok)
		}
		assertProvenanceLayers(t, composition.Provenance, path, []ProvenanceLayer{LayerDefaults, LayerInclude, LayerPrimary})
	}
}

func TestGraphicsProtocolSchemaMetadataIsStrictV2CLIAndRestartOnly(t *testing.T) {
	fields, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]FieldMetadata, len(fields))
	for _, field := range fields {
		byPath[field.Path] = field
	}
	for _, path := range []string{"graphics.sixel.enabled", "graphics.iterm.enabled"} {
		field, ok := byPath[path]
		if !ok || field.Kind != KindBoolean || !field.Available || !field.CLIOverride || field.ApplyScope != ApplyRestart || field.RuntimeOverride {
			t.Fatalf("%s metadata=%#v ok=%v", path, field, ok)
		}
	}
	v1, err := SchemaFields(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range v1 {
		if field.Path == "graphics.sixel.enabled" || field.Path == "graphics.iterm.enabled" {
			t.Fatalf("v1 schema unexpectedly contains %s", field.Path)
		}
	}
}

func TestGraphicsCompositionUnsetProfileAndProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,graphics={kitty={enabled=true},limits={encoded_bytes_per_pane=1024,image_count_per_pane=8}}}`)
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},default_profile="small",graphics={limits={encoded_bytes_per_pane=2048,image_count_per_pane=unset}},profiles={small={graphics={limits={gpu_bytes_per_context=4096}}}}}`)
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
	if !cfg.Graphics.Kitty.Enabled || cfg.Graphics.Limits.EncodedBytesPerPane != 2048 || cfg.Graphics.Limits.ImageCountPerPane != termimage.HardImagesPerPane || cfg.Graphics.Limits.GPUBytesPerContext != 4096 {
		t.Fatalf("composed=%#v", cfg.Graphics)
	}
	record, ok := composition.Provenance.Lookup("graphics.limits.encoded_bytes_per_pane")
	if !ok || filepath.Base(record.Winner.CanonicalSource) != "primary.lua" {
		t.Fatalf("provenance=%#v", record)
	}
	profile, ok := composition.Provenance.Lookup("graphics.limits.gpu_bytes_per_context")
	if !ok || profile.Winner.Layer != LayerProfile || profile.Winner.Name != "small" {
		t.Fatalf("profile provenance=%#v", profile)
	}
}
