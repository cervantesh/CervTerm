package config

import "testing"

func TestDefaultsValidate(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}
	if cfg.Window.Width != 1100 || cfg.Font.Size != 14 || cfg.Scrolling.WheelMultiplier != 3 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	cfg := Defaults()
	cfg.Window.Width = 10
	cfg.Colors.Foreground = "red"
	cfg.Cursor.Shape = "triangle"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation errors")
	}
}
