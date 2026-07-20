package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBellConfigV2StrictAndLive(t *testing.T) {
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,bell={mode="visual",focus="always",throttle_ms=500,visual_duration_ms=180}}`), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bell != (BellConfig{Mode: "visual", Focus: "always", ThrottleMS: 500, VisualDurationMS: 180}) {
		t.Fatalf("bell = %#v", cfg.Bell)
	}
	base := Defaults()
	merged := MergeLiveConfig(base, cfg)
	if merged.Bell != cfg.Bell {
		t.Fatalf("live bell = %#v", merged.Bell)
	}
}

func TestBellConfigRejectsInvalidDocuments(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`return {config_version=1,bell={mode="visual"}}`, "requires config_version = 2"},
		{`return {config_version=2,bell=true}`, "bell: must be table"},
		{`return {config_version=2,bell={mode="flash"}}`, "bell.mode"},
		{`return {config_version=2,bell={focus="sometimes"}}`, "bell.focus"},
		{`return {config_version=2,bell={throttle_ms=-1}}`, "bell.throttle_ms"},
		{`return {config_version=2,bell={visual_duration_ms=49}}`, "bell.visual_duration_ms"},
		{`return {config_version=2,bell={unknown=true}}`, "bell.unknown: unknown field"},
	} {
		cfg, err := LoadLua(writeLuaDocument(t, test.body), Defaults())
		if err == nil {
			err = cfg.Validate()
		}
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: err=%v, want %q", test.body, err, test.want)
		}
	}
}

func TestBellConfigCompositionAndProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},default_profile="quiet",bell={focus="always"},profiles={quiet={bell={mode="disabled"}}}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,bell={mode="audible",throttle_ms=800,visual_duration_ms=200}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 2, Path: "bell.mode", Value: "visual"}}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Bell != (BellConfig{Mode: "visual", Focus: "always", ThrottleMS: 800, VisualDurationMS: 200}) {
		t.Fatalf("composed bell = %#v", cfg.Bell)
	}
	if record, ok := composition.Provenance.Lookup("bell.mode"); !ok || record.Path != "bell.mode" {
		t.Fatalf("bell provenance = %#v, ok=%v", record, ok)
	}
}
