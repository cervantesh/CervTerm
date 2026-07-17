package main

import (
	"flag"
	"strings"
	"testing"
)

func parseCompositionFlags(t *testing.T, args ...string) (*compositionFlags, error) {
	t.Helper()
	flags := flag.NewFlagSet("cervterm-test", flag.ContinueOnError)
	values := registerCompositionFlags(flags)
	return values, flags.Parse(args)
}

func TestCompositionFlagsPreservePresenceEnvironmentAndOrder(t *testing.T) {
	args := []string{"--environment", "", "--profile=work", "--config-override", "window.opacity=0.8", "--config-override=font.family=A=B"}
	values, err := parseCompositionFlags(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(name string) (string, bool) {
		switch name {
		case "CERVTERM_ENV":
			return "windows", true
		case "CERVTERM_PROFILE":
			return "", true
		default:
			return "", false
		}
	}
	options, err := values.candidateOptions(args, lookup)
	if err != nil {
		t.Fatal(err)
	}
	selection := options.Composition.Selection
	if selection.EnvironmentOverride == nil || *selection.EnvironmentOverride != "" || selection.EnvironmentVariableValue == nil || *selection.EnvironmentVariableValue != "windows" {
		t.Fatalf("environment selection inputs = %#v", selection)
	}
	if selection.ProfileOverride == nil || *selection.ProfileOverride != "work" || selection.ProfileVariableValue == nil || *selection.ProfileVariableValue != "" {
		t.Fatalf("profile selection inputs = %#v", selection)
	}
	overrides := options.Composition.CLIOverrides
	if len(overrides) != 2 || overrides[0].ArgumentIndex != 4 || overrides[0].Path != "window.opacity" || overrides[0].Value != "0.8" || overrides[1].ArgumentIndex != 6 || overrides[1].Path != "font.family" || overrides[1].Value != "A=B" {
		t.Fatalf("CLI overrides = %#v", overrides)
	}
	if !values.explicitlyRequested() {
		t.Fatal("explicit request was not retained")
	}
}

func TestCompositionFlagsAbsentAndMalformedOverrideRedaction(t *testing.T) {
	values, err := parseCompositionFlags(t)
	if err != nil {
		t.Fatal(err)
	}
	options, err := values.candidateOptions(nil, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	selection := options.Composition.Selection
	if selection.EnvironmentOverride != nil || selection.EnvironmentVariableValue != nil || selection.ProfileOverride != nil || selection.ProfileVariableValue != nil || values.explicitlyRequested() {
		t.Fatalf("absent inputs = %#v", selection)
	}

	const raw = "super-secret-without-separator"
	values, err = parseCompositionFlags(t, "--config-override", raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = values.candidateOptions([]string{"--config-override", raw}, func(string) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "argument 1") || strings.Contains(err.Error(), raw) {
		t.Fatalf("redacted malformed override error = %v", err)
	}
}

func TestConfigOverrideArgumentIndexesStopAtDoubleDash(t *testing.T) {
	args := []string{"-version", "--config-override=x=1", "--config-override", "y=2", "--", "--config-override=z=3"}
	indexes := configOverrideArgumentIndexes(args)
	if len(indexes) != 2 || indexes[0] != 2 || indexes[1] != 3 {
		t.Fatalf("argument indexes = %v", indexes)
	}
}

func TestExplicitCompositionInputsRequireV2Source(t *testing.T) {
	values, err := parseCompositionFlags(t, "--profile", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompositionTarget(values, "", 0); err == nil || !strings.Contains(err.Error(), "explicit config_version=2 source") {
		t.Fatalf("missing source error = %v", err)
	}
	if err := validateCompositionTarget(values, "cervterm.lua", 1); err == nil || !strings.Contains(err.Error(), "require config_version=2") {
		t.Fatalf("v1 source error = %v", err)
	}
	if err := validateCompositionTarget(values, "cervterm.lua", 2); err != nil {
		t.Fatalf("v2 source error = %v", err)
	}
	absent, err := parseCompositionFlags(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompositionTarget(absent, "cervterm.lua", 1); err != nil {
		t.Fatalf("legacy v1 without explicit composition input = %v", err)
	}
}
