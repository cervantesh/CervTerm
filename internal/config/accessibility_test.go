package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccessibilityConfigDefaultsStrictAndRestartScoped(t *testing.T) {
	defaults := Defaults()
	if defaults.Accessibility.Enabled || defaults.Accessibility.Scope != "visible" {
		t.Fatalf("defaults=%#v", defaults.Accessibility)
	}
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,accessibility={enabled=true,scope="visible"}}`), defaults)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Accessibility.Enabled || cfg.Accessibility.Scope != "visible" {
		t.Fatalf("decoded=%#v", cfg.Accessibility)
	}
	live := MergeLiveConfig(defaults, cfg)
	if live.Accessibility != defaults.Accessibility {
		t.Fatal("restart-scoped accessibility leaked into live merge")
	}
	metadata, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, field := range metadata {
		if field.Path == "accessibility.enabled" || field.Path == "accessibility.scope" {
			found[field.Path] = field.ApplyScope == ApplyRestart && !field.RuntimeOverride
		}
	}
	if !found["accessibility.enabled"] || !found["accessibility.scope"] {
		t.Fatalf("metadata=%v", found)
	}
}

func TestAccessibilityCompositionProvenanceAndTemplate(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,accessibility={enabled=true,scope="visible"}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if !cfg.Accessibility.Enabled || cfg.Accessibility.Scope != "visible" {
		t.Fatalf("composed=%#v", cfg.Accessibility)
	}
	for _, path := range []string{"accessibility.enabled", "accessibility.scope"} {
		if _, ok := composition.Provenance.Lookup(path); !ok {
			t.Fatalf("missing provenance for %s", path)
		}
	}
	template := DefaultLua()
	if !strings.Contains(template, "accessibility = {") || !strings.Contains(template, "scope = \"visible\"") {
		t.Fatalf("default template missing accessibility contract:\n%s", template)
	}
	loaded, err := LoadLua(writeLuaDocument(t, template), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Accessibility != Defaults().Accessibility {
		t.Fatalf("template accessibility=%#v", loaded.Accessibility)
	}
}

func TestAccessibilityConfigRejectsInvalidDocuments(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`return {config_version=1,accessibility={enabled=true}}`, "requires config_version = 2"},
		{`return {config_version=2,accessibility=true}`, "accessibility: must be table"},
		{`return {config_version=2,accessibility={enabled="yes"}}`, "accessibility.enabled: must be boolean"},
		{`return {config_version=2,accessibility={unknown=true}}`, "accessibility.unknown: unknown field"},
	} {
		_, err := LoadLua(writeLuaDocument(t, test.body), Defaults())
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: err=%v want=%q", test.body, err, test.want)
		}
	}
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,accessibility={scope="all"}}`), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "accessibility.scope must be visible") {
		t.Fatalf("scope validation err=%v", err)
	}
}

func TestAccessibilityEvaluationDoesNotMutateConfigOrPersistState(t *testing.T) {
	path := writeLuaDocument(t, `return {config_version=2,accessibility={enabled=true,scope="visible"}}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	_ = DiffConfig(Defaults(), cfg)
	_ = MergeLiveConfig(Defaults(), cfg)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || beforeInfo.Mode() != afterInfo.Mode() || beforeInfo.Size() != afterInfo.Size() {
		t.Fatalf("accessibility evaluation mutated config: before=%q after=%q", before, after)
	}
}
