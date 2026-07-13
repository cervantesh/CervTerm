package config

import (
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestLoadLuaConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {
  window = { width = 1200, height = 800, padding_x = 10, padding_y = 11 },
  font = { family = "Go Mono", size = 16 },
  colors = { foreground = "#ffffff", background = "#000000", cursor = "#112233" },
  scrolling = { history = 4000, wheel_multiplier = 5 },
  cursor = { shape = "beam", blink_interval_ms = 700, thickness = 0.2 },
  shell = { program = "cmd.exe", args = { "/Q" }, env = { FOO = "bar" } },
}`)

	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatalf("LoadLua failed: %v", err)
	}
	if cfg.Window.Width != 1200 || cfg.Window.PaddingY != 11 || cfg.Font.Size != 16 {
		t.Fatalf("window/font overrides missing: %#v", cfg)
	}
	if cfg.Shell.Program != "cmd.exe" || len(cfg.Shell.Args) != 1 || cfg.Shell.Env["FOO"] != "bar" {
		t.Fatalf("shell overrides missing: %#v", cfg.Shell)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loaded config should validate: %v", err)
	}
}

func TestLoadLuaRequiresReturnedTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.lua")
	writeTestFile(t, path, `return "bad"`)
	if _, err := LoadLua(path, Defaults()); err == nil {
		t.Fatalf("expected returned table error")
	}
}

func TestFromTable(t *testing.T) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer state.Close()
	if err := state.DoString(`cfg = {
  window = { width = 1300 },
  font = { family = "Go Mono", size = 18 },
  render = { bidi = true, text_gamma = 2.2, text_darken = 0.25, text_raster = "go", damage = "frame" },
  shell = { args = { "/Q", "/K" }, env = { FOO = "bar" } },
}`); err != nil {
		t.Fatalf("DoString failed: %v", err)
	}
	root := state.GetGlobal("cfg").(*lua.LTable)
	cfg := FromTable(Defaults(), root)
	if cfg.Window.Width != 1300 || cfg.Font.Size != 18 || cfg.Font.Family != "Go Mono" {
		t.Fatalf("FromTable did not apply scalar fields: %#v", cfg)
	}
	if !cfg.Render.Bidi {
		t.Fatal("FromTable did not apply render.bidi")
	}
	if cfg.Render.TextGamma != 2.2 || cfg.Render.TextDarken != 0.25 {
		t.Fatalf("FromTable did not apply text coverage fields: %#v", cfg.Render)
	}
	if cfg.Render.TextRaster != "go" {
		t.Fatalf("FromTable render.text_raster = %q, want go", cfg.Render.TextRaster)
	}
	if cfg.Render.Damage != "frame" {
		t.Fatalf("FromTable render.damage = %q, want frame", cfg.Render.Damage)
	}
	if len(cfg.Shell.Args) != 2 || cfg.Shell.Args[1] != "/K" || cfg.Shell.Env["FOO"] != "bar" {
		t.Fatalf("FromTable did not apply shell fields: %#v", cfg.Shell)
	}
}

func TestDefaultLuaRoundTrip(t *testing.T) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer state.Close()
	if err := state.DoString(DefaultLua()); err != nil {
		t.Fatalf("DefaultLua failed to parse: %v", err)
	}
	root, ok := state.Get(-1).(*lua.LTable)
	if !ok {
		t.Fatalf("DefaultLua returned %s, want table", state.Get(-1).Type())
	}
	cfg := FromTable(Config{}, root)
	defaults := Defaults()
	if cfg.Render.TextGamma != defaults.Render.TextGamma || cfg.Render.TextDarken != defaults.Render.TextDarken || cfg.Render.TextRaster != defaults.Render.TextRaster || cfg.Render.Damage != defaults.Render.Damage {
		t.Fatalf("round-trip render config = %#v, want %#v", cfg.Render, defaults.Render)
	}
}
