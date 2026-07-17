package config

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestCLIOverridesApplyAfterProfileLeftToRight(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,colors={background="#000000"},default_profile="work",profiles={work={window={opacity=0.9},font={family="Profile"}}}}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{CLIOverrides: []CLIOverride{
		{ArgumentIndex: 3, Path: "window.opacity", Value: "0.8"},
		{ArgumentIndex: 4, Path: "font.family", Value: "JetBrains Mono"},
		{ArgumentIndex: 5, Path: "font.ligatures", Value: "false"},
		{ArgumentIndex: 6, Path: "scrolling.history", Value: "1234"},
		{ArgumentIndex: 7, Path: "shell.args", Value: `["pwsh","-NoLogo"]`},
		{ArgumentIndex: 8, Path: "window.opacity", Value: "0.75"},
	}})
	defer state.Close()
	defer graph.Close()
	cfg := FromDocument(Defaults(), composition.Document)
	if cfg.Window.Opacity != 0.75 || cfg.Font.Family != "JetBrains Mono" || cfg.Font.Ligatures || cfg.Scrolling.History != 1234 || strings.Join(cfg.Shell.Args, ",") != "pwsh,-NoLogo" {
		t.Fatalf("CLI-composed config window=%#v font=%#v scrolling=%#v shell=%#v", cfg.Window, cfg.Font, cfg.Scrolling, cfg.Shell)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("CLI-composed config invalid: %v", err)
	}
	opacity, _ := composition.Provenance.Lookup("window.opacity")
	if got := provenanceLayers(composition.Provenance, "window.opacity"); !equalLayers(got, []ProvenanceLayer{LayerDefaults, LayerProfile, LayerCLI, LayerCLI}) {
		t.Fatalf("opacity layers = %v", got)
	}
	if !opacity.Winner.HasCLIArgumentIndex || opacity.Winner.CLIArgumentIndex != 8 || opacity.Winner.Name != "--config-override" || opacity.Winner.RequestedSource != "" || opacity.Winner.CanonicalSource != "" {
		t.Fatalf("opacity CLI provenance = %#v", opacity)
	}
	previousCLI := opacity.Overwritten[len(opacity.Overwritten)-1]
	if previousCLI.Layer != LayerCLI || !previousCLI.HasCLIArgumentIndex || previousCLI.CLIArgumentIndex != 3 {
		t.Fatalf("previous CLI provenance = %#v", previousCLI)
	}
	opacity.Winner.CLIArgumentIndex = 99
	fresh, _ := composition.Provenance.Lookup("window.opacity")
	if fresh.Winner.CLIArgumentIndex != 8 {
		t.Fatalf("external provenance mutation changed stored audit data: %#v", fresh)
	}
}

func TestCLIOverrideValidationAndRedaction(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2}`)
	tests := []struct {
		name     string
		override CLIOverride
		want     string
	}{
		{name: "unknown", override: CLIOverride{ArgumentIndex: 1, Path: "font.missing", Value: "12"}, want: "unknown configuration path"},
		{name: "whitespace path", override: CLIOverride{ArgumentIndex: 1, Path: " font.size", Value: "12"}, want: "canonical dotted field"},
		{name: "record", override: CLIOverride{ArgumentIndex: 1, Path: "font", Value: `{}`}, want: "not CLI-overridable"},
		{name: "keys", override: CLIOverride{ArgumentIndex: 1, Path: "keys", Value: `[]`}, want: "not CLI-overridable"},
		{name: "events", override: CLIOverride{ArgumentIndex: 1, Path: "events", Value: `{}`}, want: "not CLI-overridable"},
		{name: "sensitive map", override: CLIOverride{ArgumentIndex: 1, Path: "shell.env", Value: "SUPER_SECRET"}, want: "sensitive fields"},
		{name: "sensitive key", override: CLIOverride{ArgumentIndex: 1, Path: "shell.env.TOKEN", Value: "SUPER_SECRET"}, want: "sensitive fields"},
		{name: "bad bool", override: CLIOverride{ArgumentIndex: 1, Path: "font.ligatures", Value: "yes"}, want: "JSON boolean"},
		{name: "null bool", override: CLIOverride{ArgumentIndex: 1, Path: "font.ligatures", Value: "null"}, want: "JSON boolean"},
		{name: "fraction integer", override: CLIOverride{ArgumentIndex: 1, Path: "scrolling.history", Value: "1.5"}, want: "JSON integer"},
		{name: "trailing integer", override: CLIOverride{ArgumentIndex: 1, Path: "scrolling.history", Value: "12 13"}, want: "JSON integer"},
		{name: "mixed list", override: CLIOverride{ArgumentIndex: 1, Path: "shell.args", Value: `["pwsh",2]`}, want: "array of strings"},
		{name: "null list", override: CLIOverride{ArgumentIndex: 1, Path: "shell.args", Value: `null`}, want: "array of strings"},
		{name: "negative index", override: CLIOverride{ArgumentIndex: -1, Path: "font.size", Value: "12"}, want: "negative argument index"},
		{name: "malformed quoted string", override: CLIOverride{ArgumentIndex: 1, Path: "font.family", Value: `"unterminated`}, want: "JSON string"},
	}
	if strconv.IntSize == 64 {
		tests = append(tests, struct {
			name     string
			override CLIOverride
			want     string
		}{name: "precision loss integer", override: CLIOverride{ArgumentIndex: 1, Path: "scrolling.history", Value: "9007199254740993"}, want: "exactly representable"})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := lua.NewState()
			defer state.Close()
			graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
			if err != nil {
				t.Fatal(err)
			}
			defer graph.Close()
			_, err = ComposeSourceGraph(state, graph, CompositionOptions{CLIOverrides: []CLIOverride{test.override}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "SUPER_SECRET") {
				t.Fatalf("error leaked raw sensitive value: %v", err)
			}
		})
	}
}

func TestCLIOverrideNodeLimitAndQuotedString(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2}`)
	state := lua.NewState()
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	overrides := []CLIOverride{
		{ArgumentIndex: 1, Path: "shell.args", Value: `["a","b","c"]`},
		{ArgumentIndex: 2, Path: "font.family", Value: `"Quoted Font"`},
	}
	if _, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 4, CLIOverrides: overrides}); err == nil || !strings.Contains(err.Error(), "node/list-entry limit 4") {
		t.Fatalf("CLI node limit error = %v", err)
	}
	composition, err := ComposeSourceGraph(state, graph, CompositionOptions{MaxMergedNodes: 5, CLIOverrides: overrides})
	if err != nil || composition.NodeCount != 5 {
		t.Fatalf("CLI exact limit nodes=%d err=%v", composition.NodeCount, err)
	}
	if cfg := FromDocument(Defaults(), composition.Document); cfg.Font.Family != "Quoted Font" {
		t.Fatalf("quoted string = %q", cfg.Font.Family)
	}
}

func TestSchemaMetadataAdvertisesCLIOverrideCapabilities(t *testing.T) {
	fields, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := make(map[string]FieldMetadata, len(fields))
	for _, field := range fields {
		capabilities[field.Path] = field
	}
	for _, path := range []string{"window.opacity", "font.family", "font.size", "shell.args"} {
		if !capabilities[path].CLIOverride {
			t.Fatalf("%s should be CLI-overridable: %#v", path, capabilities[path])
		}
	}
	for _, path := range []string{"font", "shell.env", "keys", "events", "profiles"} {
		if capabilities[path].CLIOverride {
			t.Fatalf("%s must not be CLI-overridable: %#v", path, capabilities[path])
		}
	}
	if !capabilities["shell.env"].Sensitive {
		t.Fatal("shell.env must remain sensitive")
	}
}

func TestCLIOverrideDoesNotChangeSourceIdentity(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2}`)
	state, graph, composition := buildComposition(t, primary, CompositionOptions{CLIOverrides: []CLIOverride{{ArgumentIndex: 1, Path: "font.family", Value: "Mono"}}})
	defer state.Close()
	defer graph.Close()
	if composition.Document.Source != canonicalTestSource(t, filepath.Join(dir, "primary.lua")) {
		t.Fatalf("composition source = %q", composition.Document.Source)
	}
}
