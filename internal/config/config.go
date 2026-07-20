package config

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"cervterm/internal/fontdesc"
	"cervterm/internal/quickselect"
)

const (
	MaxScrollbackHistory = 10_000
	MaxWindowPadding     = 256
)

type Config struct {
	Window            WindowConfig
	LayoutPersistence LayoutPersistenceConfig
	Font              FontConfig
	ColorScheme       string `json:",omitempty"`
	Colors            ColorsConfig
	Background        BackgroundConfig
	Scrolling         ScrollingConfig
	Scrollbar         ScrollbarConfig
	Cursor            CursorConfig
	Clipboard         ClipboardConfig
	IME               IMEConfig
	Bell              BellConfig
	Notification      NotificationConfig
	Render            RenderConfig
	Shell             ShellConfig
	QuickSelect       QuickSelectConfig
	TabBar            TabBarConfig
	LaunchMenu        []LaunchTarget
}

type WindowConfig struct {
	Width             int
	Height            int
	InitialRows       int
	InitialCols       int
	Decorations       string
	Titlebar          string
	PaddingX          int
	PaddingY          int
	PaddingLeft       int
	PaddingRight      int
	PaddingTop        int
	PaddingBottom     int
	DynamicTitle      bool
	Opacity           float64
	TextOpacity       float64
	BackgroundOpacity float64
	Blur              bool
}

type FontConfig struct {
	Family         string
	Descriptors    []fontdesc.Descriptor `json:"Descriptors,omitempty"`
	Fallback       []fontdesc.Descriptor `json:"Fallback,omitempty"`
	Rules          []fontdesc.Rule       `json:"Rules,omitempty"`
	Size           float64
	Ligatures      bool
	Features       map[string]int `json:"Features,omitempty"`
	LineHeight     float64
	CellWidth      float64
	BaselineOffset float64
	GlyphOffsetX   float64
	GlyphOffsetY   float64
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
	// Enabled is the legacy v1 switch. Mode is authoritative for v2 documents.
	Enabled         bool
	Mode            string
	StableGutter    bool
	AnimationFPS    int
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
	MaxFPS          int
	Redraw          string
	Damage          string
}

type ShellConfig struct {
	Program          string
	Args             []string
	WorkingDirectory string
	Env              map[string]string
}

type QuickSelectRule struct {
	ID       string
	Pattern  string
	Action   quickselect.Action
	Priority int
}

type QuickSelectConfig struct {
	Rules    []QuickSelectRule
	Compiled []quickselect.PreparedRule `json:"-"`
}

func Defaults() Config {
	return Config{
		Window: WindowConfig{
			Width: 1100, Height: 720, InitialRows: 0, InitialCols: 0, Decorations: "system", Titlebar: "dark",
			PaddingX: 6, PaddingY: 6, PaddingLeft: 6, PaddingRight: 6, PaddingTop: 6, PaddingBottom: 6,
			DynamicTitle: true, Opacity: 1.0, TextOpacity: 1.0, BackgroundOpacity: 1.0, Blur: true,
		},
		Font: FontConfig{Family: "Go Mono", Size: 14, Ligatures: false, Features: map[string]int{}, LineHeight: 1, CellWidth: 1},
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
			Enabled: true, Mode: "scrolling", StableGutter: true, AnimationFPS: 60,
			ReservedWidthPX: 12, WidthPX: 8, MarginPX: 2, RadiusPX: 4, MinThumbPX: 24,
			TrackColor: "#10172266", ThumbColor: "#60E8F0CC", ThumbHoverColor: "#7CF4F9E6", ThumbPressColor: "#B6FAFFFF",
			AutoHideDelayMS: 1000, FadeMS: 150, PageStep: 0.9, TrackClick: "page",
		},
		TabBar: TabBarConfig{
			Mode: "multiple", Position: "top", HeightPX: 28, MinWidthPX: 96, MaxWidthPX: 220, PaddingX: 8,
			ShowNewButton: true, ShowCloseButton: true,
		},
		Cursor:       CursorConfig{Shape: "underline", Blink: true, BlinkIntervalMS: 1000, Thickness: 0.15},
		Clipboard:    ClipboardConfig{OSC52: "off"},
		IME:          IMEConfig{Enabled: false},
		Bell:         BellConfig{Mode: "disabled", Focus: "unfocused", ThrottleMS: 250, VisualDurationMS: 120},
		Notification: NotificationConfig{Enabled: false, Focus: "unfocused", RateLimitMS: 5000},
		Render:       RenderConfig{Bidi: false, TextGamma: 1.15, TextDarken: 0.0, TextRaster: "go", StatsHotkey: "ctrl+shift+i", ZoomInHotkey: "ctrl+equal", ZoomOutHotkey: "ctrl+minus", ZoomResetHotkey: "ctrl+0", VSync: true, MaxFPS: 0, Redraw: "on_demand", Damage: "rows"},
		Shell:        ShellConfig{Args: []string{}, Env: map[string]string{}},
	}
}

// Clone returns a detached configuration copy, including mutable shell values.
func (c Config) Clone() Config {
	c.Font.Descriptors = append([]fontdesc.Descriptor(nil), c.Font.Descriptors...)
	c.Font.Fallback = append([]fontdesc.Descriptor(nil), c.Font.Fallback...)
	c.Font.Rules = cloneFontRules(c.Font.Rules)
	if c.Font.Features != nil {
		features := make(map[string]int, len(c.Font.Features))
		for tag, value := range c.Font.Features {
			features[tag] = value
		}
		c.Font.Features = features
	}
	c.Background.Layers = cloneBackgroundLayers(c.Background.Layers)
	c.Shell.Args = append([]string(nil), c.Shell.Args...)
	if c.Shell.Env != nil {
		environment := make(map[string]string, len(c.Shell.Env))
		for key, value := range c.Shell.Env {
			environment[key] = value
		}
		c.Shell.Env = environment
	}
	c.QuickSelect.Rules = append([]QuickSelectRule(nil), c.QuickSelect.Rules...)
	c.QuickSelect.Compiled = append([]quickselect.PreparedRule(nil), c.QuickSelect.Compiled...)
	c.LaunchMenu = cloneLaunchTargets(c.LaunchMenu)
	return c
}

func cloneFontRules(rules []fontdesc.Rule) []fontdesc.Rule {
	if rules == nil {
		return nil
	}
	cloned := make([]fontdesc.Rule, len(rules))
	for index, rule := range rules {
		cloned[index] = rule
		cloned[index].Match.Styles = append([]fontdesc.Style(nil), rule.Match.Styles...)
		cloned[index].Match.Ranges = append([]fontdesc.RuneRange(nil), rule.Match.Ranges...)
	}
	return cloned
}

func normalizeDescriptorConfigList(path string, descriptors []fontdesc.Descriptor, limit int) ([]fontdesc.Descriptor, []error) {
	if len(descriptors) > limit {
		return nil, []error{fmt.Errorf("%s must contain at most %d entries", path, limit)}
	}
	normalized := make([]fontdesc.Descriptor, len(descriptors))
	var errs []error
	for index, descriptor := range descriptors {
		value, err := descriptor.Normalize()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s[%d]: %w", path, index+1, err))
			continue
		}
		normalized[index] = value
	}
	return normalized, errs
}

func normalizeRuleConfigList(rules []fontdesc.Rule) ([]fontdesc.Rule, []error) {
	if len(rules) > fontdesc.MaxRules {
		return nil, []error{fmt.Errorf("font.rules must contain at most %d entries", fontdesc.MaxRules)}
	}
	normalized := make([]fontdesc.Rule, len(rules))
	totalRanges := 0
	var errs []error
	for index, rule := range rules {
		value, err := rule.Normalize()
		if err != nil {
			errs = append(errs, fmt.Errorf("font.rules[%d]: %w", index+1, err))
			continue
		}
		totalRanges += len(value.Match.Ranges)
		normalized[index] = value
	}
	if totalRanges > fontdesc.MaxTotalRanges {
		errs = append(errs, fmt.Errorf("font.rules contain %d normalized ranges, maximum is %d", totalRanges, fontdesc.MaxTotalRanges))
	}
	return normalized, errs
}

func PrepareQuickSelect(rules []QuickSelectRule) ([]quickselect.PreparedRule, error) {
	if len(rules) > quickselect.MaxRules {
		return nil, fmt.Errorf("quick_select.rules must contain at most %d entries", quickselect.MaxRules)
	}
	seen := make(map[string]struct{}, len(rules))
	prepared := make([]quickselect.PreparedRule, len(rules))
	for i, rule := range rules {
		if _, ok := seen[rule.ID]; ok {
			return nil, fmt.Errorf("quick_select.rules[%d].id %q is duplicated", i+1, rule.ID)
		}
		seen[rule.ID] = struct{}{}
		compiled, err := quickselect.PrepareRuleWithAction(rule.ID, rule.Pattern, rule.Action, rule.Priority)
		if err != nil {
			return nil, fmt.Errorf("quick_select.rules[%d]: %w", i+1, err)
		}
		prepared[i] = compiled
	}
	return prepared, nil
}

func (c Config) Validate() error {
	var errs []error
	preparedQuickSelect, quickSelectErr := PrepareQuickSelect(c.QuickSelect.Rules)
	_ = preparedQuickSelect
	if quickSelectErr != nil {
		errs = append(errs, quickSelectErr)
	}
	if err := validateLaunchMenu(c.LaunchMenu); err != nil {
		errs = append(errs, err)
	}
	if err := validateLayoutPersistencePath(c.LayoutPersistence.Path); err != nil {
		errs = append(errs, err)
	}
	if c.Window.Width < 100 || c.Window.Height < 100 {
		errs = append(errs, errors.New("window width and height must be >= 100"))
	}
	if (c.Window.InitialRows == 0) != (c.Window.InitialCols == 0) {
		errs = append(errs, errors.New("window.initial_rows and window.initial_cols must both be zero or both be set"))
	} else if c.Window.InitialRows != 0 && (c.Window.InitialRows < 10 || c.Window.InitialRows > 1000 || c.Window.InitialCols < 10 || c.Window.InitialCols > 1000) {
		errs = append(errs, errors.New("window.initial_rows and window.initial_cols must both be between 10 and 1000"))
	}
	if c.Window.Decorations != "system" && c.Window.Decorations != "none" {
		errs = append(errs, fmt.Errorf("window.decorations %q must be system or none", c.Window.Decorations))
	}
	if c.Window.Titlebar != "system" && c.Window.Titlebar != "dark" {
		errs = append(errs, fmt.Errorf("window.titlebar %q must be system or dark", c.Window.Titlebar))
	}
	if c.Window.PaddingX < 0 || c.Window.PaddingY < 0 {
		errs = append(errs, errors.New("window padding aliases must be >= 0"))
	}
	for _, padding := range []struct {
		path  string
		value int
	}{
		{"padding_left", c.Window.PaddingLeft},
		{"padding_right", c.Window.PaddingRight},
		{"padding_top", c.Window.PaddingTop},
		{"padding_bottom", c.Window.PaddingBottom},
	} {
		if padding.value < 0 || padding.value > MaxWindowPadding {
			errs = append(errs, fmt.Errorf("window.%s must be between 0 and %d", padding.path, MaxWindowPadding))
		}
	}
	if math.IsNaN(c.Window.Opacity) || math.IsInf(c.Window.Opacity, 0) || c.Window.Opacity < 0 || c.Window.Opacity > 1 {
		errs = append(errs, errors.New("window.opacity must be a finite number between 0.0 and 1.0"))
	}
	if math.IsNaN(c.Window.TextOpacity) || math.IsInf(c.Window.TextOpacity, 0) || c.Window.TextOpacity < 0 || c.Window.TextOpacity > 1 {
		errs = append(errs, errors.New("window.text_opacity must be a finite number between 0.0 and 1.0"))
	}
	if math.IsNaN(c.Window.BackgroundOpacity) || math.IsInf(c.Window.BackgroundOpacity, 0) || c.Window.BackgroundOpacity < 0 || c.Window.BackgroundOpacity > 1 {
		errs = append(errs, errors.New("window.background_opacity must be a finite number between 0.0 and 1.0"))
	}
	if err := validateBackgroundLayers(c.Background.Layers); err != nil {
		errs = append(errs, err)
	}
	if c.Font.Size <= 0 {
		errs = append(errs, errors.New("font size must be > 0"))
	}
	normalizedPrimary, primaryErrors := normalizeDescriptorConfigList("font.descriptors", c.Font.Descriptors, fontdesc.MaxPrimaryDescriptors)
	errs = append(errs, primaryErrors...)
	normalizedFallback, fallbackErrors := normalizeDescriptorConfigList("font.fallback", c.Font.Fallback, fontdesc.MaxFallbackDescriptors)
	errs = append(errs, fallbackErrors...)
	normalizedRules, ruleErrors := normalizeRuleConfigList(c.Font.Rules)
	errs = append(errs, ruleErrors...)
	features, featureErr := fontdesc.NewFeatureSet(c.Font.Ligatures, c.Font.Features)
	if featureErr != nil {
		errs = append(errs, fmt.Errorf("font.features: %w", featureErr))
	}
	metrics, metricErr := fontdesc.NewMetricProjection(c.Font.LineHeight, c.Font.CellWidth, c.Font.BaselineOffset, c.Font.GlyphOffsetX, c.Font.GlyphOffsetY)
	if metricErr != nil {
		errs = append(errs, fmt.Errorf("font metrics: %w", metricErr))
	}
	if len(primaryErrors) == 0 && len(fallbackErrors) == 0 && len(ruleErrors) == 0 && featureErr == nil && metricErr == nil {
		if _, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: normalizedPrimary, Fallback: normalizedFallback, Rules: normalizedRules, Features: features.CanonicalBytes(), Metrics: metrics.CanonicalBytes()}); err != nil {
			errs = append(errs, fmt.Errorf("font canonical payload: %w", err))
		}
	}
	if c.Scrolling.History < 0 || c.Scrolling.History > MaxScrollbackHistory || c.Scrolling.WheelMultiplier <= 0 {
		errs = append(errs, fmt.Errorf("scrolling history must be between 0 and %d and wheel_multiplier > 0", MaxScrollbackHistory))
	}
	if c.Scrollbar.ReservedWidthPX < 0 || c.Scrollbar.WidthPX < 0 || c.Scrollbar.MarginPX < 0 || c.Scrollbar.RadiusPX < 0 {
		errs = append(errs, errors.New("scrollbar dimensions must be >= 0"))
	}
	if c.Scrollbar.Mode != "always" && c.Scrollbar.Mode != "hover" && c.Scrollbar.Mode != "scrolling" && c.Scrollbar.Mode != "never" {
		errs = append(errs, fmt.Errorf("scrollbar.mode %q must be always, hover, scrolling, or never", c.Scrollbar.Mode))
	}
	if c.Scrollbar.AnimationFPS < 1 || c.Scrollbar.AnimationFPS > 240 {
		errs = append(errs, errors.New("scrollbar.animation_fps must be between 1 and 240"))
	}
	if c.Scrollbar.Mode != "never" && (c.Scrollbar.WidthPX <= 0 || c.Scrollbar.ReservedWidthPX <= 0) {
		errs = append(errs, errors.New("non-never scrollbar width_px and reserved_width_px must be > 0"))
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
	errs = append(errs, validateTabBar(c.TabBar)...)
	if c.Cursor.Shape != "block" && c.Cursor.Shape != "underline" && c.Cursor.Shape != "beam" {
		errs = append(errs, fmt.Errorf("cursor shape %q must be block, underline, or beam", c.Cursor.Shape))
	}
	if c.Cursor.BlinkIntervalMS <= 0 || c.Cursor.Thickness <= 0 {
		errs = append(errs, errors.New("cursor blink_interval_ms and thickness must be > 0"))
	}
	if c.Clipboard.OSC52 != "write" && c.Clipboard.OSC52 != "off" {
		errs = append(errs, fmt.Errorf("clipboard.osc52 %q must be write or off", c.Clipboard.OSC52))
	}
	errs = append(errs, validateBell(c.Bell)...)
	errs = append(errs, validateNotification(c.Notification)...)
	if c.Render.TextGamma < 0.5 || c.Render.TextGamma > 3.0 {
		errs = append(errs, errors.New("render.text_gamma must be between 0.5 and 3.0"))
	}
	if c.Render.TextDarken < 0.0 || c.Render.TextDarken > 0.5 {
		errs = append(errs, errors.New("render.text_darken must be between 0.0 and 0.5"))
	}
	if c.Render.TextRaster != "auto" && c.Render.TextRaster != "go" && c.Render.TextRaster != "subpixel" {
		errs = append(errs, fmt.Errorf("render.text_raster %q must be auto, go, or subpixel", c.Render.TextRaster))
	}
	if c.Render.MaxFPS < 0 || c.Render.MaxFPS > 1000 {
		errs = append(errs, errors.New("render.max_fps must be between 0 and 1000"))
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
	if c.EffectiveBackgroundAlpha() < 0xff && c.Window.Opacity < 1 {
		errs = append(errs, errors.New("translucent terminal background and window.opacity < 1 cannot be enabled together"))
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

// EffectiveBackgroundAlpha applies the terminal-background opacity multiplier
// once to the configured solid background alpha.
func (c Config) EffectiveBackgroundAlpha() uint8 {
	return uint8(math.Round(float64(c.BackgroundAlpha()) * c.Window.BackgroundOpacity))
}
