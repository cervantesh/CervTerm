package config

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

const MaxScrollbackHistory = 10_000

type Config struct {
	Window      WindowConfig
	Font        FontConfig
	ColorScheme string `json:",omitempty"`
	Colors      ColorsConfig
	Scrolling   ScrollingConfig
	Scrollbar   ScrollbarConfig
	Cursor      CursorConfig
	Clipboard   ClipboardConfig
	Render      RenderConfig
	Shell       ShellConfig
}

type WindowConfig struct {
	Width        int
	Height       int
	PaddingX     int
	PaddingY     int
	DynamicTitle bool
	Opacity      float64
	Blur         bool
}

type FontConfig struct {
	Family    string
	Size      float64
	Ligatures bool
}

type ColorsConfig struct {
	Foreground          string
	Background          string
	Cursor              string
	SelectionBackground string
	ChromeBackground    string
	ChromeMuted         string
	Accent              string
	Split               string
	SearchMatch         string
	Error               string
	ANSI                [16]string
	IndexedColors       IndexedColorOverrides
}

type ScrollingConfig struct {
	History                int
	WheelMultiplier        int
	HideCursorWhenScrolled bool
}

type ScrollbarConfig struct {
	Enabled         bool
	ReservedWidthPX int
	WidthPX         int
	MarginPX        int
	RadiusPX        int
	MinThumbPX      int
	TrackColor      string
	ThumbColor      string
	ThumbHoverColor string
	ThumbPressColor string
	AutoHideDelayMS int
	FadeMS          int
	PageStep        float64
	TrackClick      string
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
	Bidi            bool
	TextGamma       float64
	TextDarken      float64
	TextRaster      string
	StatsHotkey     string
	ZoomInHotkey    string
	ZoomOutHotkey   string
	ZoomResetHotkey string
	VSync           bool
	Redraw          string
	Damage          string
}

type ShellConfig struct {
	Program          string
	Args             []string
	WorkingDirectory string
	Env              map[string]string
}

func Defaults() Config {
	return Config{
		Window: WindowConfig{
			Width: 1100, Height: 720, PaddingX: 6, PaddingY: 6, DynamicTitle: true,
			Opacity: 1.0, Blur: true,
		},
		Font: FontConfig{Family: "Go Mono", Size: 14, Ligatures: false},
		Colors: ColorsConfig{
			Foreground: "#E6E1D8", Background: "#080B12E6", Cursor: "#60E8F0", SelectionBackground: "#2A6377",
			ChromeBackground: "#10141CF0", ChromeMuted: "#A8B3C7FF", Accent: "#60E8F0FF",
			Split: "#4A5263FF", SearchMatch: "#7A5C12FF", Error: "#D87272FF",
			ANSI: [16]string{
				"#1B2232", "#FF5C8A", "#8BF59A", "#F8D866", "#7AA2FF", "#D88CFF", "#60E8F0", "#D8DEEA",
				"#57627A", "#FF7AA8", "#A6FFB5", "#FFE68A", "#9BB8FF", "#E5A7FF", "#90F4FF", "#FFFFFF",
			},
		},
		Scrolling: ScrollingConfig{History: 2000, WheelMultiplier: 3, HideCursorWhenScrolled: true},
		Scrollbar: ScrollbarConfig{
			Enabled: true, ReservedWidthPX: 12, WidthPX: 8, MarginPX: 2, RadiusPX: 4, MinThumbPX: 24,
			TrackColor: "#10172266", ThumbColor: "#60E8F0CC", ThumbHoverColor: "#7CF4F9E6", ThumbPressColor: "#B6FAFFFF",
			AutoHideDelayMS: 1000, FadeMS: 150, PageStep: 0.9, TrackClick: "page",
		},
		Cursor:    CursorConfig{Shape: "underline", Blink: true, BlinkIntervalMS: 1000, Thickness: 0.15},
		Clipboard: ClipboardConfig{OSC52: "off"},
		Render:    RenderConfig{Bidi: false, TextGamma: 1.15, TextDarken: 0.0, TextRaster: "go", StatsHotkey: "ctrl+shift+i", ZoomInHotkey: "ctrl+equal", ZoomOutHotkey: "ctrl+minus", ZoomResetHotkey: "ctrl+0", VSync: true, Redraw: "on_demand", Damage: "rows"},
		Shell:     ShellConfig{Args: []string{}, Env: map[string]string{}},
	}
}

// Clone returns a detached configuration copy, including mutable shell values.
func (c Config) Clone() Config {
	c.Shell.Args = append([]string(nil), c.Shell.Args...)
	if c.Shell.Env != nil {
		environment := make(map[string]string, len(c.Shell.Env))
		for key, value := range c.Shell.Env {
			environment[key] = value
		}
		c.Shell.Env = environment
	}
	return c
}

func (c Config) Validate() error {
	var errs []error
	if c.Window.Width < 100 || c.Window.Height < 100 {
		errs = append(errs, errors.New("window width and height must be >= 100"))
	}
	if c.Window.PaddingX < 0 || c.Window.PaddingY < 0 {
		errs = append(errs, errors.New("window padding must be >= 0"))
	}
	if math.IsNaN(c.Window.Opacity) || math.IsInf(c.Window.Opacity, 0) || c.Window.Opacity < 0 || c.Window.Opacity > 1 {
		errs = append(errs, errors.New("window.opacity must be a finite number between 0.0 and 1.0"))
	}
	if c.Font.Size <= 0 {
		errs = append(errs, errors.New("font size must be > 0"))
	}
	if c.Scrolling.History < 0 || c.Scrolling.History > MaxScrollbackHistory || c.Scrolling.WheelMultiplier <= 0 {
		errs = append(errs, fmt.Errorf("scrolling history must be between 0 and %d and wheel_multiplier > 0", MaxScrollbackHistory))
	}
	if c.Scrollbar.ReservedWidthPX < 0 || c.Scrollbar.WidthPX < 0 || c.Scrollbar.MarginPX < 0 || c.Scrollbar.RadiusPX < 0 {
		errs = append(errs, errors.New("scrollbar dimensions must be >= 0"))
	}
	if c.Scrollbar.Enabled && (c.Scrollbar.WidthPX <= 0 || c.Scrollbar.ReservedWidthPX <= 0) {
		errs = append(errs, errors.New("enabled scrollbar width_px and reserved_width_px must be > 0"))
	}
	if c.Scrollbar.WidthPX+2*c.Scrollbar.MarginPX > c.Scrollbar.ReservedWidthPX {
		errs = append(errs, errors.New("scrollbar width_px plus margins must fit reserved_width_px"))
	}
	if c.Scrollbar.RadiusPX*2 > c.Scrollbar.WidthPX {
		errs = append(errs, errors.New("scrollbar radius_px must not exceed half width_px"))
	}
	if c.Scrollbar.MinThumbPX <= 0 || math.IsNaN(c.Scrollbar.PageStep) || math.IsInf(c.Scrollbar.PageStep, 0) || c.Scrollbar.PageStep <= 0 {
		errs = append(errs, errors.New("scrollbar min_thumb_px and page_step must be > 0"))
	}
	if c.Scrollbar.AutoHideDelayMS < 0 || c.Scrollbar.FadeMS < 0 {
		errs = append(errs, errors.New("scrollbar auto_hide_delay_ms and fade_ms must be >= 0"))
	}
	if c.Scrollbar.TrackClick != "page" && c.Scrollbar.TrackClick != "jump" {
		errs = append(errs, fmt.Errorf("scrollbar.track_click %q must be page or jump", c.Scrollbar.TrackClick))
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
	if c.Render.TextRaster != "auto" && c.Render.TextRaster != "go" && c.Render.TextRaster != "subpixel" {
		errs = append(errs, fmt.Errorf("render.text_raster %q must be auto, go, or subpixel", c.Render.TextRaster))
	}
	if c.Render.Redraw != "on_demand" && c.Render.Redraw != "continuous" {
		errs = append(errs, fmt.Errorf("render.redraw %q must be on_demand or continuous", c.Render.Redraw))
	}
	if c.Render.Damage != "rows" && c.Render.Damage != "frame" {
		errs = append(errs, fmt.Errorf("render.damage %q must be rows or frame", c.Render.Damage))
	}
	for name, value := range map[string]string{
		"foreground": c.Colors.Foreground, "background": c.Colors.Background, "cursor": c.Colors.Cursor,
		"selection_background": c.Colors.SelectionBackground, "chrome_background": c.Colors.ChromeBackground,
		"chrome_muted": c.Colors.ChromeMuted, "accent": c.Colors.Accent, "split": c.Colors.Split,
		"search_match": c.Colors.SearchMatch, "error": c.Colors.Error,
	} {
		if !isHexColor(value) {
			errs = append(errs, fmt.Errorf("colors.%s must be #RRGGBB or #RRGGBBAA", name))
		}
	}
	for index, value := range c.Colors.ANSI {
		if !isHexRGBColor(value) {
			errs = append(errs, fmt.Errorf("colors.ansi[%d] must be #RRGGBB", index+1))
		}
	}
	for slot, value := range c.Colors.IndexedColors {
		if value != "" && !isHexRGBColor(value) {
			errs = append(errs, fmt.Errorf("colors.indexed_colors[%d] must be #RRGGBB", slot+firstIndexedColor))
		}
	}
	for name, value := range map[string]string{
		"track_color": c.Scrollbar.TrackColor, "thumb_color": c.Scrollbar.ThumbColor,
		"thumb_hover_color": c.Scrollbar.ThumbHoverColor, "thumb_press_color": c.Scrollbar.ThumbPressColor,
	} {
		if !isHexColor(value) {
			errs = append(errs, fmt.Errorf("scrollbar.%s must be #RRGGBB or #RRGGBBAA", name))
		}
	}
	if c.BackgroundAlpha() < 0xff && c.Window.Opacity < 1 {
		errs = append(errs, errors.New("transparent colors.background and window.opacity < 1 cannot be enabled together"))
	}
	return errors.Join(errs...)
}

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)
var hexRGBColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func isHexColor(value string) bool {
	return hexColorPattern.MatchString(value)
}

func isHexRGBColor(value string) bool {
	return hexRGBColorPattern.MatchString(value)
}

// BackgroundAlpha returns the configured background alpha. Invalid colors are
// treated as opaque; Validate reports their syntax separately.
func (c Config) BackgroundAlpha() uint8 {
	if len(c.Colors.Background) != 9 {
		return 0xff
	}
	n, err := strconv.ParseUint(c.Colors.Background[7:9], 16, 8)
	if err != nil {
		return 0xff
	}
	return uint8(n)
}
