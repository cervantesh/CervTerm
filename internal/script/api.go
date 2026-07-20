package script

import (
	"math"

	"cervterm/internal/config"

	lua "github.com/yuin/gopher-lua"
)

// Host is the terminal surface exposed to Lua handlers. Read methods use 0-based
// row/column indices; the Lua boundary converts to 1-based.
type Host interface {
	WriteInput(data string)
	Notify(message string)
	Selection() string
	SetClipboard(text string)
	Clipboard() string
	Scroll(lines int) bool
	ScrollToBottom()
	ScrollbackLen() int
	Size() (cols, rows int)
	Cursor() (row, col int)
	Title() string
	Cwd() string
	SetTitle(title string)
	Line(row int) (string, bool)
	LineWrapped(row int) (bool, bool)
	FontSize() float64
	SetFontSize(pts float64)
	Search(query string) bool
}

// fontSizeMin and fontSizeMax bound term:set_font_size. Clamping at the Lua
// boundary keeps the frontend from having to re-validate and matches the plan.
const (
	fontSizeMin = 6.0
	fontSizeMax = 72.0
)

func termTable(state *lua.LState, host Host) *lua.LTable {
	tbl := state.NewTable()
	tbl.RawSetString("write", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.WriteInput(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("notify", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.Notify(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("selection", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Selection()))
		return 1
	}))
	tbl.RawSetString("copy", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.SetClipboard(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("clipboard", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Clipboard()))
		return 1
	}))
	tbl.RawSetString("scroll", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LBool(host.Scroll(state.CheckInt(2))))
		return 1
	}))
	tbl.RawSetString("scroll_to_bottom", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.ScrollToBottom()
		return 0
	}))
	tbl.RawSetString("scrollback", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(host.ScrollbackLen()))
		return 1
	}))
	tbl.RawSetString("size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cols, rows := host.Size()
		state.Push(lua.LNumber(cols))
		state.Push(lua.LNumber(rows))
		return 2
	}))
	tbl.RawSetString("cursor", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row, col := host.Cursor()
		state.Push(lua.LNumber(row + 1))
		state.Push(lua.LNumber(col + 1))
		return 2
	}))
	tbl.RawSetString("title", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Title()))
		return 1
	}))
	tbl.RawSetString("cwd", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(host.Cwd()))
		return 1
	}))
	tbl.RawSetString("set_title", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		host.SetTitle(state.CheckString(2))
		return 0
	}))
	tbl.RawSetString("line", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row := state.CheckInt(2)
		// Out-of-range rows yield "" so the Lua return type is always a string.
		text, _ := host.Line(row - 1)
		state.Push(lua.LString(text))
		return 1
	}))
	tbl.RawSetString("line_wrapped", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		row := state.CheckInt(2)
		wrapped, _ := host.LineWrapped(row - 1)
		state.Push(lua.LBool(wrapped))
		return 1
	}))
	tbl.RawSetString("font_size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(host.FontSize()))
		return 1
	}))
	tbl.RawSetString("set_font_size", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		pts := math.Max(fontSizeMin, math.Min(fontSizeMax, float64(state.CheckNumber(2))))
		host.SetFontSize(pts)
		return 0
	}))
	tbl.RawSetString("search", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LBool(host.Search(state.CheckString(2))))
		return 1
	}))
	tbl.RawSetString("reload_config", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		state.Push(lua.LBool(cfgHost.RequestConfigReload()))
		return 1
	}))
	tbl.RawSetString("window_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(checkRuntimeConfigHost(state, host).RuntimeConfig().Window.Opacity))
		return 1
	}))
	tbl.RawSetString("set_window_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		cfg.Window.Opacity = float64(state.CheckNumber(2))
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("text_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(checkRuntimeConfigHost(state, host).RuntimeConfig().Window.TextOpacity))
		return 1
	}))
	tbl.RawSetString("set_text_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		cfg.Window.TextOpacity = float64(state.CheckNumber(2))
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("background_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LNumber(checkRuntimeConfigHost(state, host).RuntimeConfig().Window.BackgroundOpacity))
		return 1
	}))
	tbl.RawSetString("set_background_opacity", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		cfg.Window.BackgroundOpacity = float64(state.CheckNumber(2))
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("background", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LString(checkRuntimeConfigHost(state, host).RuntimeConfig().Colors.Background))
		return 1
	}))
	tbl.RawSetString("set_background", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		cfg.Colors.Background = state.CheckString(2)
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("blur", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(lua.LBool(checkRuntimeConfigHost(state, host).RuntimeConfig().Window.Blur))
		return 1
	}))
	tbl.RawSetString("set_blur", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		cfg.Window.Blur = state.CheckBool(2)
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("scrolling", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(scrollingTable(state, checkRuntimeConfigHost(state, host).RuntimeConfig().Scrolling))
		return 1
	}))
	tbl.RawSetString("set_scrolling", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		fromScrollingTable(state, state.CheckTable(2), &cfg.Scrolling)
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	tbl.RawSetString("scrollbar", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		state.Push(scrollbarTable(state, checkRuntimeConfigHost(state, host).RuntimeConfig().Scrollbar))
		return 1
	}))
	tbl.RawSetString("set_scrollbar", state.NewFunction(func(state *lua.LState) int {
		state.CheckTypes(1, lua.LTTable)
		cfgHost := checkRuntimeConfigHost(state, host)
		cfg := cfgHost.RuntimeConfig()
		fromScrollbarTable(state, state.CheckTable(2), &cfg.Scrollbar)
		applyRuntimeConfig(state, cfgHost, cfg)
		return 0
	}))
	return tbl
}

type runtimeConfigHost interface {
	RuntimeConfig() config.Config
	ApplyRuntimeConfig(config.Config) error
	RequestConfigReload() bool
}

func checkRuntimeConfigHost(state *lua.LState, host Host) runtimeConfigHost {
	h, ok := host.(runtimeConfigHost)
	if !ok {
		state.RaiseError("runtime configuration is unavailable")
	}
	return h
}

func applyRuntimeConfig(state *lua.LState, host runtimeConfigHost, cfg config.Config) {
	if err := host.ApplyRuntimeConfig(cfg); err != nil {
		state.RaiseError("invalid runtime config: %v", err)
	}
}

func scrollingTable(state *lua.LState, cfg config.ScrollingConfig) *lua.LTable {
	t := state.NewTable()
	t.RawSetString("history", lua.LNumber(cfg.History))
	t.RawSetString("wheel_multiplier", lua.LNumber(cfg.WheelMultiplier))
	t.RawSetString("hide_cursor_when_scrolled", lua.LBool(cfg.HideCursorWhenScrolled))
	return t
}

func fromScrollingTable(state *lua.LState, t *lua.LTable, cfg *config.ScrollingConfig) {
	optionalInt(state, t, "history", &cfg.History)
	optionalInt(state, t, "wheel_multiplier", &cfg.WheelMultiplier)
	optionalBool(state, t, "hide_cursor_when_scrolled", &cfg.HideCursorWhenScrolled)
}

func scrollbarTable(state *lua.LState, cfg config.ScrollbarConfig) *lua.LTable {
	t := state.NewTable()
	t.RawSetString("enabled", lua.LBool(cfg.Enabled))
	t.RawSetString("reserved_width_px", lua.LNumber(cfg.ReservedWidthPX))
	t.RawSetString("width_px", lua.LNumber(cfg.WidthPX))
	t.RawSetString("margin_px", lua.LNumber(cfg.MarginPX))
	t.RawSetString("radius_px", lua.LNumber(cfg.RadiusPX))
	t.RawSetString("min_thumb_px", lua.LNumber(cfg.MinThumbPX))
	t.RawSetString("track_color", lua.LString(cfg.TrackColor))
	t.RawSetString("thumb_color", lua.LString(cfg.ThumbColor))
	t.RawSetString("thumb_hover_color", lua.LString(cfg.ThumbHoverColor))
	t.RawSetString("thumb_press_color", lua.LString(cfg.ThumbPressColor))
	t.RawSetString("auto_hide_delay_ms", lua.LNumber(cfg.AutoHideDelayMS))
	t.RawSetString("fade_ms", lua.LNumber(cfg.FadeMS))
	t.RawSetString("page_step", lua.LNumber(cfg.PageStep))
	t.RawSetString("track_click", lua.LString(cfg.TrackClick))
	return t
}

func fromScrollbarTable(state *lua.LState, t *lua.LTable, cfg *config.ScrollbarConfig) {
	optionalBool(state, t, "enabled", &cfg.Enabled)
	optionalInt(state, t, "reserved_width_px", &cfg.ReservedWidthPX)
	optionalInt(state, t, "width_px", &cfg.WidthPX)
	optionalInt(state, t, "margin_px", &cfg.MarginPX)
	optionalInt(state, t, "radius_px", &cfg.RadiusPX)
	optionalInt(state, t, "min_thumb_px", &cfg.MinThumbPX)
	optionalString(state, t, "track_color", &cfg.TrackColor)
	optionalString(state, t, "thumb_color", &cfg.ThumbColor)
	optionalString(state, t, "thumb_hover_color", &cfg.ThumbHoverColor)
	optionalString(state, t, "thumb_press_color", &cfg.ThumbPressColor)
	optionalInt(state, t, "auto_hide_delay_ms", &cfg.AutoHideDelayMS)
	optionalInt(state, t, "fade_ms", &cfg.FadeMS)
	optionalNumber(state, t, "page_step", &cfg.PageStep)
	optionalString(state, t, "track_click", &cfg.TrackClick)
}

func optionalInt(state *lua.LState, t *lua.LTable, key string, dst *int) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return
	}
	n, ok := v.(lua.LNumber)
	if !ok {
		state.RaiseError("%s must be a number", key)
	}
	*dst = int(n)
}

func optionalNumber(state *lua.LState, t *lua.LTable, key string, dst *float64) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return
	}
	n, ok := v.(lua.LNumber)
	if !ok {
		state.RaiseError("%s must be a number", key)
	}
	*dst = float64(n)
}

func optionalBool(state *lua.LState, t *lua.LTable, key string, dst *bool) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return
	}
	b, ok := v.(lua.LBool)
	if !ok {
		state.RaiseError("%s must be a boolean", key)
	}
	*dst = bool(b)
}

func optionalString(state *lua.LState, t *lua.LTable, key string, dst *string) {
	v := t.RawGetString(key)
	if v == lua.LNil {
		return
	}
	s, ok := v.(lua.LString)
	if !ok {
		state.RaiseError("%s must be a string", key)
	}
	*dst = string(s)
}
