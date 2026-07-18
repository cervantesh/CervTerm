package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLuaLoadsAndValidates(t *testing.T) {
	template := DefaultLua()
	if !strings.Contains(template, "return {") || !strings.Contains(template, "config_version = 2,") || !strings.Contains(template, "font =") {
		t.Fatalf("default Lua template missing expected sections:\n%s", template)
	}
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	cfg, err := LoadLua(path, Defaults())
	if err != nil {
		t.Fatalf("LoadLua(DefaultLua()) failed: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultLua config should validate: %v", err)
	}
}

func TestDefaultLuaContainsAppearanceSchema(t *testing.T) {
	template := DefaultLua()
	for _, field := range []string{"padding_left =", "padding_right =", "padding_top =", "padding_bottom =", "opacity =", "text_opacity =", "background_opacity =", "blur =", "scrollbar =", "reserved_width_px =", "thumb_hover_color =", "track_click ="} {
		if !strings.Contains(template, field) {
			t.Fatalf("default Lua template missing %q", field)
		}
	}
}

func TestDefaultLuaDocumentsTypedAndCallbackActions(t *testing.T) {
	template := DefaultLua()
	for _, fragment := range []string{
		`local cervterm = require("cervterm")`,
		"cervterm.action.CopySelection",
		"cervterm.action.ScrollPage(1)",
		"cervterm.action.Zoom(1)",
		`cervterm.action.SplitPane("columns")`,
		"cervterm.action.Multiple",
		`label = "Send greeting"`,
		"one-second watchdog",
	} {
		if !strings.Contains(template, fragment) {
			t.Fatalf("default Lua template missing %q", fragment)
		}
	}
}
