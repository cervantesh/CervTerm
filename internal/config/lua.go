package config

import (
	"fmt"
	"path/filepath"

	"cervterm/internal/fontdesc"

	lua "github.com/yuin/gopher-lua"
)

func LoadLua(path string, base Config) (Config, error) {
	return loadLua(path, path, base)
}

func loadLua(evalPath, sourcePath string, base Config) (Config, error) {
	state := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer state.Close()
	if err := state.DoFile(evalPath); err != nil {
		return base, err
	}
	value := state.Get(-1)
	table, ok := value.(*lua.LTable)
	if !ok {
		return base, fmt.Errorf("config must return a table, got %s", value.Type().String())
	}
	var document Document
	var err error
	version, versionErr := documentVersion(sourcePath, table)
	if versionErr != nil {
		return base, versionErr
	}
	hasNamedScheme := version >= 2 && (table.RawGetString("color_scheme") != lua.LNil || table.RawGetString("color_schemes") != lua.LNil)
	if hasNamedScheme {
		document, err = decodeCompositionDocument(sourcePath, table, map[string]fieldSchema{
			"color_schemes": {name: "color_schemes", kind: KindColorSchemeMap},
		})
	} else {
		document, err = DecodeDocument(sourcePath, table)
	}
	if err != nil {
		return base, err
	}
	if hasNamedScheme {
		canonical, canonicalErr := filepath.Abs(sourcePath)
		if canonicalErr != nil {
			return base, canonicalErr
		}
		graph := &SourceGraph{
			Primary: canonical,
			Sources: []SourceNode{{RequestedPath: sourcePath, CanonicalPath: canonical, SelectedPath: evalPath, Document: document}},
			state:   state,
		}
		composition, composeErr := ComposeSourceGraph(state, graph, CompositionOptions{})
		if composeErr != nil {
			return base, composeErr
		}
		return FromDocument(base, composition.Document), nil
	}
	return FromDocument(base, document), nil
}

func FromTable(cfg Config, root *lua.LTable) Config {
	if tbl := tableField(root, "window"); tbl != nil {
		cfg.Window.Width = intField(tbl, "width", cfg.Window.Width)
		cfg.Window.Height = intField(tbl, "height", cfg.Window.Height)
		cfg.Window.PaddingX = intField(tbl, "padding_x", cfg.Window.PaddingX)
		cfg.Window.PaddingY = intField(tbl, "padding_y", cfg.Window.PaddingY)
		cfg.Window.DynamicTitle = boolField(tbl, "dynamic_title", cfg.Window.DynamicTitle)
		cfg.Window.Opacity = numberField(tbl, "opacity", cfg.Window.Opacity)
		cfg.Window.TextOpacity = numberField(tbl, "text_opacity", cfg.Window.TextOpacity)
		cfg.Window.BackgroundOpacity = numberField(tbl, "background_opacity", cfg.Window.BackgroundOpacity)
		cfg.Window.Blur = boolField(tbl, "blur", cfg.Window.Blur)
	}
	if tbl := tableField(root, "font"); tbl != nil {
		cfg.Font.Family = stringField(tbl, "family", cfg.Font.Family)
		cfg.Font.Size = numberField(tbl, "size", cfg.Font.Size)
		cfg.Font.Ligatures = boolField(tbl, "ligatures", cfg.Font.Ligatures)
	}
	if tbl := tableField(root, "colors"); tbl != nil {
		cfg.Colors.Foreground = stringField(tbl, "foreground", cfg.Colors.Foreground)
		cfg.Colors.Background = stringField(tbl, "background", cfg.Colors.Background)
		cfg.Colors.Cursor = stringField(tbl, "cursor", cfg.Colors.Cursor)
		cfg.Colors.SelectionBackground = stringField(tbl, "selection_background", cfg.Colors.SelectionBackground)
		cfg.Colors.ChromeBackground = stringField(tbl, "chrome_background", cfg.Colors.ChromeBackground)
		cfg.Colors.ChromeMuted = stringField(tbl, "chrome_muted", cfg.Colors.ChromeMuted)
		cfg.Colors.Accent = stringField(tbl, "accent", cfg.Colors.Accent)
		cfg.Colors.Split = stringField(tbl, "split", cfg.Colors.Split)
		cfg.Colors.SearchMatch = stringField(tbl, "search_match", cfg.Colors.SearchMatch)
		cfg.Colors.Error = stringField(tbl, "error", cfg.Colors.Error)
		cfg.Colors.ANSI = ansiField(tbl, "ansi", cfg.Colors.ANSI)
		cfg.Colors.IndexedColors = indexedColorsField(tbl, "indexed_colors", cfg.Colors.IndexedColors)
	}
	if tbl := tableField(root, "scrolling"); tbl != nil {
		cfg.Scrolling.History = intField(tbl, "history", cfg.Scrolling.History)
		cfg.Scrolling.WheelMultiplier = intField(tbl, "wheel_multiplier", cfg.Scrolling.WheelMultiplier)
		cfg.Scrolling.HideCursorWhenScrolled = boolField(tbl, "hide_cursor_when_scrolled", cfg.Scrolling.HideCursorWhenScrolled)
	}
	if tbl := tableField(root, "scrollbar"); tbl != nil {
		cfg.Scrollbar.Enabled = boolField(tbl, "enabled", cfg.Scrollbar.Enabled)
		cfg.Scrollbar.Mode = stringField(tbl, "mode", cfg.Scrollbar.Mode)
		cfg.Scrollbar.StableGutter = boolField(tbl, "stable_gutter", cfg.Scrollbar.StableGutter)
		cfg.Scrollbar.AnimationFPS = intField(tbl, "animation_fps", cfg.Scrollbar.AnimationFPS)
		cfg.Scrollbar.ReservedWidthPX = intField(tbl, "reserved_width_px", cfg.Scrollbar.ReservedWidthPX)
		cfg.Scrollbar.WidthPX = intField(tbl, "width_px", cfg.Scrollbar.WidthPX)
		cfg.Scrollbar.MarginPX = intField(tbl, "margin_px", cfg.Scrollbar.MarginPX)
		cfg.Scrollbar.RadiusPX = intField(tbl, "radius_px", cfg.Scrollbar.RadiusPX)
		cfg.Scrollbar.MinThumbPX = intField(tbl, "min_thumb_px", cfg.Scrollbar.MinThumbPX)
		cfg.Scrollbar.TrackColor = stringField(tbl, "track_color", cfg.Scrollbar.TrackColor)
		cfg.Scrollbar.ThumbColor = stringField(tbl, "thumb_color", cfg.Scrollbar.ThumbColor)
		cfg.Scrollbar.ThumbHoverColor = stringField(tbl, "thumb_hover_color", cfg.Scrollbar.ThumbHoverColor)
		cfg.Scrollbar.ThumbPressColor = stringField(tbl, "thumb_press_color", cfg.Scrollbar.ThumbPressColor)
		cfg.Scrollbar.AutoHideDelayMS = intField(tbl, "auto_hide_delay_ms", cfg.Scrollbar.AutoHideDelayMS)
		cfg.Scrollbar.FadeMS = intField(tbl, "fade_ms", cfg.Scrollbar.FadeMS)
		cfg.Scrollbar.PageStep = numberField(tbl, "page_step", cfg.Scrollbar.PageStep)
		cfg.Scrollbar.TrackClick = stringField(tbl, "track_click", cfg.Scrollbar.TrackClick)
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
		cfg.Render.ZoomInHotkey = stringField(tbl, "zoom_in_hotkey", cfg.Render.ZoomInHotkey)
		cfg.Render.ZoomOutHotkey = stringField(tbl, "zoom_out_hotkey", cfg.Render.ZoomOutHotkey)
		cfg.Render.ZoomResetHotkey = stringField(tbl, "zoom_reset_hotkey", cfg.Render.ZoomResetHotkey)
		cfg.Render.VSync = boolField(tbl, "vsync", cfg.Render.VSync)
		cfg.Render.MaxFPS = intField(tbl, "max_fps", cfg.Render.MaxFPS)
		cfg.Render.Redraw = stringField(tbl, "redraw", cfg.Render.Redraw)
		cfg.Render.Damage = stringField(tbl, "damage", cfg.Render.Damage)
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

func descriptorListField(tbl *lua.LTable, key string, fallback []fontdesc.Descriptor) []fontdesc.Descriptor {
	list, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	out := make([]fontdesc.Descriptor, list.Len())
	for index := range out {
		entry, ok := list.RawGetInt(index + 1).(*lua.LTable)
		if !ok {
			return fallback
		}
		descriptor := fontdesc.Descriptor{
			Family:         string(entry.RawGetString("family").(lua.LString)),
			CollectionFace: stringField(entry, "collection_face", ""),
			Weight:         intField(entry, "weight", fontdesc.DefaultWeight),
			Style:          fontdesc.Style(stringField(entry, "style", string(fontdesc.StyleNormal))),
			Stretch:        intField(entry, "stretch", fontdesc.DefaultStretch),
			AttributeMode:  fontdesc.AttributeMode(stringField(entry, "attribute_mode", string(fontdesc.AttributeModeAugment))),
		}
		if raw := entry.RawGetString("collection_index"); raw != lua.LNil {
			descriptor.CollectionIndex = fontdesc.SomeCollectionIndex(uint32(raw.(lua.LNumber)))
		}
		normalized, err := descriptor.Normalize()
		if err != nil {
			return fallback
		}
		out[index] = normalized
	}
	return out
}

func fontRuleListField(tbl *lua.LTable, key string, fallback []fontdesc.Rule) []fontdesc.Rule {
	list, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	out := make([]fontdesc.Rule, list.Len())
	for index := range out {
		entry, ok := list.RawGetInt(index + 1).(*lua.LTable)
		if !ok {
			return fallback
		}
		matchTable, ok := entry.RawGetString("match").(*lua.LTable)
		if !ok {
			return fallback
		}
		use, err := parseDescriptorValue("", fmt.Sprintf("font.rules[%d].use", index+1), entry.RawGetString("use"))
		if err != nil {
			return fallback
		}
		match, _, err := parseFontRuleMatch("", fmt.Sprintf("font.rules[%d].match", index+1), matchTable)
		if err != nil {
			return fallback
		}
		rule, err := (fontdesc.Rule{Match: match, Use: use}).Normalize()
		if err != nil {
			return fallback
		}
		out[index] = rule
	}
	return out
}

func ansiField(tbl *lua.LTable, key string, fallback [16]string) [16]string {
	list, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok || list.Len() != len(fallback) {
		return fallback
	}
	var out [16]string
	for index := range out {
		value, ok := list.RawGetInt(index + 1).(lua.LString)
		if !ok {
			return fallback
		}
		out[index] = string(value)
	}
	return out
}

func indexedColorsField(tbl *lua.LTable, key string, fallback IndexedColorOverrides) IndexedColorOverrides {
	mapTable, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	var out IndexedColorOverrides
	for _, index := range indexedColorKeys(mapTable) {
		value, ok := mapTable.RawGetInt(index).(lua.LString)
		if !ok {
			continue
		}
		_ = out.Set(uint8(index), string(value))
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

func integerMapField(tbl *lua.LTable, key string, fallback map[string]int) map[string]int {
	mapTable, ok := tbl.RawGetString(key).(*lua.LTable)
	if !ok {
		return fallback
	}
	out := make(map[string]int)
	mapTable.ForEach(func(k, v lua.LValue) {
		name, keyOK := k.(lua.LString)
		number, valueOK := v.(lua.LNumber)
		if keyOK && valueOK {
			out[string(name)] = int(number)
		}
	})
	return out
}
