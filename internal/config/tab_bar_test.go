package config

import (
	"testing"

	"github.com/yuin/gopher-lua"
)

func TestTabBarDefaultsAndValidation(t *testing.T) {
	cfg := Defaults()
	if cfg.TabBar.Mode != "multiple" || cfg.TabBar.Position != "top" || cfg.TabBar.HeightPX != 28 || cfg.TabBar.MinWidthPX != 96 || cfg.TabBar.MaxWidthPX != 220 || !cfg.TabBar.ShowNewButton || !cfg.TabBar.ShowCloseButton {
		t.Fatalf("defaults=%#v", cfg.TabBar)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestTabBarValidationRejectsEnumsAndBounds(t *testing.T) {
	cases := []func(*Config){func(c *Config) { c.TabBar.Mode = "sometimes" }, func(c *Config) { c.TabBar.Position = "middle" }, func(c *Config) { c.TabBar.HeightPX = 17 }, func(c *Config) { c.TabBar.MinWidthPX = 10 }, func(c *Config) { c.TabBar.MaxWidthPX = c.TabBar.MinWidthPX - 1 }, func(c *Config) { c.TabBar.PaddingX = 65 }}
	for i, mutate := range cases {
		cfg := Defaults()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("case %d err=%v", i, err)
		}
	}
}
func TestLuaTabBarDecodesAllLeaves(t *testing.T) {
	state := lua.NewState()
	defer state.Close()
	if err := state.DoString(`cfg={tab_bar={mode="always",position="bottom",height_px=36,min_width_px=80,max_width_px=300,padding_x=12,show_new_button=false,show_close_button=false}}`); err != nil {
		t.Fatal(err)
	}
	cfg := FromTable(Defaults(), state.GetGlobal("cfg").(*lua.LTable))
	if cfg.TabBar.Mode != "always" || cfg.TabBar.Position != "bottom" || cfg.TabBar.HeightPX != 36 || cfg.TabBar.MinWidthPX != 80 || cfg.TabBar.MaxWidthPX != 300 || cfg.TabBar.PaddingX != 12 || cfg.TabBar.ShowNewButton || cfg.TabBar.ShowCloseButton {
		t.Fatalf("tab bar=%#v", cfg.TabBar)
	}
}
