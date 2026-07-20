package config

import (
	"math"
	"testing"
)

func TestTerminalOpacityControlsAreV2Only(t *testing.T) {
	v1 := paddingDocument(t, `return { window = { text_opacity = 0.2, background_opacity = 0.3 } }`)
	gotV1 := FromDocument(Defaults(), v1)
	if gotV1.Window.TextOpacity != 1 || gotV1.Window.BackgroundOpacity != 1 {
		t.Fatalf("v1 applied v2 opacity fields: %#v", gotV1.Window)
	}
	v2 := paddingDocument(t, `return { config_version = 2, window = { text_opacity = 0.2, background_opacity = 0.3 } }`)
	gotV2 := FromDocument(Defaults(), v2)
	if gotV2.Window.TextOpacity != 0.2 || gotV2.Window.BackgroundOpacity != 0.3 {
		t.Fatalf("v2 opacity fields = %#v", gotV2.Window)
	}
}

func TestTerminalOpacityValidationAndEffectiveAlpha(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"negative text", func(c *Config) { c.Window.TextOpacity = -0.1 }},
		{"high background", func(c *Config) { c.Window.BackgroundOpacity = 1.1 }},
		{"nan text", func(c *Config) { c.Window.TextOpacity = math.NaN() }},
		{"native and framebuffer opacity", func(c *Config) { c.Window.BackgroundOpacity = 0.5; c.Window.Opacity = 0.9 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Colors.Background = "#102030FF"
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
	cfg := Defaults()
	cfg.Colors.Background = "#10203080"
	cfg.Window.BackgroundOpacity = 0.5
	if got := cfg.EffectiveBackgroundAlpha(); got != 64 {
		t.Fatalf("effective alpha = %d, want 64", got)
	}
}

func TestTerminalOpacitySchemaIsLiveAndRuntimeScoped(t *testing.T) {
	fields, err := SchemaFields(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"window.text_opacity", "window.background_opacity"} {
		found := false
		for _, field := range fields {
			if field.Path == path {
				found = true
				if field.ApplyScope != ApplyLive || !field.RuntimeOverride {
					t.Fatalf("%s metadata = %#v", path, field)
				}
			}
		}
		if !found {
			t.Fatalf("missing %s", path)
		}
	}
}
