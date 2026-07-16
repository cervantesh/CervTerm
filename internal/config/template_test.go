package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLuaLoadsAndValidates(t *testing.T) {
	template := DefaultLua()
	if !strings.Contains(template, "return {") || !strings.Contains(template, "font =") {
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
	for _, field := range []string{"opacity =", "blur =", "scrollbar =", "reserved_width_px =", "thumb_hover_color =", "track_click ="} {
		if !strings.Contains(template, field) {
			t.Fatalf("default Lua template missing %q", field)
		}
	}
}
