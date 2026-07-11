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
	if cfg.Render.Bidi {
		t.Fatal("render.bidi must default to false")
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

func TestValidateAcceptsBidiFlag(t *testing.T) {
	cfg := Defaults()
	cfg.Render.Bidi = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("render.bidi should validate: %v", err)
	}
}
