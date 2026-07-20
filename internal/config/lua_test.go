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
  font = { family = "Go Mono", size = 18, ligatures = true },
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
	if !cfg.Font.Ligatures {
		t.Fatal("FromTable did not apply font.ligatures")
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

func TestFromTableAppearanceAndScrollbar(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`cfg = {
	  window = { opacity = 0.75, blur = false },
	  colors = { background = "#010203FF" },
	  scrollbar = { enabled = false, reserved_width_px = 18, width_px = 10, margin_px = 4, radius_px = 5, min_thumb_px = 30, track_color = "#11111122", thumb_color = "#22222233", thumb_hover_color = "#33333344", thumb_press_color = "#44444455", auto_hide_delay_ms = 700, fade_ms = 90, page_step = 0.75, track_click = "jump" },
	}`); err != nil {
		t.Fatal(err)
	}
	cfg := FromTable(Defaults(), state.GetGlobal("cfg").(*lua.LTable))
	if cfg.Window.Opacity != .75 || cfg.Window.Blur || cfg.Colors.Background != "#010203FF" {
		t.Fatalf("appearance overrides missing: %#v %#v", cfg.Window, cfg.Colors)
	}
	if cfg.Scrollbar.Enabled || cfg.Scrollbar.ReservedWidthPX != 18 || cfg.Scrollbar.PageStep != .75 || cfg.Scrollbar.TrackClick != "jump" {
		t.Fatalf("scrollbar overrides missing: %#v", cfg.Scrollbar)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("overrides should validate: %v", err)
	}
}
