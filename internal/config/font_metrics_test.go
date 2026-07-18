package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2FontMetricsDecodeAndV1Ignore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {config_version=2,font={line_height=1.5,cell_width=1.25,baseline_offset=2.5,glyph_offset_x=-3,glyph_offset_y=4}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.LineHeight != 1.5 || cfg.Font.CellWidth != 1.25 || cfg.Font.BaselineOffset != 2.5 || cfg.Font.GlyphOffsetX != -3 || cfg.Font.GlyphOffsetY != 4 {
		t.Fatalf("font metrics=%#v", cfg.Font)
	}
	writeTestFile(t, path, `return {font={line_height="ignored",cell_width=2}}`)
	cfg, err = LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.LineHeight != 1 || cfg.Font.CellWidth != 1 {
		t.Fatalf("v1 metrics leaked=%#v", cfg.Font)
	}
}

func TestFontMetricValidationPathsAndBoundaries(t *testing.T) {
	valid := Defaults()
	valid.Font.LineHeight, valid.Font.CellWidth = 0.5, 3
	valid.Font.BaselineOffset, valid.Font.GlyphOffsetX, valid.Font.GlyphOffsetY = -64, 64, 0
	if err := valid.Validate(); err != nil {
		t.Fatalf("metric boundaries rejected: %v", err)
	}
	cases := []struct{ field, value, want string }{
		{"line_height", "0.49", "line_height"}, {"cell_width", "3.01", "cell_width"},
		{"baseline_offset", "-65", "baseline_offset"}, {"glyph_offset_x", "65", "glyph_offset_x"},
		{"glyph_offset_y", `"bad"`, "glyph_offset_y"},
	}
	for _, test := range cases {
		t.Run(test.field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cervterm.lua")
			writeTestFile(t, path, fmt.Sprintf(`return {config_version=2,font={%s=%s}}`, test.field, test.value))
			_, err := LoadLua(path, Defaults())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestComposeAndCLIFontMetricsReplacementUnsetProvenance(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","high.lua"}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,font={line_height=1.5,cell_width=1.25,baseline_offset=2,glyph_offset_x=3,glyph_offset_y=4}}`)
	writeGraphLua(t, dir, "high.lua", `return {config_version=2,font={line_height=unset,baseline_offset=-2}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 2, Path: "font.glyph_offset_x", Value: "-5.5"}}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Font.LineHeight != 1 || cfg.Font.CellWidth != 1.25 || cfg.Font.BaselineOffset != -2 || cfg.Font.GlyphOffsetX != -5.5 || cfg.Font.GlyphOffsetY != 4 {
		t.Fatalf("composed metrics=%#v", cfg.Font)
	}
	assertProvenanceLayers(t, composition.Provenance, "font.line_height", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerInclude})
	assertProvenanceLayers(t, composition.Provenance, "font.glyph_offset_x", []ProvenanceLayer{LayerDefaults, LayerInclude, LayerCLI})
}

func TestFontMetricDiagnosticsAndRestartDiff(t *testing.T) {
	cfg := Defaults()
	cfg.Font.LineHeight, cfg.Font.CellWidth, cfg.Font.BaselineOffset, cfg.Font.GlyphOffsetX, cfg.Font.GlyphOffsetY = 1.5, 1.25, 2, -3, 4
	paths := []string{"font.line_height", "font.cell_width", "font.baseline_offset", "font.glyph_offset_x", "font.glyph_offset_y"}
	diagnostic, err := DiagnoseConfig(cfg, Provenance{}, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostic.Fields) != len(paths) {
		t.Fatalf("metric diagnostics=%#v", diagnostic.Fields)
	}
	changes := DiffConfig(cfg, Defaults())
	for _, path := range paths {
		found := false
		for _, change := range changes {
			if change.Path == path && change.Scope == ApplyRestart {
				found = true
			}
		}
		if !found {
			t.Fatalf("restart diff missing %s: %#v", path, changes)
		}
	}
}

func TestComposedV1IgnoresMalformedFontMetrics(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"legacy.lua"}}`)
	writeGraphLua(t, dir, "legacy.lua", `return {font={line_height="bad",cell_width=2}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Font.LineHeight != 1 || cfg.Font.CellWidth != 1 || composition.Document.Has("font.line_height") {
		t.Fatalf("v1 metrics leaked: %#v", cfg.Font)
	}
}
