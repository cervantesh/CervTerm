package config

import "fmt"

// AccessibilityConfig controls restart-scoped, visible-only platform accessibility.
type AccessibilityConfig struct {
	Enabled bool
	Scope   string
}

func (config AccessibilityConfig) validate() error {
	if config.Scope != "visible" {
		return fmt.Errorf("accessibility.scope must be visible")
	}
	return nil
}

func decodeV2PlatformConfig(document Document, cfg *Config) {
	if render := tableField(document.Root, "render"); render != nil {
		cfg.Render.MaxFPS = intField(render, "max_fps", cfg.Render.MaxFPS)
	}
	if imeConfig := tableField(document.Root, "ime"); imeConfig != nil {
		cfg.IME.Enabled = boolField(imeConfig, "enabled", cfg.IME.Enabled)
	}
	if accessibilityConfig := tableField(document.Root, "accessibility"); accessibilityConfig != nil {
		cfg.Accessibility.Enabled = boolField(accessibilityConfig, "enabled", cfg.Accessibility.Enabled)
		cfg.Accessibility.Scope = stringField(accessibilityConfig, "scope", cfg.Accessibility.Scope)
	}
}
