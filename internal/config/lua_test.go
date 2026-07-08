package config

import (
	"path/filepath"
	"testing"
)

func TestLoadLuaConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	writeTestFile(t, path, `return {
  window = { width = 1200, height = 800, padding_x = 10, padding_y = 11 },
  font = { family = "Go Mono", size = 16 },
  colors = { foreground = "#ffffff", background = "#000000", cursor = "#112233" },
  scrolling = { history = 4000, wheel_multiplier = 5 },
  cursor = { shape = "beam", blink_interval_ms = 700, thickness = 0.2 },
  shell = { program = "pwsh", args = { "-NoLogo" }, env = { FOO = "bar" } },
}`)

	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatalf("LoadLua failed: %v", err)
	}
	if cfg.Window.Width != 1200 || cfg.Window.PaddingY != 11 || cfg.Font.Size != 16 {
		t.Fatalf("window/font overrides missing: %#v", cfg)
	}
	if cfg.Shell.Program != "pwsh" || len(cfg.Shell.Args) != 1 || cfg.Shell.Env["FOO"] != "bar" {
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
