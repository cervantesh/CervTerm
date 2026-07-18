package config

import (
	"cervterm/internal/fontdesc"
	"cervterm/internal/quickselect"
	"maps"
	"reflect"
	"slices"
)

// ConfigChange describes a changed public Config leaf without retaining values.
type ConfigChange struct {
	Path  string
	Scope ApplyScope
}

func fontRulesEqual(left, right []fontdesc.Rule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftID, leftErr := left[index].ID()
		rightID, rightErr := right[index].ID()
		if leftErr != nil || rightErr != nil || leftID != rightID {
			return false
		}
	}
	return true
}

func appendChange(changes []ConfigChange, changed bool, path string, scope ApplyScope) []ConfigChange {
	if changed {
		return append(changes, ConfigChange{Path: path, Scope: scope})
	}
	return changes
}

// DiffConfig compares every public Config leaf in deterministic schema order.
// Values are deliberately omitted so sensitive shell.env data cannot leak.
func DiffConfig(desired, effective Config) []ConfigChange {
	changes := make([]ConfigChange, 0)
	changes = appendChange(changes, desired.Window.Width != effective.Window.Width, "window.width", ApplyNewWindow)
	changes = appendChange(changes, desired.Window.Height != effective.Window.Height, "window.height", ApplyNewWindow)
	changes = appendChange(changes, desired.Window.InitialRows != effective.Window.InitialRows, "window.initial_rows", ApplyNewWindow)
	changes = appendChange(changes, desired.Window.InitialCols != effective.Window.InitialCols, "window.initial_cols", ApplyNewWindow)
	changes = appendChange(changes, desired.Window.Decorations != effective.Window.Decorations, "window.decorations", ApplyWindowRecreate)
	changes = appendChange(changes, desired.Window.Titlebar != effective.Window.Titlebar, "window.titlebar", ApplyWindowRecreate)
	changes = appendChange(changes, desired.Window.PaddingX != effective.Window.PaddingX, "window.padding_x", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingY != effective.Window.PaddingY, "window.padding_y", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingLeft != effective.Window.PaddingLeft, "window.padding_left", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingRight != effective.Window.PaddingRight, "window.padding_right", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingTop != effective.Window.PaddingTop, "window.padding_top", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingBottom != effective.Window.PaddingBottom, "window.padding_bottom", ApplyRestart)
	changes = appendChange(changes, desired.Window.DynamicTitle != effective.Window.DynamicTitle, "window.dynamic_title", ApplyRestart)
	changes = appendChange(changes, desired.Window.Opacity != effective.Window.Opacity, "window.opacity", ApplyLive)
	changes = appendChange(changes, desired.Window.TextOpacity != effective.Window.TextOpacity, "window.text_opacity", ApplyLive)
	changes = appendChange(changes, desired.Window.BackgroundOpacity != effective.Window.BackgroundOpacity, "window.background_opacity", ApplyLive)
	changes = appendChange(changes, desired.Window.Blur != effective.Window.Blur, "window.blur", ApplyLive)

	changes = appendChange(changes, desired.Font.Family != effective.Font.Family, "font.family", ApplyRestart)
	changes = appendChange(changes, !slices.Equal(desired.Font.Descriptors, effective.Font.Descriptors), "font.descriptors", ApplyRestart)
	changes = appendChange(changes, !slices.Equal(desired.Font.Fallback, effective.Font.Fallback), "font.fallback", ApplyRestart)
	changes = appendChange(changes, !fontRulesEqual(desired.Font.Rules, effective.Font.Rules), "font.rules", ApplyRestart)
	changes = appendChange(changes, desired.Font.Size != effective.Font.Size, "font.size", ApplyRestart)
	changes = appendChange(changes, desired.Font.Ligatures != effective.Font.Ligatures, "font.ligatures", ApplyRestart)
	changes = appendChange(changes, !maps.Equal(desired.Font.Features, effective.Font.Features), "font.features", ApplyRestart)
	changes = appendChange(changes, desired.Font.LineHeight != effective.Font.LineHeight, "font.line_height", ApplyRestart)
	changes = appendChange(changes, desired.Font.CellWidth != effective.Font.CellWidth, "font.cell_width", ApplyRestart)
	changes = appendChange(changes, desired.Font.BaselineOffset != effective.Font.BaselineOffset, "font.baseline_offset", ApplyRestart)
	changes = appendChange(changes, desired.Font.GlyphOffsetX != effective.Font.GlyphOffsetX, "font.glyph_offset_x", ApplyRestart)
	changes = appendChange(changes, desired.Font.GlyphOffsetY != effective.Font.GlyphOffsetY, "font.glyph_offset_y", ApplyRestart)

	changes = appendChange(changes, desired.ColorScheme != effective.ColorScheme, "color_scheme", ApplyLive)

	changes = appendChange(changes, desired.Colors.Foreground != effective.Colors.Foreground, "colors.foreground", ApplyLive)
	changes = appendChange(changes, desired.Colors.Background != effective.Colors.Background, "colors.background", ApplyLive)
	changes = appendChange(changes, desired.Colors.Cursor != effective.Colors.Cursor, "colors.cursor", ApplyLive)
	changes = appendChange(changes, desired.Colors.SelectionBackground != effective.Colors.SelectionBackground, "colors.selection_background", ApplyLive)
	changes = appendChange(changes, desired.Colors.ChromeBackground != effective.Colors.ChromeBackground, "colors.chrome_background", ApplyLive)
	changes = appendChange(changes, desired.Colors.ChromeMuted != effective.Colors.ChromeMuted, "colors.chrome_muted", ApplyLive)
	changes = appendChange(changes, desired.Colors.Accent != effective.Colors.Accent, "colors.accent", ApplyLive)
	changes = appendChange(changes, desired.Colors.Split != effective.Colors.Split, "colors.split", ApplyLive)
	changes = appendChange(changes, desired.Colors.SearchMatch != effective.Colors.SearchMatch, "colors.search_match", ApplyLive)
	changes = appendChange(changes, desired.Colors.Error != effective.Colors.Error, "colors.error", ApplyLive)
	changes = appendChange(changes, desired.Colors.ANSI != effective.Colors.ANSI, "colors.ansi", ApplyLive)
	changes = appendChange(changes, desired.Colors.IndexedColors != effective.Colors.IndexedColors, "colors.indexed_colors", ApplyLive)
	changes = appendChange(changes, !reflect.DeepEqual(desired.Background.Layers, effective.Background.Layers), "background.layers", ApplyLive)

	changes = appendChange(changes, desired.Scrolling.History != effective.Scrolling.History, "scrolling.history", ApplyLive)
	changes = appendChange(changes, desired.Scrolling.WheelMultiplier != effective.Scrolling.WheelMultiplier, "scrolling.wheel_multiplier", ApplyLive)
	changes = appendChange(changes, desired.Scrolling.HideCursorWhenScrolled != effective.Scrolling.HideCursorWhenScrolled, "scrolling.hide_cursor_when_scrolled", ApplyLive)

	changes = appendChange(changes, desired.Scrollbar.Enabled != effective.Scrollbar.Enabled, "scrollbar.enabled", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.Mode != effective.Scrollbar.Mode, "scrollbar.mode", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.StableGutter != effective.Scrollbar.StableGutter, "scrollbar.stable_gutter", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.AnimationFPS != effective.Scrollbar.AnimationFPS, "scrollbar.animation_fps", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.ReservedWidthPX != effective.Scrollbar.ReservedWidthPX, "scrollbar.reserved_width_px", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.WidthPX != effective.Scrollbar.WidthPX, "scrollbar.width_px", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.MarginPX != effective.Scrollbar.MarginPX, "scrollbar.margin_px", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.RadiusPX != effective.Scrollbar.RadiusPX, "scrollbar.radius_px", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.MinThumbPX != effective.Scrollbar.MinThumbPX, "scrollbar.min_thumb_px", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.TrackColor != effective.Scrollbar.TrackColor, "scrollbar.track_color", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.ThumbColor != effective.Scrollbar.ThumbColor, "scrollbar.thumb_color", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.ThumbHoverColor != effective.Scrollbar.ThumbHoverColor, "scrollbar.thumb_hover_color", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.ThumbPressColor != effective.Scrollbar.ThumbPressColor, "scrollbar.thumb_press_color", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.AutoHideDelayMS != effective.Scrollbar.AutoHideDelayMS, "scrollbar.auto_hide_delay_ms", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.FadeMS != effective.Scrollbar.FadeMS, "scrollbar.fade_ms", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.PageStep != effective.Scrollbar.PageStep, "scrollbar.page_step", ApplyLive)
	changes = appendChange(changes, desired.Scrollbar.TrackClick != effective.Scrollbar.TrackClick, "scrollbar.track_click", ApplyLive)
	changes = appendChange(changes, desired.TabBar.Mode != effective.TabBar.Mode, "tab_bar.mode", ApplyLive)
	changes = appendChange(changes, desired.TabBar.Position != effective.TabBar.Position, "tab_bar.position", ApplyLive)
	changes = appendChange(changes, desired.TabBar.HeightPX != effective.TabBar.HeightPX, "tab_bar.height_px", ApplyLive)
	changes = appendChange(changes, desired.TabBar.MinWidthPX != effective.TabBar.MinWidthPX, "tab_bar.min_width_px", ApplyLive)
	changes = appendChange(changes, desired.TabBar.MaxWidthPX != effective.TabBar.MaxWidthPX, "tab_bar.max_width_px", ApplyLive)
	changes = appendChange(changes, desired.TabBar.PaddingX != effective.TabBar.PaddingX, "tab_bar.padding_x", ApplyLive)
	changes = appendChange(changes, desired.TabBar.ShowNewButton != effective.TabBar.ShowNewButton, "tab_bar.show_new_button", ApplyLive)
	changes = appendChange(changes, desired.TabBar.ShowCloseButton != effective.TabBar.ShowCloseButton, "tab_bar.show_close_button", ApplyLive)

	changes = appendChange(changes, desired.Cursor.Shape != effective.Cursor.Shape, "cursor.shape", ApplyLive)
	changes = appendChange(changes, desired.Cursor.Blink != effective.Cursor.Blink, "cursor.blink", ApplyLive)
	changes = appendChange(changes, desired.Cursor.BlinkIntervalMS != effective.Cursor.BlinkIntervalMS, "cursor.blink_interval_ms", ApplyLive)
	changes = appendChange(changes, desired.Cursor.Thickness != effective.Cursor.Thickness, "cursor.thickness", ApplyLive)

	changes = appendChange(changes, desired.Clipboard.OSC52 != effective.Clipboard.OSC52, "clipboard.osc52", ApplyRestart)

	changes = appendChange(changes, desired.Render.Bidi != effective.Render.Bidi, "render.bidi", ApplyRestart)
	changes = appendChange(changes, desired.Render.TextGamma != effective.Render.TextGamma, "render.text_gamma", ApplyRestart)
	changes = appendChange(changes, desired.Render.TextDarken != effective.Render.TextDarken, "render.text_darken", ApplyRestart)
	changes = appendChange(changes, desired.Render.TextRaster != effective.Render.TextRaster, "render.text_raster", ApplyRestart)
	changes = appendChange(changes, desired.Render.StatsHotkey != effective.Render.StatsHotkey, "render.stats_hotkey", ApplyRestart)
	changes = appendChange(changes, desired.Render.ZoomInHotkey != effective.Render.ZoomInHotkey, "render.zoom_in_hotkey", ApplyRestart)
	changes = appendChange(changes, desired.Render.ZoomOutHotkey != effective.Render.ZoomOutHotkey, "render.zoom_out_hotkey", ApplyRestart)
	changes = appendChange(changes, desired.Render.ZoomResetHotkey != effective.Render.ZoomResetHotkey, "render.zoom_reset_hotkey", ApplyRestart)
	changes = appendChange(changes, desired.Render.VSync != effective.Render.VSync, "render.vsync", ApplyRestart)
	changes = appendChange(changes, desired.Render.MaxFPS != effective.Render.MaxFPS, "render.max_fps", ApplyLive)
	changes = appendChange(changes, desired.Render.Redraw != effective.Render.Redraw, "render.redraw", ApplyRestart)
	changes = appendChange(changes, desired.Render.Damage != effective.Render.Damage, "render.damage", ApplyRestart)

	changes = appendChange(changes, desired.Shell.Program != effective.Shell.Program, "shell.program", ApplyNewPane)
	changes = appendChange(changes, !slices.Equal(desired.Shell.Args, effective.Shell.Args), "shell.args", ApplyNewPane)
	changes = appendChange(changes, desired.Shell.WorkingDirectory != effective.Shell.WorkingDirectory, "shell.working_directory", ApplyNewPane)
	changes = appendChange(changes, !maps.Equal(desired.Shell.Env, effective.Shell.Env), "shell.env", ApplyNewPane)
	changes = appendChange(changes, !reflect.DeepEqual(desired.LaunchMenu, effective.LaunchMenu), "launch_menu", ApplyLive)
	changes = appendChange(changes, !slices.Equal(desired.QuickSelect.Rules, effective.QuickSelect.Rules), "quick_select.rules", ApplyLive)
	return changes
}

// PendingConfigChanges excludes changes already applied to the active window.
func PendingConfigChanges(desired, effective Config) []ConfigChange {
	changes := DiffConfig(desired, effective)
	pending := make([]ConfigChange, 0, len(changes))
	for _, change := range changes {
		if change.Scope != ApplyLive {
			pending = append(pending, change)
		}
	}
	return pending
}

// MergeLiveConfig copies the fields classified live from source into base.
func MergeLiveConfig(base, source Config) Config {
	base = base.Clone()
	base.Window.Opacity = source.Window.Opacity
	base.Window.TextOpacity = source.Window.TextOpacity
	base.Window.BackgroundOpacity = source.Window.BackgroundOpacity
	base.Window.Blur = source.Window.Blur
	base.ColorScheme = source.ColorScheme
	base.Colors = source.Colors
	base.Background = source.Background
	base.Background.Layers = cloneBackgroundLayers(source.Background.Layers)
	base.Scrolling = source.Scrolling
	base.Scrollbar = source.Scrollbar
	base.TabBar = source.TabBar
	base.Cursor = source.Cursor
	base.Render.MaxFPS = source.Render.MaxFPS
	base.LaunchMenu = cloneLaunchTargets(source.LaunchMenu)
	base.QuickSelect = source.QuickSelect
	base.QuickSelect.Rules = append([]QuickSelectRule(nil), source.QuickSelect.Rules...)
	base.QuickSelect.Compiled = append([]quickselect.PreparedRule(nil), source.QuickSelect.Compiled...)
	return base
}
