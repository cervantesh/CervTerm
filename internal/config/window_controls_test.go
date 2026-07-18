package config

import "testing"

func TestWindowControlsV2AndV1Isolation(t *testing.T) {
	v2 := paddingDocument(t, `return {config_version=2,window={initial_rows=24,initial_cols=80,decorations="none",titlebar="system"}}`)
	cfg := FromDocument(Defaults(), v2)
	if cfg.Window.InitialRows != 24 || cfg.Window.InitialCols != 80 || cfg.Window.Decorations != "none" || cfg.Window.Titlebar != "system" {
		t.Fatalf("v2 controls=%#v", cfg.Window)
	}
	v1 := paddingDocument(t, `return {window={initial_rows=24,initial_cols=80,decorations="none",titlebar="system"}}`)
	cfg = FromDocument(Defaults(), v1)
	if cfg.Window.InitialRows != 0 || cfg.Window.InitialCols != 0 || cfg.Window.Decorations != "system" || cfg.Window.Titlebar != "dark" {
		t.Fatalf("v1 isolation=%#v", cfg.Window)
	}
}

func TestWindowControlValidation(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Window.InitialRows = 24 }, func(c *Config) { c.Window.InitialRows, c.Window.InitialCols = 9, 80 },
		func(c *Config) { c.Window.Decorations = "custom" }, func(c *Config) { c.Window.Titlebar = "hidden" },
	} {
		cfg := Defaults()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid controls accepted: %#v", cfg.Window)
		}
	}
}
