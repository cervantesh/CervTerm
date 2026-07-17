package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"

	lua "github.com/yuin/gopher-lua"
)

func TestLoadV2FontDescriptorsNormalizesAndPreservesIndexZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {config_version=2,font={family="Legacy",descriptors={
  {family="  JetBrains   Mono  ",collection_index=0},
  {family="Fallback",collection_face="Italic",weight=700,style="italic",stretch=125,attribute_mode="fixed"}
}}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Font.Descriptors) != 2 {
		t.Fatalf("descriptors=%#v", cfg.Font.Descriptors)
	}
	first, second := cfg.Font.Descriptors[0], cfg.Font.Descriptors[1]
	if first.Family != "JetBrains Mono" || !first.CollectionIndex.Present || first.CollectionIndex.Value != 0 || first.Weight != 400 || first.Style != fontdesc.StyleNormal || first.Stretch != 100 || first.AttributeMode != fontdesc.AttributeModeAugment {
		t.Fatalf("first descriptor=%#v", first)
	}
	if second.CollectionFace != "Italic" || second.Weight != 700 || second.Style != fontdesc.StyleItalic || second.Stretch != 125 || second.AttributeMode != fontdesc.AttributeModeFixed {
		t.Fatalf("second descriptor=%#v", second)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestV1IgnoresFontDescriptors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {font={family="Legacy",descriptors="not-v2"}}`)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Font.Family != "Legacy" || len(cfg.Font.Descriptors) != 0 {
		t.Fatalf("v1 font=%#v", cfg.Font)
	}
}

func TestV2FontDescriptorValidationPaths(t *testing.T) {
	entries := []struct{ name, descriptor, want string }{
		{"not list", `"bad"`, "font.descriptors"},
		{"sparse", `{[2]={family="F"}}`, "dense"},
		{"entry type", `{1}`, "font.descriptors[1]"},
		{"missing family", `{{weight=400}}`, "font.descriptors[1].family"},
		{"empty family", `{{family=" "}}`, "font.descriptors[1].family"},
		{"unknown", `{{family="F",wat=true}}`, "font.descriptors[1].wat"},
		{"face type", `{{family="F",collection_face=1}}`, "font.descriptors[1].collection_face"},
		{"blank face", `{{family="F",collection_face=" "}}`, "font.descriptors[1].collection_face"},
		{"index fraction", `{{family="F",collection_index=1.5}}`, "font.descriptors[1].collection_index"},
		{"index range", `{{family="F",collection_index=256}}`, "font.descriptors[1].collection_index"},
		{"selectors", `{{family="F",collection_face="R",collection_index=0}}`, "font.descriptors[1].collection_index"},
		{"weight", `{{family="F",weight=99}}`, "font.descriptors[1].weight"},
		{"style", `{{family="F",style="roman"}}`, "font.descriptors[1].style"},
		{"stretch", `{{family="F",stretch=201}}`, "font.descriptors[1].stretch"},
		{"mode", `{{family="F",attribute_mode="replace"}}`, "font.descriptors[1].attribute_mode"},
	}
	for _, tt := range entries {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cervterm.lua")
			writeTestFile(t, path, fmt.Sprintf(`return {config_version=2,font={descriptors=%s}}`, tt.descriptor))
			_, err := LoadLua(path, Defaults())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want path/content %q", err, tt.want)
			}
		})
	}
}

func TestV2FontDescriptorCountBound(t *testing.T) {
	parts := make([]string, fontdesc.MaxPrimaryDescriptors+1)
	for i := range parts {
		parts[i] = `{family="F"}`
	}
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {config_version=2,font={descriptors={`+strings.Join(parts, ",")+`}}}`)
	if _, err := LoadLua(path, Defaults()); err == nil || !strings.Contains(err.Error(), "at most 32") {
		t.Fatalf("count error=%v", err)
	}
}

func TestComposeFontDescriptorsReplaceAndUnsetAtomically(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","replace.lua"}}`)
	writeGraphLua(t, dir, "low.lua", `return {config_version=2,font={descriptors={{family="A"},{family="B"}}}}`)
	writeGraphLua(t, dir, "replace.lua", `return {config_version=2,font={descriptors={{family="C"}}}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Descriptors) != 1 || cfg.Font.Descriptors[0].Family != "C" {
		t.Fatalf("composed descriptors=%#v", cfg.Font.Descriptors)
	}
	assertProvenanceLayers(t, composition.Provenance, "font.descriptors", []ProvenanceLayer{LayerInclude, LayerInclude})

	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"low.lua","reset.lua"}}`)
	writeGraphLua(t, dir, "reset.lua", `return {config_version=2,font={descriptors=unset}}`)
	state2, graph2, composition2 := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state2.Close()
	defer graph2.Close()
	cfg = FromDocument(Defaults(), composition2.Document)
	if len(cfg.Font.Descriptors) != 0 {
		t.Fatalf("unset descriptors=%#v", cfg.Font.Descriptors)
	}
	if record, ok := composition2.Provenance.Lookup("font.descriptors"); !ok || !record.Tombstone {
		t.Fatalf("unset provenance=%#v ok=%v", record, ok)
	}
}

func TestConfigCloneDetachesFontDescriptors(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Descriptors = []fontdesc.Descriptor{{Family: "Original"}}
	clone := cfg.Clone()
	clone.Font.Descriptors[0].Family = "Mutated"
	if cfg.Font.Descriptors[0].Family != "Original" {
		t.Fatal("descriptor slice aliased")
	}
}

func TestDiagnoseFontDescriptorsUsesCanonicalAtomicValue(t *testing.T) {
	cfg := Defaults()
	cfg.Font.Descriptors = []fontdesc.Descriptor{{Family: "Example", CollectionIndex: fontdesc.SomeCollectionIndex(0)}}
	diagnostic, err := DiagnoseConfig(cfg, Provenance{}, []string{"font.family", "font.descriptors"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostic.Fields) != 2 {
		t.Fatalf("fields=%#v", diagnostic.Fields)
	}
	if diagnostic.Fields[0].Metadata.Path != "font.family" || diagnostic.Fields[0].ShadowedBy != "font.descriptors" {
		t.Fatalf("family shadow diagnostic=%#v", diagnostic.Fields[0])
	}
	value := diagnostic.Fields[1].Value
	for _, want := range []string{`"family":"Example"`, `"collection_index":0`, `"weight":400`, `"style":"normal"`, `"stretch":100`, `"attribute_mode":"augment"`} {
		if !strings.Contains(value, want) {
			t.Fatalf("descriptor diagnostic missing %q: %s", want, value)
		}
	}
}

func TestCLIFontDescriptorOverrideIsTypedAtomicAndProvenanced(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,font={descriptors={{family="Low"}}}}`)
	options := CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 4, Path: "font.descriptors", Value: `[{"family":"CLI","collection_index":0,"weight":700,"style":"italic","stretch":125,"attribute_mode":"fixed"}]`}}}
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), options)
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if len(cfg.Font.Descriptors) != 1 || cfg.Font.Descriptors[0].Family != "CLI" || !cfg.Font.Descriptors[0].CollectionIndex.Present || cfg.Font.Descriptors[0].Weight != 700 {
		t.Fatalf("CLI descriptors=%#v", cfg.Font.Descriptors)
	}
	record, ok := composition.Provenance.Lookup("font.descriptors")
	if !ok || record.Winner.Layer != LayerCLI || !record.Winner.HasCLIArgumentIndex || record.Winner.CLIArgumentIndex != 4 {
		t.Fatalf("CLI provenance=%#v ok=%v", record, ok)
	}
}

func TestCLIFontDescriptorOverrideRejectsInvalidJSON(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	resolved, resolveErr := resolveCLIOverridePath("font.descriptors")
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	for _, raw := range []string{
		`[{"family":"F","unknown":true}]`,
		`[{"family":"F","collection_index":1e-400}]`,
		`[{"family":"F","weight":1.5}]`,
		`[{"family":"F","stretch":9007199254740993}]`,
		`[{"family":"F","weight":null}]`,
		`[{"family":null}]`,
	} {
		if _, _, err := decodeCLIOverrideValue(state, resolved, raw); err == nil {
			t.Fatalf("invalid descriptor JSON accepted: %s", raw)
		}
	}
}
