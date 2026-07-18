package config

import "testing"

func TestScrollbarModesV2ShadowLegacyAndV1Isolation(t *testing.T) {
	v2 := paddingDocument(t, `return {config_version=2,scrollbar={enabled=false,mode="always",stable_gutter=false,animation_fps=30}}`)
	cfg := FromDocument(Defaults(), v2)
	if cfg.Scrollbar.Mode != "always" || !cfg.Scrollbar.Enabled || cfg.Scrollbar.StableGutter || cfg.Scrollbar.AnimationFPS != 30 {
		t.Fatalf("v2 scrollbar = %#v", cfg.Scrollbar)
	}
	v1 := paddingDocument(t, `return {scrollbar={enabled=false,mode="always",stable_gutter=false,animation_fps=30}}`)
	cfg = FromDocument(Defaults(), v1)
	if cfg.Scrollbar.Mode != "never" || cfg.Scrollbar.Enabled || !cfg.Scrollbar.StableGutter || cfg.Scrollbar.AnimationFPS != 60 {
		t.Fatalf("v1 scrollbar isolation = %#v", cfg.Scrollbar)
	}
}

func TestScrollbarModeValidation(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(cfg *Config) { cfg.Scrollbar.Mode = "sometimes" },
		func(cfg *Config) { cfg.Scrollbar.AnimationFPS = 0 },
		func(cfg *Config) { cfg.Scrollbar.AnimationFPS = 241 },
	} {
		cfg := Defaults()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid scrollbar accepted: %#v", cfg.Scrollbar)
		}
	}
}
