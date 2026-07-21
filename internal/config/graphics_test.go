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
	if got.Kitty.Enabled || got.Limits.EncodedBytesPerPane != termimage.HardEncodedBytesPerPane || got.Limits.DecodedBytesPerPane != termimage.HardDecodedBytesPerPane || got.Limits.ImageCountPerPane != termimage.HardImagesPerPane || got.Limits.PlacementCountPerPane != termimage.HardPlacementsPerPane || got.Limits.GPUBytesPerContext != termimage.HardGPUBytesPerContext {
		t.Fatalf("defaults=%#v", got)
	}
}

func TestGraphicsStrictV2DecodeAndRestartDiff(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`return {config_version=2,graphics={kitty={enabled=true},limits={encoded_bytes_per_pane=1024,decoded_bytes_per_pane=2048,image_count_per_pane=3,placement_count_per_pane=4,gpu_bytes_per_context=4096}}}`); err != nil {
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
	if !got.Kitty.Enabled || got.Limits.EncodedBytesPerPane != 1024 || got.Limits.DecodedBytesPerPane != 2048 || got.Limits.ImageCountPerPane != 3 || got.Limits.PlacementCountPerPane != 4 || got.Limits.GPUBytesPerContext != 4096 {
		t.Fatalf("decoded=%#v", got)
	}
	changes := DiffConfig(cfg, Defaults())
	if len(changes) != 6 {
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

func TestGraphicsApprovedHardCapDefaultsAreAccepted(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	source := `return {config_version=2,graphics={kitty={enabled=false},limits={encoded_bytes_per_pane=8388608,decoded_bytes_per_pane=67108864,image_count_per_pane=256,placement_count_per_pane=1024,gpu_bytes_per_context=268435456}}}`
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
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`return {graphics={kitty={enabled=true}}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeDocument("v1.lua", state.Get(-1).(*lua.LTable)); err == nil || !strings.Contains(err.Error(), "requires config_version = 2") {
		t.Fatalf("v1 error=%v", err)
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
