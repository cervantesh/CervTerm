package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLayoutPersistenceDefaultsDisabled(t *testing.T) {
	got := Defaults().LayoutPersistence
	if got != (LayoutPersistenceConfig{Enabled: false, Path: ""}) {
		t.Fatalf("layout persistence defaults = %#v", got)
	}
}

func TestLayoutPersistenceV2OnlyStrictLoadAndValidation(t *testing.T) {
	cfg, err := LoadLua(writeLuaDocument(t, `return {config_version=2,layout_persistence={enabled=true,path="state/layout.json"}}`), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayoutPersistence != (LayoutPersistenceConfig{Enabled: true, Path: "state/layout.json"}) {
		t.Fatalf("layout persistence = %#v", cfg.LayoutPersistence)
	}

	for _, test := range []struct {
		name, source, want string
	}{
		{"v1", `return {config_version=1,layout_persistence={enabled=true}}`, "requires config_version = 2"},
		{"wrong table", `return {config_version=2,layout_persistence=true}`, "layout_persistence: must be table"},
		{"wrong enabled", `return {config_version=2,layout_persistence={enabled="yes"}}`, "layout_persistence.enabled: must be boolean"},
		{"wrong path", `return {config_version=2,layout_persistence={path=false}}`, "layout_persistence.path: must be string"},
		{"unknown", `return {config_version=2,layout_persistence={file="x"}}`, "layout_persistence.file: unknown field"},
		{"too long", `return {config_version=2,layout_persistence={path=string.rep("x",4097)}}`, "at most 4096 bytes"},
		{"control", `return {config_version=2,layout_persistence={path="bad\npath"}}`, "must not contain NUL or control"},
		{"invalid utf8", `return {config_version=2,layout_persistence={path=string.char(255)}}`, "valid UTF-8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadLua(writeLuaDocument(t, test.source), Defaults())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLayoutPersistenceCompositionProfileAndCLI(t *testing.T) {
	dir := t.TempDir()
	writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},default_profile="selected",layout_persistence={path="primary.json"},profiles={selected={layout_persistence={enabled=true}}}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,layout_persistence={enabled=false,path="base.json"}}`)
	state, graph, composition := buildComposition(t, filepath.Join(dir, "primary.lua"), CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 3, Path: "layout_persistence.enabled", Value: "false"}}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.LayoutPersistence != (LayoutPersistenceConfig{Enabled: false, Path: "primary.json"}) {
		t.Fatalf("composed layout persistence = %#v", cfg.LayoutPersistence)
	}
	if record, ok := composition.Provenance.Lookup("layout_persistence.path"); !ok || !record.Sensitive {
		t.Fatalf("path provenance = %#v, ok=%v", record, ok)
	}

	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 4, Path: "layout_persistence.path", Value: "secret.json"}}}); err == nil || !strings.Contains(err.Error(), "sensitive fields cannot be supplied") {
		t.Fatalf("sensitive path CLI error = %v", err)
	}
}

func TestLayoutPersistenceSchemaAndDiff(t *testing.T) {
	v1, err := SchemaFields(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range v1 {
		if strings.HasPrefix(field.Path, "layout_persistence") {
			t.Fatalf("v1 exposed layout persistence: %#v", field)
		}
	}
	v2, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]FieldMetadata{}
	for _, field := range v2 {
		metadata[field.Path] = field
	}
	enabled := metadata["layout_persistence.enabled"]
	path := metadata["layout_persistence.path"]
	if enabled.Kind != KindBoolean || enabled.ApplyScope != ApplyRestart || !enabled.CLIOverride || enabled.Sensitive || enabled.RuntimeOverride {
		t.Fatalf("enabled metadata = %#v", enabled)
	}
	if path.Kind != KindString || path.ApplyScope != ApplyRestart || path.CLIOverride || !path.Sensitive || path.RuntimeOverride {
		t.Fatalf("path metadata = %#v", path)
	}

	base := Defaults()
	desired := base.Clone()
	desired.LayoutPersistence = LayoutPersistenceConfig{Enabled: true, Path: "layout.json"}
	want := []ConfigChange{{Path: "layout_persistence.enabled", Scope: ApplyRestart}, {Path: "layout_persistence.path", Scope: ApplyRestart}}
	if got := DiffConfig(desired, base); !reflect.DeepEqual(got, want) {
		t.Fatalf("diff = %#v, want %#v", got, want)
	}
	if got := MergeLiveConfig(base, desired).LayoutPersistence; got != base.LayoutPersistence {
		t.Fatalf("live merge mutated layout persistence: %#v", got)
	}
}

func TestLoadingLayoutPersistencePerformsNoLayoutIOOrWatchRegistration(t *testing.T) {
	dir := t.TempDir()
	layoutPath := filepath.Join(dir, "layout-state.json")
	configPath := filepath.Join(dir, "cervterm.lua")
	writeTestFile(t, configPath, `return {config_version=2,layout_persistence={enabled=true,path=`+luaQuote(layoutPath)+`}}`)
	cfg, err := LoadLua(configPath, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cfg.LayoutPersistence.Path) != filepath.Clean(layoutPath) {
		t.Fatalf("path = %q", cfg.LayoutPersistence.Path)
	}
	if _, err := os.Stat(layoutPath); !os.IsNotExist(err) {
		t.Fatalf("layout path was touched: %v", err)
	}

	state, graph, _ := buildComposition(t, configPath, CompositionOptions{})
	defer state.Close()
	defer graph.Close()
	for _, dependency := range graph.Dependencies {
		if filepath.Clean(dependency.Canonical) == filepath.Clean(layoutPath) || filepath.Clean(dependency.Selected) == filepath.Clean(layoutPath) {
			t.Fatalf("layout path registered as dependency: %#v", dependency)
		}
	}
	if _, err := os.Stat(layoutPath); !os.IsNotExist(err) {
		t.Fatalf("composition touched layout path: %v", err)
	}
}
