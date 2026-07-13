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
	if cfg.Render.TextGamma != 1.15 || cfg.Render.TextDarken != 0.0 {
		t.Fatalf("unexpected text coverage defaults: %#v", cfg.Render)
	}
	if cfg.Render.TextRaster != "go" {
		t.Fatalf("render.text_raster default = %q, want auto", cfg.Render.TextRaster)
	}
}

func TestValidateTextRaster(t *testing.T) {
	for _, tt := range []struct {
		value   string
		wantErr bool
	}{{"auto", false}, {"go", false}, {"subpixel", false}, {"directwrite", true}, {"", true}} {
		t.Run(tt.value, func(t *testing.T) {
			cfg := Defaults()
			cfg.Render.TextRaster = tt.value
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
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

func TestValidateTextCoverage(t *testing.T) {
	tests := []struct {
		name          string
		gamma, darken float64
		wantErr       bool
	}{
		{"minimums", 0.5, 0, false},
		{"maximums", 3, 0.5, false},
		{"defaults", 1.4, 0.1, false},
		{"gamma too low", 0.49, 0.1, true},
		{"gamma too high", 3.01, 0.1, true},
		{"darken too low", 1.4, -0.01, true},
		{"darken too high", 1.4, 0.51, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Render.TextGamma = tt.gamma
			cfg.Render.TextDarken = tt.darken
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
