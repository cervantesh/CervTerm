package config

import (
	"math"
	"strings"
	"testing"
)

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
	if cfg.Render.Damage != "rows" {
		t.Fatalf("render.damage default = %q, want rows", cfg.Render.Damage)
	}
	if cfg.Font.Ligatures {
		t.Fatal("font.ligatures must default to false")
	}
	if cfg.Clipboard.OSC52 != "off" {
		t.Fatalf("clipboard.osc52 default = %q, want off", cfg.Clipboard.OSC52)
	}
}

func TestANSIColorDefaultsAndValidation(t *testing.T) {
	want := [16]string{
		"#1B2232", "#FF5C8A", "#8BF59A", "#F8D866", "#7AA2FF", "#D88CFF", "#60E8F0", "#D8DEEA",
		"#57627A", "#FF7AA8", "#A6FFB5", "#FFE68A", "#9BB8FF", "#E5A7FF", "#90F4FF", "#FFFFFF",
	}
	cfg := Defaults()
	if cfg.Colors.ANSI != want {
		t.Fatalf("colors.ansi defaults = %#v, want %#v", cfg.Colors.ANSI, want)
	}
	cfg.Colors.ANSI[3] = "#01020380"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "colors.ansi[4]") {
		t.Fatalf("Validate() error = %v, want indexed ANSI color error", err)
	}
}

func TestValidateOSC52(t *testing.T) {
	for _, tt := range []struct {
		value   string
		wantErr bool
	}{{"write", false}, {"off", false}, {"read", true}, {"", true}, {"ask", true}} {
		t.Run(tt.value, func(t *testing.T) {
			cfg := Defaults()
			cfg.Clipboard.OSC52 = tt.value
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDamage(t *testing.T) {
	for _, tt := range []struct {
		value   string
		wantErr bool
	}{{"rows", false}, {"frame", false}, {"", true}, {"cell", true}} {
		t.Run(tt.value, func(t *testing.T) {
			cfg := Defaults()
			cfg.Render.Damage = tt.value
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
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

func TestValidateRejectsExcessiveScrollbackHistory(t *testing.T) {
	cfg := Defaults()
	cfg.Scrolling.History = MaxScrollbackHistory + 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected excessive scrollback history to fail validation")
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

func TestAppearanceDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Colors.Background != "#080B12E6" || cfg.Window.Opacity != 1 || !cfg.Window.Blur {
		t.Fatalf("unexpected appearance defaults: window=%#v colors=%#v", cfg.Window, cfg.Colors)
	}
	if !cfg.Scrollbar.Enabled || cfg.Scrollbar.ReservedWidthPX != 12 || cfg.Scrollbar.WidthPX != 8 || cfg.Scrollbar.MarginPX != 2 || cfg.Scrollbar.RadiusPX != 4 || cfg.Scrollbar.MinThumbPX != 24 {
		t.Fatalf("unexpected scrollbar geometry defaults: %#v", cfg.Scrollbar)
	}
	if cfg.Scrollbar.AutoHideDelayMS != 1000 || cfg.Scrollbar.FadeMS != 150 || cfg.Scrollbar.TrackClick != "page" {
		t.Fatalf("unexpected scrollbar behavior defaults: %#v", cfg.Scrollbar)
	}
}

func TestValidateAppearance(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*Config)
		wantErr bool
	}{
		{"rgb background", func(c *Config) { c.Colors.Background = "#010203" }, false},
		{"rgba background", func(c *Config) { c.Colors.Background = "#01020380" }, false},
		{"opacity zero opaque background", func(c *Config) { c.Colors.Background = "#010203"; c.Window.Opacity = 0 }, false},
		{"exclusive opacity modes", func(c *Config) { c.Window.Opacity = .8 }, true},
		{"opacity nan", func(c *Config) { c.Window.Opacity = math.NaN() }, true},
		{"opacity high", func(c *Config) { c.Window.Opacity = 1.01 }, true},
		{"bad rgba", func(c *Config) { c.Colors.Background = "#0102030" }, true},
		{"slot too narrow", func(c *Config) { c.Scrollbar.ReservedWidthPX = 11 }, true},
		{"radius too large", func(c *Config) { c.Scrollbar.RadiusPX = 5 }, true},
		{"bad track mode", func(c *Config) { c.Scrollbar.TrackClick = "center" }, true},
		{"bad thumb color", func(c *Config) { c.Scrollbar.ThumbColor = "cyan" }, true},
		{"bad semantic chrome color", func(c *Config) { c.Colors.SearchMatch = "amber" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			tt.edit(&cfg)
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestBackgroundAlpha(t *testing.T) {
	cfg := Defaults()
	if got := cfg.BackgroundAlpha(); got != 0xe6 {
		t.Fatalf("BackgroundAlpha() = %#x, want 0xe6", got)
	}
	cfg.Colors.Background = "#010203"
	if got := cfg.BackgroundAlpha(); got != 0xff {
		t.Fatalf("RGB BackgroundAlpha() = %#x, want 0xff", got)
	}
}
