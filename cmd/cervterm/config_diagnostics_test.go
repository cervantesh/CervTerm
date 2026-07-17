package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/script"
)

func writeDiagnosticConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExplainConfigComposesFiltersProvenanceAndRedaction(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,colors={background="#080B12"},shell={env={TOKEN="never-print-this"}},profiles={work={window={opacity=0.8}}},default_profile="work"}`)
	profile := "work"
	options := script.CandidateOptions{Composition: config.CompositionOptions{
		Selection:    config.SelectionOptions{ProfileOverride: &profile},
		CLIOverrides: []config.CLIOverride{{ArgumentIndex: 5, Path: "window.opacity", Value: "0.7"}},
	}}
	var stdout, stderr bytes.Buffer
	exit := runExplainConfigTo(&stdout, &stderr, configDiagnosticOptions{ConfigPath: path, Candidate: options, Fields: []string{"window.opacity", "shell.env", "window.opacity"}})
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"format-version: 1", `profile: "work" [explicit]`, "window.opacity = 0.7", "shell.env = <redacted>", "layer=cli", "argument=5"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "never-print-this") || strings.Count(output, "window.opacity =") != 1 || strings.Contains(output, "font.family =") {
		t.Fatalf("filtered/redacted output leaked or duplicated:\n%s", output)
	}
}

func TestExplainConfigShowsSelectedSchemeProvenanceWithoutCatalogDump(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {
config_version=2,
color_scheme="selected",
color_schemes={
selected={background="#112233",indexed_colors={[16]="#161616"}},
unselected={background="#ABCDEF"},
},
}`)
	var stdout, stderr bytes.Buffer
	exit := runExplainConfigTo(&stdout, &stderr, configDiagnosticOptions{ConfigPath: path, Fields: []string{"color_scheme", "colors.background", "colors.indexed_colors"}})
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`color_scheme = "selected"`,
		"colors.background = \"#112233\"",
		`colors.indexed_colors = {"16":"#161616"}`,
		"layer=primary",
		`layer=color_scheme name="selected"`,
		"colors.indexed_colors[16]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "color_schemes") || strings.Contains(output, "#ABCDEF") {
		t.Fatalf("diagnostic dumped the local scheme catalog:\n%s", output)
	}
}

func TestExplainConfigRejectsUnknownAndV1Targets(t *testing.T) {
	v2 := writeDiagnosticConfig(t, `return {config_version=2,colors={background="#080B12"}}`)
	var stdout, stderr bytes.Buffer
	if exit := runExplainConfigTo(&stdout, &stderr, configDiagnosticOptions{ConfigPath: v2, Fields: []string{"unknown.path"}}); exit != 2 || !strings.Contains(stderr.String(), `unknown config field "unknown.path"`) {
		t.Fatalf("unknown exit=%d stderr=%q", exit, stderr.String())
	}
	v1 := writeDiagnosticConfig(t, `return {colors={background="#080B12"}}`)
	stdout.Reset()
	stderr.Reset()
	if exit := runExplainConfigTo(&stdout, &stderr, configDiagnosticOptions{ConfigPath: v1}); exit != 2 || !strings.Contains(stderr.String(), "requires config_version=2") {
		t.Fatalf("v1 exit=%d stderr=%q", exit, stderr.String())
	}
}

func TestLoadConfigDiagnosticDoctorBoundaries(t *testing.T) {
	report, cleanup, err := loadConfigDiagnostic(configDiagnosticOptions{DisableDiscovery: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if report.SourcePath != "" || report.Composition || report.AuthoredVersion != 0 {
		t.Fatalf("no-source report = %#v", report)
	}
	v1 := writeDiagnosticConfig(t, `return {colors={background="#080B12"}}`)
	report, cleanup, err = loadConfigDiagnostic(configDiagnosticOptions{ConfigPath: v1}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if report.AuthoredVersion != 1 || report.Composition {
		t.Fatalf("v1 report = %#v", report)
	}
}

func TestExplainConfigReportsDescriptorShadowing(t *testing.T) {
	path := writeDiagnosticConfig(t, `return {config_version=2,font={family="Legacy",descriptors={{family="Go Mono"}}}}`)
	var stdout, stderr bytes.Buffer
	exit := runExplainConfigTo(&stdout, &stderr, configDiagnosticOptions{ConfigPath: path, Fields: []string{"font.family", "font.descriptors"}})
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`font.family = "Legacy" [scope=restart shadowed_by=font.descriptors]`, `font.descriptors = [{"family":"Go Mono"`, `"weight":400`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
