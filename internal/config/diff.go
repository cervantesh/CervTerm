package config

import (
	"maps"
	"slices"
)

// ConfigChange describes a changed public Config leaf without retaining values.
type ConfigChange struct {
	Path  string
	Scope ApplyScope
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
	changes = appendChange(changes, desired.Window.PaddingX != effective.Window.PaddingX, "window.padding_x", ApplyRestart)
	changes = appendChange(changes, desired.Window.PaddingY != effective.Window.PaddingY, "window.padding_y", ApplyRestart)
	changes = appendChange(changes, desired.Window.DynamicTitle != effective.Window.DynamicTitle, "window.dynamic_title", ApplyRestart)
	changes = appendChange(changes, desired.Window.Opacity != effective.Window.Opacity, "window.opacity", ApplyLive)
	changes = appendChange(changes, desired.Window.Blur != effective.Window.Blur, "window.blur", ApplyLive)

	changes = appendChange(changes, desired.Font.Family != effective.Font.Family, "font.family", ApplyRestart)
	changes = appendChange(changes, desired.Font.Size != effective.Font.Size, "font.size", ApplyRestart)
	changes = appendChange(changes, desired.Font.Ligatures != effective.Font.Ligatures, "font.ligatures", ApplyRestart)

	changes = appendChange(changes, desired.Colors.Foreground != effective.Colors.Foreground, "colors.foreground", ApplyLive)
	changes = appendChange(changes, desired.Colors.Background != effective.Colors.Background, "colors.background", ApplyLive)
	changes = appendChange(changes, desired.Colors.Cursor != effective.Colors.Cursor, "colors.cursor", ApplyLive)
	changes = appendChange(changes, desired.Colors.SelectionBackground != effective.Colors.SelectionBackground, "colors.selection_background", ApplyLive)
	changes = appendChange(changes, desired.Colors.ANSI != effective.Colors.ANSI, "colors.ansi", ApplyLive)

	changes = appendChange(changes, desired.Scrolling.History != effective.Scrolling.History, "scrolling.history", ApplyLive)
	changes = appendChange(changes, desired.Scrolling.WheelMultiplier != effective.Scrolling.WheelMultiplier, "scrolling.wheel_multiplier", ApplyLive)
	changes = appendChange(changes, desired.Scrolling.HideCursorWhenScrolled != effective.Scrolling.HideCursorWhenScrolled, "scrolling.hide_cursor_when_scrolled", ApplyLive)

	changes = appendChange(changes, desired.Scrollbar.Enabled != effective.Scrollbar.Enabled, "scrollbar.enabled", ApplyLive)
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
	changes = appendChange(changes, desired.Render.Redraw != effective.Render.Redraw, "render.redraw", ApplyRestart)
	changes = appendChange(changes, desired.Render.Damage != effective.Render.Damage, "render.damage", ApplyRestart)

	changes = appendChange(changes, desired.Shell.Program != effective.Shell.Program, "shell.program", ApplyNewPane)
	changes = appendChange(changes, !slices.Equal(desired.Shell.Args, effective.Shell.Args), "shell.args", ApplyNewPane)
	changes = appendChange(changes, desired.Shell.WorkingDirectory != effective.Shell.WorkingDirectory, "shell.working_directory", ApplyNewPane)
	changes = appendChange(changes, !maps.Equal(desired.Shell.Env, effective.Shell.Env), "shell.env", ApplyNewPane)
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
	base.Window.Blur = source.Window.Blur
	base.Colors = source.Colors
	base.Scrolling = source.Scrolling
	base.Scrollbar = source.Scrollbar
	base.Cursor = source.Cursor
	return base
}
