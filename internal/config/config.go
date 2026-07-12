package config

import (
	"errors"
	"fmt"
	"regexp"
)

type Config struct {
	Window    WindowConfig
	Font      FontConfig
	Colors    ColorsConfig
	Scrolling ScrollingConfig
	Cursor    CursorConfig
	Clipboard ClipboardConfig
	Render    RenderConfig
	Shell     ShellConfig
}

type WindowConfig struct {
	Width        int
	Height       int
	PaddingX     int
	PaddingY     int
	DynamicTitle bool
}

type FontConfig struct {
	Family string
	Size   float64
}

type ColorsConfig struct {
	Foreground          string
	Background          string
	Cursor              string
	SelectionBackground string
}

type ScrollingConfig struct {
	History                int
	WheelMultiplier        int
	HideCursorWhenScrolled bool
}

type CursorConfig struct {
	Shape           string
	Blink           bool
	BlinkIntervalMS int
	Thickness       float64
}

type ClipboardConfig struct {
	OSC52 string
}

type RenderConfig struct {
	Bidi       bool
	TextGamma  float64
	TextDarken float64
}

type ShellConfig struct {
	Program          string
	Args             []string
	WorkingDirectory string
	Env              map[string]string
}

func Defaults() Config {
	return Config{
		Window:    WindowConfig{Width: 1100, Height: 720, PaddingX: 18, PaddingY: 44, DynamicTitle: true},
		Font:      FontConfig{Family: "Go Mono", Size: 14},
		Colors:    ColorsConfig{Foreground: "#E6E1D8", Background: "#080B12", Cursor: "#60E8F0", SelectionBackground: "#2A6377"},
		Scrolling: ScrollingConfig{History: 2000, WheelMultiplier: 3, HideCursorWhenScrolled: true},
		Cursor:    CursorConfig{Shape: "underline", Blink: true, BlinkIntervalMS: 1000, Thickness: 0.15},
		Clipboard: ClipboardConfig{OSC52: "write"},
		Render:    RenderConfig{Bidi: false, TextGamma: 1.4, TextDarken: 0.1},
		Shell:     ShellConfig{Args: []string{}, Env: map[string]string{}},
	}
}

func (c Config) Validate() error {
	var errs []error
	if c.Window.Width < 100 || c.Window.Height < 100 {
		errs = append(errs, errors.New("window width and height must be >= 100"))
	}
	if c.Window.PaddingX < 0 || c.Window.PaddingY < 0 {
		errs = append(errs, errors.New("window padding must be >= 0"))
	}
	if c.Font.Size <= 0 {
		errs = append(errs, errors.New("font size must be > 0"))
	}
	if c.Scrolling.History < 0 || c.Scrolling.WheelMultiplier <= 0 {
		errs = append(errs, errors.New("scrolling history must be >= 0 and wheel_multiplier > 0"))
	}
	if c.Cursor.Shape != "block" && c.Cursor.Shape != "underline" && c.Cursor.Shape != "beam" {
		errs = append(errs, fmt.Errorf("cursor shape %q must be block, underline, or beam", c.Cursor.Shape))
	}
	if c.Cursor.BlinkIntervalMS <= 0 || c.Cursor.Thickness <= 0 {
		errs = append(errs, errors.New("cursor blink_interval_ms and thickness must be > 0"))
	}
	if c.Clipboard.OSC52 != "write" && c.Clipboard.OSC52 != "off" {
		errs = append(errs, fmt.Errorf("clipboard.osc52 %q must be write or off", c.Clipboard.OSC52))
	}
	if c.Render.TextGamma < 0.5 || c.Render.TextGamma > 3.0 {
		errs = append(errs, errors.New("render.text_gamma must be between 0.5 and 3.0"))
	}
	if c.Render.TextDarken < 0.0 || c.Render.TextDarken > 0.5 {
		errs = append(errs, errors.New("render.text_darken must be between 0.0 and 0.5"))
	}
	for name, value := range map[string]string{"foreground": c.Colors.Foreground, "background": c.Colors.Background, "cursor": c.Colors.Cursor, "selection_background": c.Colors.SelectionBackground} {
		if !isHexColor(value) {
			errs = append(errs, fmt.Errorf("colors.%s must be #RRGGBB", name))
		}
	}
	return errors.Join(errs...)
}

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func isHexColor(value string) bool {
	return hexColorPattern.MatchString(value)
}
