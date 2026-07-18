package config

import (
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func decodeLaunchMenu(t *testing.T, body string) (Config, error) {
	t.Helper()
	s := lua.NewState()
	defer s.Close()
	if err := s.DoString("cfg=" + body); err != nil {
		t.Fatal(err)
	}
	doc, err := DecodeDocument("launch.lua", s.GetGlobal("cfg").(*lua.LTable))
	if err != nil {
		return Config{}, err
	}
	cfg := FromTable(Defaults(), doc.Root)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func TestLaunchMenuDecodeCloneAndBounds(t *testing.T) {
	cfg, err := decodeLaunchMenu(t, `{config_version=2,launch_menu={{id="pwsh",label="PowerShell",program="pwsh",args={"-NoLogo","a b"},cwd="C:/work",env={TOKEN="secret"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LaunchMenu) != 1 || cfg.LaunchMenu[0].Args[1] != "a b" || cfg.LaunchMenu[0].Env["TOKEN"] != "secret" {
		t.Fatalf("menu=%#v", cfg.LaunchMenu)
	}
	clone := cfg.Clone()
	clone.LaunchMenu[0].Args[0] = "changed"
	clone.LaunchMenu[0].Env["TOKEN"] = "changed"
	if cfg.LaunchMenu[0].Args[0] == "changed" || cfg.LaunchMenu[0].Env["TOKEN"] == "changed" {
		t.Fatal("clone aliases launch target")
	}
}

func TestLaunchMenuRejectsUnsafeOrOversizedDescriptors(t *testing.T) {
	cases := []string{
		`{config_version=2,launch_menu={{id="x",label="X",program="tool"},{id="x",label="Y",program="other"}}}`,
		`{config_version=2,launch_menu={{id="x",label="X",program="bad\0tool"}}}`,
		`{config_version=2,launch_menu={{id="` + strings.Repeat("x", 65) + `",label="X",program="tool"}}}`,
		`{config_version=2,launch_menu={{id="x",label="X",program="tool",unknown=true}}}`,
	}
	for _, body := range cases {
		if _, err := decodeLaunchMenu(t, body); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestLaunchMenuCompositionReplacesListAndMarksEnvSensitive(t *testing.T) {
	dir := t.TempDir()
	primary := writeGraphLua(t, dir, "primary.lua", `return {config_version=2,includes={"base.lua"},launch_menu={{id="new",label="New",program="new",env={TOKEN="secret"}}}}`)
	writeGraphLua(t, dir, "base.lua", `return {config_version=2,launch_menu={{id="old",label="Old",program="old"}}}`)
	s := lua.NewState()
	s.SetGlobal("unset", NewUnsetValue(s))
	defer s.Close()
	graph, err := BuildSourceGraph(s, primary, DefaultSourceGraphOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	composition, err := ComposeSourceGraph(s, graph, CompositionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := FromTable(Defaults(), composition.Document.Root)
	if len(cfg.LaunchMenu) != 1 || cfg.LaunchMenu[0].ID != "new" {
		t.Fatalf("menu=%#v", cfg.LaunchMenu)
	}
	record, ok := composition.Provenance.Lookup(`launch_menu[1].env["TOKEN"]`)
	if !ok || !record.Sensitive {
		t.Fatalf("record=%#v ok=%v", record, ok)
	}
}
