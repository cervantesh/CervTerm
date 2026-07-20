package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDiagnoseConfigUsesDeterministicSchemaLeafOrderAndJSONValues(t *testing.T) {
	cfg := Defaults()
	cfg.Shell.Args = []string{"pwsh", "-NoLogo"}
	diagnostic, err := DiagnoseConfig(cfg, Provenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, field := range metadata {
		if field.Available && field.Kind != KindTable {
			want = append(want, field.Path)
		}
	}
	got := make([]string, 0, len(diagnostic.Fields))
	values := make(map[string]string, len(diagnostic.Fields))
	for _, field := range diagnostic.Fields {
		got = append(got, field.Metadata.Path)
		values[field.Metadata.Path] = field.Value
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic order = %v, want %v", got, want)
	}
	checks := map[string]string{
		"config_version":           "2",
		"window.width":             "1100",
		"font.family":              `"Go Mono"`,
		"font.ligatures":           "false",
		"colors.chrome_background": `"#10141CF0"`,
		"colors.chrome_muted":      `"#A8B3C7FF"`,
		"colors.accent":            `"#60E8F0FF"`,
		"colors.split":             `"#4A5263FF"`,
		"colors.search_match":      `"#7A5C12FF"`,
		"colors.error":             `"#D87272FF"`,
		"shell.args":               `["pwsh","-NoLogo"]`,
	}
	for path, wantValue := range checks {
		if gotValue := values[path]; gotValue != wantValue {
			t.Fatalf("%s value = %q, want %q", path, gotValue, wantValue)
		}
	}
}

func TestDiagnoseConfigFiltersAreExactDeduplicatedAndSchemaOrdered(t *testing.T) {
	diagnostic, err := DiagnoseConfig(Defaults(), Provenance{}, []string{"render.vsync", "window.width", "render.vsync"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{diagnostic.Fields[0].Metadata.Path, diagnostic.Fields[1].Metadata.Path}
	want := []string{"window.width", "render.vsync"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered paths = %v, want %v", got, want)
	}
	for _, path := range []string{"window", "window.not_a_leaf"} {
		_, err := DiagnoseConfig(Defaults(), Provenance{}, []string{path})
		if err == nil || err.Error() != path {
			t.Fatalf("filter %q error = %v", path, err)
		}
	}
}

func TestDiagnoseConfigRedactsSensitiveValuesAndRuntimeBodies(t *testing.T) {
	const secret = "do-not-emit-this-secret"
	cfg := Defaults()
	cfg.Shell.Env = map[string]string{"TOKEN": secret}
	provenance := newProvenance()
	provenance.set(`shell.env["TOKEN"]`, ProvenanceOrigin{Layer: LayerPrimary, Name: "primary"}, false, true)
	provenance.set("keys", ProvenanceOrigin{Layer: LayerPrimary, Name: "primary"}, false, false)
	provenance.set("events.output", ProvenanceOrigin{Layer: LayerProfile, Name: "work"}, false, false)

	diagnostic, err := DiagnoseConfig(cfg, provenance, []string{"shell.env", "keys", "events"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("diagnostic leaked secret: %s", encoded)
	}
	values := map[string]string{}
	for _, field := range diagnostic.Fields {
		values[field.Metadata.Path] = field.Value
	}
	if values["shell.env"] != DiagnosticRedacted || values["keys"] != DiagnosticConfigured || values["events"] != DiagnosticConfigured {
		t.Fatalf("diagnostic markers = %#v", values)
	}
	if strings.Contains(string(encoded), "function") {
		t.Fatalf("diagnostic retained runtime body: %s", encoded)
	}

	unset, err := DiagnoseConfig(cfg, Provenance{}, []string{"keys", "events"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range unset.Fields {
		if field.Value != DiagnosticUnset {
			t.Fatalf("%s = %q, want %q", field.Metadata.Path, field.Value, DiagnosticUnset)
		}
	}
}

func TestDiagnoseConfigProvenanceChainIsDetachedAndValueFree(t *testing.T) {
	provenance := newProvenance()
	provenance.set("font.family", ProvenanceOrigin{Layer: LayerDefaults, Name: "built-in defaults"}, false, false)
	provenance.set("font.family", ProvenanceOrigin{Layer: LayerPrimary, Name: "primary", CanonicalSource: "config.lua", AuthoredVersion: 2, Version: 2}, false, false)

	diagnostic, err := DiagnoseConfig(Defaults(), provenance, []string{"font.family"})
	if err != nil {
		t.Fatal(err)
	}
	record := diagnostic.Fields[0].Provenance[0]
	if len(record.Overwritten) != 1 || record.Overwritten[0].Layer != LayerDefaults || record.Winner.Layer != LayerPrimary {
		t.Fatalf("provenance chain = %#v", record)
	}
	record.Overwritten[0].Name = "mutated"
	diagnostic.Fields[0].Provenance[0].Overwritten[0].Name = "also mutated"
	fresh, err := DiagnoseConfig(Defaults(), provenance, []string{"font.family"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.Fields[0].Provenance[0].Overwritten[0].Name; got != "built-in defaults" {
		t.Fatalf("stored provenance mutated through diagnostic: %q", got)
	}
	encoded, err := json.Marshal(fresh.Fields[0].Provenance)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Go Mono") {
		t.Fatalf("provenance retained config value: %s", encoded)
	}
}
