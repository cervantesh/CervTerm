package config

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func LoadLua(path string, base Config) (Config, error) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer state.Close()
	if err := state.DoFile(path); err != nil {
		return base, err
	}
	value := state.Get(-1)
	table, ok := value.(*lua.LTable)
	if !ok {
		return base, fmt.Errorf("config must return a table, got %s", value.Type().String())
	}
	return FromTable(base, table), nil
}

func FromTable(cfg Config, root *lua.LTable) Config {
	if tbl := tableField(root, "window"); tbl != nil {
		cfg.Window.Width = intField(tbl, "width", cfg.Window.Width)
		cfg.Window.Height = intField(tbl, "height", cfg.Window.Height)
		cfg.Window.PaddingX = intField(tbl, "padding_x", cfg.Window.PaddingX)
		cfg.Window.PaddingY = intField(tbl, "padding_y", cfg.Window.PaddingY)
		cfg.Window.DynamicTitle = boolField(tbl, "dynamic_title", cfg.Window.DynamicTitle)
	}
	if tbl := tableField(root, "font"); tbl != nil {
		cfg.Font.Family = stringField(tbl, "family", cfg.Font.Family)
		cfg.Font.Size = numberField(tbl, "size", cfg.Font.Size)
	}
	if tbl := tableField(root, "colors"); tbl != nil {
		cfg.Colors.Foreground = stringField(tbl, "foreground", cfg.Colors.Foreground)
		cfg.Colors.Background = stringField(tbl, "background", cfg.Colors.Background)
		cfg.Colors.Cursor = stringField(tbl, "cursor", cfg.Colors.Cursor)
		cfg.Colors.SelectionBackground = stringField(tbl, "selection_background", cfg.Colors.SelectionBackground)
	}
	if tbl := tableField(root, "scrolling"); tbl != nil {
		cfg.Scrolling.History = intField(tbl, "history", cfg.Scrolling.History)
		cfg.Scrolling.WheelMultiplier = intField(tbl, "wheel_multiplier", cfg.Scrolling.WheelMultiplier)
		cfg.Scrolling.HideCursorWhenScrolled = boolField(tbl, "hide_cursor_when_scrolled", cfg.Scrolling.HideCursorWhenScrolled)
	}
	if tbl := tableField(root, "cursor"); tbl != nil {
		cfg.Cursor.Shape = stringField(tbl, "shape", cfg.Cursor.Shape)
		cfg.Cursor.Blink = boolField(tbl, "blink", cfg.Cursor.Blink)
		cfg.Cursor.BlinkIntervalMS = intField(tbl, "blink_interval_ms", cfg.Cursor.BlinkIntervalMS)
		cfg.Cursor.Thickness = numberField(tbl, "thickness", cfg.Cursor.Thickness)
	}
	if tbl := tableField(root, "clipboard"); tbl != nil {
		cfg.Clipboard.OSC52 = stringField(tbl, "osc52", cfg.Clipboard.OSC52)
	}
	if tbl := tableField(root, "render"); tbl != nil {
		cfg.Render.Bidi = boolField(tbl, "bidi", cfg.Render.Bidi)
		cfg.Render.TextGamma = numberField(tbl, "text_gamma", cfg.Render.TextGamma)
		cfg.Render.TextDarken = numberField(tbl, "text_darken", cfg.Render.TextDarken)
		cfg.Render.TextRaster = stringField(tbl, "text_raster", cfg.Render.TextRaster)
		cfg.Render.StatsHotkey = stringField(tbl, "stats_hotkey", cfg.Render.StatsHotkey)
		cfg.Render.VSync = boolField(tbl, "vsync", cfg.Render.VSync)
	}
	if tbl := tableField(root, "shell"); tbl != nil {
		cfg.Shell.Program = stringField(tbl, "program", cfg.Shell.Program)
		cfg.Shell.WorkingDirectory = stringField(tbl, "working_directory", cfg.Shell.WorkingDirectory)
		cfg.Shell.Args = stringListField(tbl, "args", cfg.Shell.Args)
		cfg.Shell.Env = stringMapField(tbl, "env", cfg.Shell.Env)
	}
	return cfg
}

func tableField(tbl *lua.LTable, key string) *lua.LTable {
	value, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return nil
	}
	return value
}

func stringField(tbl *lua.LTable, key, fallback string) string {
	if value, ok := tbl.RawGetString(key).(lua.LString); ok {
		return string(value)
	}
	return fallback
}

func intField(tbl *lua.LTable, key string, fallback int) int {
	if value, ok := tbl.RawGetString(key).(lua.LNumber); ok {
		return int(value)
	}
	return fallback
}

func numberField(tbl *lua.LTable, key string, fallback float64) float64 {
	if value, ok := tbl.RawGetString(key).(lua.LNumber); ok {
		return float64(value)
	}
	return fallback
}

func boolField(tbl *lua.LTable, key string, fallback bool) bool {
	if value, ok := tbl.RawGetString(key).(lua.LBool); ok {
		return bool(value)
	}
	return fallback
}

func stringListField(tbl *lua.LTable, key string, fallback []string) []string {
	list, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	out := make([]string, 0, list.Len())
	for i := 1; i <= list.Len(); i++ {
		if value, ok := list.RawGetInt(i).(lua.LString); ok {
			out = append(out, string(value))
		}
	}
	return out
}

func stringMapField(tbl *lua.LTable, key string, fallback map[string]string) map[string]string {
	mapTable, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	out := make(map[string]string)
	mapTable.ForEach(func(k, v lua.LValue) {
		key, keyOK := k.(lua.LString)
		value, valueOK := v.(lua.LString)
		if keyOK && valueOK {
			out[string(key)] = string(value)
		}
	})
	return out
}
