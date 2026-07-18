package config

import (
	"strings"
	"testing"

	"cervterm/internal/quickselect"
	lua "github.com/yuin/gopher-lua"
)

func decodeQuickSelect(t *testing.T, body string) (Config, error) {
	t.Helper()
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString("cfg=" + body); err != nil {
		t.Fatal(err)
	}
	doc, err := DecodeDocument("quick.lua", state.GetGlobal("cfg").(*lua.LTable))
	if err != nil {
		return Config{}, err
	}
	cfg := FromTable(Defaults(), doc.Root)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func TestQuickSelectRulesDecodeCompileAndClone(t *testing.T) {
	cfg, err := decodeQuickSelect(t, `{config_version=2,quick_select={rules={{id="issue",pattern="[A-Z]+-[0-9]+",action="copy",priority=10},{id="url",pattern="https://[^ ]+",action="open"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.QuickSelect.Rules) != 2 || len(cfg.QuickSelect.Compiled) != 2 || cfg.QuickSelect.Rules[0].Action != quickselect.ActionCopy {
		t.Fatalf("quick=%#v", cfg.QuickSelect)
	}
	clone := cfg.Clone()
	clone.QuickSelect.Rules[0].ID = "changed"
	if cfg.QuickSelect.Rules[0].ID == "changed" {
		t.Fatal("clone aliased rules")
	}
}

func TestQuickSelectRulesRejectInvalidCandidates(t *testing.T) {
	cases := []string{
		`{config_version=2,quick_select={rules={{id="x",pattern="[",action="copy"}}}}`,
		`{config_version=2,quick_select={rules={{id="x",pattern="x",action="run"}}}}`,
		`{config_version=2,quick_select={rules={{id="x",pattern="x",action="copy"},{id="x",pattern="y",action="copy"}}}}`,
		`{config_version=2,quick_select={rules={{id="` + strings.Repeat("x", 65) + `",pattern="x",action="copy"}}}}`,
	}
	for _, body := range cases {
		if _, err := decodeQuickSelect(t, body); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestQuickSelectMergeLiveConfigDetachesCompiledRules(t *testing.T) {
	source := Defaults()
	source.QuickSelect.Rules = []QuickSelectRule{{ID: "x", Pattern: "x", Action: quickselect.ActionCopy}}
	source.QuickSelect.Compiled, _ = PrepareQuickSelect(source.QuickSelect.Rules)
	merged := MergeLiveConfig(Defaults(), source)
	if len(merged.QuickSelect.Compiled) != 1 {
		t.Fatalf("merged=%#v", merged.QuickSelect)
	}
	merged.QuickSelect.Rules[0].ID = "changed"
	if source.QuickSelect.Rules[0].ID == "changed" {
		t.Fatal("merge aliased rules")
	}
}

func TestQuickSelectCompositionReplacesWinningRuleListWithFieldProvenance(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},quick_select={rules={{id="primary",pattern="P-[0-9]+",action="copy"}}}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,quick_select={rules={{id="base",pattern="B-[0-9]+",action="copy"}}}}`)
	state := lua.NewState()
	state.SetGlobal("unset", NewUnsetValue(state))
	defer state.Close()
	graph, err := BuildSourceGraph(state, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	composition, err := ComposeSourceGraph(state, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := FromTable(Defaults(), composition.Document.Root)
	if len(cfg.QuickSelect.Rules) != 1 || cfg.QuickSelect.Rules[0].ID != "primary" {
		t.Fatalf("rules=%#v", cfg.QuickSelect.Rules)
	}
	record, ok := composition.Provenance.Lookup("quick_select.rules[1].id")
	if !ok || !strings.HasSuffix(record.Winner.CanonicalSource, "primary.lua") {
		t.Fatalf("provenance=%#v ok=%v", record, ok)
	}
}
