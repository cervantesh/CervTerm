package config

import "fmt"

func DefaultLua() string {
	cfg := Defaults()
	return fmt.Sprintf(`-- CervTerm default configuration.
-- Save as cervterm.lua and edit values as needed.
return {
  window = {
    width = %d,
    height = %d,
    padding_x = %d,
    padding_y = %d,
    dynamic_title = %t,
  },
  font = {
    family = %q,
    size = %.1f,
    -- Render programming ligatures (-> => != === etc.) when the font provides
    -- them (Fira Code, Cascadia Code, JetBrains Mono). No effect with fonts
    -- that lack ligatures (e.g. the default Go Mono). Render-only: grid,
    -- selection, and copied text are unchanged.
    ligatures = %t,
  },
  colors = {
    foreground = %q,
    background = %q,
    cursor = %q,
    selection_background = %q,
  },
  scrolling = {
    history = %d,
    wheel_multiplier = %d,
    hide_cursor_when_scrolled = %t,
  },
  cursor = {
    shape = %q,
    blink = %t,
    blink_interval_ms = %d,
    thickness = %.2f,
  },
  clipboard = {
    -- "off" (safe default) ignores OSC 52 writes from the PTY. "write" allows
    -- remote content to overwrite the clipboard (clipboard hijacking risk).
    -- Clipboard reads via OSC 52 are always denied.
    osc52 = %q,
  },
  render = {
    -- go, auto (DirectWrite on Windows), or subpixel (horizontal RGB LCDs)
    text_raster = %q,
    text_gamma = %.2f,
    text_darken = %.2f,
    -- Hotkey that toggles the two-row stats overlay (empty disables it).
    stats_hotkey = %q,
    -- Runtime zoom (font size is clamped to 6..72; empty disables a binding).
    -- "ctrl+equal" = Ctrl and the +/= key; "ctrl+minus" = Ctrl and -;
    -- reset returns to the configured font.size.
    zoom_in_hotkey = %q,
    zoom_out_hotkey = %q,
    zoom_reset_hotkey = %q,
    -- Cap the frame rate to the monitor refresh. Set false to uncap (higher
    -- FPS number, more CPU/GPU; useful only for benchmarking).
    vsync = %t,
    -- "on_demand" (default) redraws only when something visible changed, so an
    -- idle terminal draws ~0 fps. "continuous" redraws every loop iteration
    -- (the old always-draw behavior; useful for benchmarking or as an escape hatch).
    redraw = %q,
    -- "frame" is an escape hatch if partial-redraw artifacts appear.
    damage = %q,
    -- Experimental visual reordering for RTL text; logical storage is unchanged.
    bidi = %t,
  },
  shell = {
    program = %q,
    args = {},
    working_directory = %q,
    env = {},
  },
  -- keys = {
  --   {
  --     key = "p",
  --     mods = "ctrl+shift",
  --     action = function(term)
  --       term:write("echo hola desde lua\r")
  --       term:notify("saludo enviado")
  --     end,
  --   },
  --   -- Runtime zoom (font size is clamped to 6..72):
  --   { key = "equal", mods = "ctrl", action = function(term) term:set_font_size(term:font_size() + 1) end },
  --   { key = "minus", mods = "ctrl", action = function(term) term:set_font_size(term:font_size() - 1) end },
  -- },
  -- events = {
  --   output = function(term, data) end,
  --   title = function(term, title) end,
  --   bell = function(term) term:notify("ding") end,
  --   resize = function(term, cols, rows) end,
  --   focus = function(term, focused) end,
  --   scroll = function(term, offset) end,
  -- },
  -- Timers integrate with the on-demand wake loop (no busy polling). Register
  -- them at the top of the file, before "return {":
  --   local cervterm = require("cervterm")
  --   cervterm.every(1000, function(term) term:set_title(os.date("%%H:%%M:%%S")) end)
}
`, cfg.Window.Width, cfg.Window.Height, cfg.Window.PaddingX, cfg.Window.PaddingY, cfg.Window.DynamicTitle,
		cfg.Font.Family, cfg.Font.Size, cfg.Font.Ligatures,
		cfg.Colors.Foreground, cfg.Colors.Background, cfg.Colors.Cursor, cfg.Colors.SelectionBackground,
		cfg.Scrolling.History, cfg.Scrolling.WheelMultiplier, cfg.Scrolling.HideCursorWhenScrolled,
		cfg.Cursor.Shape, cfg.Cursor.Blink, cfg.Cursor.BlinkIntervalMS, cfg.Cursor.Thickness,
		cfg.Clipboard.OSC52,
		cfg.Render.TextRaster, cfg.Render.TextGamma, cfg.Render.TextDarken, cfg.Render.StatsHotkey, cfg.Render.ZoomInHotkey, cfg.Render.ZoomOutHotkey, cfg.Render.ZoomResetHotkey, cfg.Render.VSync, cfg.Render.Redraw, cfg.Render.Damage, cfg.Render.Bidi,
		cfg.Shell.Program, cfg.Shell.WorkingDirectory)
}
