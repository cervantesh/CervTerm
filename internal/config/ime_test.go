package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIMEConfigDefaultsStrictRestartAndComposition(t *testing.T) {
	if Defaults().IME.Enabled {
		t.Fatal("IME must remain disabled by default")
	}
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,ime={enabled=true}}`), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IME.Enabled {
		t.Fatal("ime.enabled was not decoded")
	}
	if MergeLiveConfig(Defaults(), cfg).IME.Enabled {
		t.Fatal("restart-scoped IME setting leaked into live merge")
	}
	metadata, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, field := range metadata {
		if field.Path == "ime.enabled" {
			found = field.Kind == KindBoolean && field.ApplyScope == ApplyRestart && !field.RuntimeOverride
		}
	}
	if !found {
		t.Fatal("missing restart-scoped ime.enabled metadata")
	}

	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,ime={enabled=true}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	if !FromDocument(Defaults(), composition.Document).IME.Enabled {
		t.Fatal("composed ime.enabled was not applied")
	}
	if _, ok := composition.Provenance.Lookup("ime.enabled"); !ok {
		t.Fatal("missing ime.enabled provenance")
	}
}

func TestIMEConfigRejectsInvalidDocuments(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`return {config_version=1,ime={enabled=true}}`, "requires config_version = 2"},
		{`return {config_version=2,ime=true}`, "ime: must be table"},
		{`return {config_version=2,ime={enabled="yes"}}`, "ime.enabled: must be boolean"},
		{`return {config_version=2,ime={unknown=true}}`, "ime.unknown: unknown field"},
	} {
		_, err := LoadLua(writeLuaDocument(t, test.body), Defaults())
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: err=%v want=%q", test.body, err, test.want)
		}
	}
}

func TestIMEDefaultTemplateRoundTripsDisabled(t *testing.T) {
	template := DefaultLua()
	if !strings.Contains(template, "ime = {") || !strings.Contains(template, "Native Windows IME/preedit integration") {
		t.Fatalf("default template missing IME contract:\n%s", template)
	}
	path := writeLuaDocument(t, template)
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IME.Enabled {
		t.Fatal("default template enabled native IME")
	}
}
