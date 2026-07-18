package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"
)

func TestV2FontFeaturesDecodeValidateAndV1Ignore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {config_version=2,font={ligatures=true,features={liga=0,ss01=2,kern=1}}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Font.Features) != 3 || cfg.Font.Features["liga"] != 0 || cfg.Font.Features["ss01"] != 2 || cfg.Font.Features["kern"] != 1 {
		t.Fatalf("features=%#v", cfg.Font.Features)
	}
	set, err := fontdesc.NewFeatureSet(cfg.Font.Ligatures, cfg.Font.Features)
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := set.Value("clig"); value != 1 {
		t.Fatalf("compatibility projection clig=%d", value)
	}

	writeTestFile(t, path, `return {font={features="ignored"}}`)
	cfg, err = LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Font.Features) != 0 {
		t.Fatalf("v1 features leaked: %#v", cfg.Font.Features)
	}
}

func TestComposedV1IgnoresMalformedFontFeatures(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"legacy.lua"}}`)
	writeGraphLua(t, dir, "legacy.lua", `return {font={features="malformed legacy value"}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Features) != 0 || composition.Document.Has("font.features") {
		t.Fatalf("composed v1 features leaked: %#v %#v", cfg.Font.Features, composition.Document.Present)
	}
	if _, ok := composition.Provenance.Lookup(`font.features["liga"]`); ok {
		t.Fatal("ignored v1 features produced provenance")
	}
}

func TestV2FontFeatureValidationPaths(t *testing.T) {
	entries := []struct{ name, features, want string }{
		{"not map", `"bad"`, "font.features"},
		{"short tag", `{abc=1}`, `font.features["abc"]`},
		{"non integer", `{liga=1.5}`, `font.features["liga"]`},
		{"negative", `{liga=-1}`, "between 0"},
		{"too large", `{liga=65536}`, "65535"},
		{"wrong value", `{liga=true}`, `font.features["liga"]`},
	}
	for _, test := range entries {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cervterm.lua")
			writeTestFile(t, path, fmt.Sprintf(`return {config_version=2,font={features=%s}}`, test.features))
			_, err := LoadLua(path, Defaults())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestComposeFontFeaturesMergeTombstoneAndCLI(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","high.lua"},font={ligatures=true}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,font={features={liga=0,ss01=1,kern=1}}}`)
	writeGraphLua(t, dir, "high.lua", `return {config_version=2,font={features={liga=unset,ss01=2}}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{
		CLIOverrides: []CLIOverride{{ArgumentIndex: 3, Path: "font.features", Value: `{"ss01":null,"dlig":1}`}},
	})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Features) != 2 || cfg.Font.Features["kern"] != 1 || cfg.Font.Features["dlig"] != 1 {
		t.Fatalf("composed features=%#v", cfg.Font.Features)
	}
	if _, exists := cfg.Font.Features["liga"]; exists {
		t.Fatal("tombstoned liga overlay survived")
	}
	if _, exists := cfg.Font.Features["ss01"]; exists {
		t.Fatal("CLI tombstone did not remove ss01")
	}
	set, err := fontdesc.NewFeatureSet(cfg.Font.Ligatures, cfg.Font.Features)
	if err != nil {
		t.Fatal(err)
	}
	if liga, _ := set.Value("liga"); liga != 1 {
		t.Fatalf("tombstone did not reveal ligature projection: liga=%d", liga)
	}
	for path, layers := range map[string][]ProvenanceLayer{
		`font.features["liga"]`: {LayerInclude, LayerInclude},
		`font.features["ss01"]`: {LayerInclude, LayerInclude, LayerCLI},
		`font.features["dlig"]`: {LayerCLI},
	} {
		assertProvenanceLayers(t, composition.Provenance, path, layers)
	}
}

func TestConfigCloneDetachesFontFeatures(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Features["ss01"] = 1
	clone := cfg.Clone()
	clone.Font.Features["ss01"] = 2
	if cfg.Font.Features["ss01"] != 1 {
		t.Fatalf("feature map aliased through clone: %#v", cfg.Font.Features)
	}
}

func TestFontFeatureDiagnosticsAndRestartDiff(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Features = map[string]int{"ss01": 1, "liga": 0}
	diagnostic, err := DiagnoseConfig(cfg, Provenance{}, []string{"font.features"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostic.Fields) != 1 || diagnostic.Fields[0].Value != `{"liga":0,"ss01":1}` || diagnostic.Fields[0].Metadata.ApplyScope != ApplyRestart {
		t.Fatalf("feature diagnostic=%#v", diagnostic.Fields)
	}
	changes := DiffConfig(cfg, Defaults())
	found := false
	for _, change := range changes {
		if change.Path == "font.features" && change.Scope == ApplyRestart {
			found = true
		}
	}
	if !found {
		t.Fatalf("feature restart diff missing: %#v", changes)
	}
}
