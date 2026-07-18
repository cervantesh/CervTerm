package config

import "fmt"

func DefaultLua() string {
	cfg := Defaults()
	return fmt.Sprintf(`-- CervTerm default configuration.
-- Save as cervterm.lua and edit values as needed.
-- Uncomment when using typed key actions below:
-- local cervterm = require("cervterm")
return {
  config_version = 2,
  -- Select a locally declared reusable palette before applying colors below.
  -- color_scheme = "My Scheme",
  -- color_schemes = {
  --   ["My Scheme"] = {
  --     foreground = "#E6E1D8",
  --     background = "#080B12",
  --     cursor = "#60E8F0",
  --     selection_background = "#2A6377",
  --     accent = "#60E8F0FF",
  --     search_match = "#7A5C12FF",
  --   },
  -- },
  window = {
    width = %d,
    height = %d,
    padding_x = %d,
    padding_y = %d,
    dynamic_title = %t,
    -- Whole-window opacity requires an opaque background (#RRGGBB or alpha FF).
    -- Blur is best-effort; unsupported platforms keep transparency without blur.
    opacity = %.2f,
    blur = %t,
  },
  font = {
    family = %q,
    -- Optional ordered primary face descriptors replace family selection as a
    -- whole list. Omitted attributes default to weight=400, style="normal",
    -- stretch=100, and attribute_mode="augment". collection_face and
    -- collection_index are mutually exclusive; index 0 is an explicit value.
    -- descriptors = {
    --   { family = "Example Mono", weight = 400, style = "normal",
    --     stretch = 100, attribute_mode = "augment" },
    --   { family = "Example Collection", collection_index = 0 },
    -- },
    -- Ordered fallback descriptors are tried lazily after rules and primaries.
    -- fallback = { { family = "Example Symbols" }, { family = "Example CJK" } },
    -- Rules are first-match face routes. A match may combine requested weight/
    -- stretch ranges, a style set, up to 64 inclusive scalar ranges, and one
    -- class: emoji, cjk, nerd_font, powerline, box_drawing, braille, or symbols.
    -- rules = {
    --   { match = { class = "emoji", styles = { "normal", "italic" } },
    --     use = { family = "Example Emoji", attribute_mode = "fixed" } },
    --   { match = { ranges = { { first = 0xE000, last = 0xF8FF } } },
    --     use = { family = "Example Icons" } },
    -- },
    size = %.1f,
    -- Render programming ligatures (-> => != === etc.) when the font provides
    -- them (Fira Code, Cascadia Code, JetBrains Mono). No effect with fonts
    -- that lack ligatures (e.g. the default Go Mono). Render-only: grid,
    -- selection, and copied text are unchanged.
    ligatures = %t,
  },
  colors = {
    foreground = %q,
    -- #RRGGBB or #RRGGBBAA; default E6 is about 90%% background opacity.
    background = %q,
    cursor = %q,
    selection_background = %q,
    -- Semantic application chrome; #RRGGBB or #RRGGBBAA.
    chrome_background = %q,
    chrome_muted = %q,
    accent = %q,
    split = %q,
    search_match = %q,
    error = %q,
    -- black/red/green/yellow/blue/purple/cyan/white, then bright variants.
    -- ANSI entries require exactly #RRGGBB (alpha is not accepted).
    ansi = {
      %q, %q, %q, %q,
      %q, %q, %q, %q,
      %q, %q, %q, %q,
      %q, %q, %q, %q,
    },
    -- Optional sparse xterm overrides; numeric keys must be 16..255.
    -- indexed_colors = { [16] = "#102030", [196] = "#FF1010" },
  },
  scrolling = {
    -- Retained scrollback rows per pane; valid range 0..10000.
    history = %d,
    wheel_multiplier = %d,
    hide_cursor_when_scrolled = %t,
  },
  scrollbar = {
    -- The slot stays reserved while the track/thumb auto-hide and fade.
    enabled = %t,
    reserved_width_px = %d,
    width_px = %d,
    margin_px = %d,
    radius_px = %d,
    min_thumb_px = %d,
    track_color = %q,
    thumb_color = %q,
    thumb_hover_color = %q,
    thumb_press_color = %q,
    auto_hide_delay_ms = %d,
    fade_ms = %d,
    page_step = %.2f,
    track_click = %q,
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
  -- Native pane defaults (Lua bindings below take precedence):
  -- Alt+Shift+= split right; Alt+Shift+- split below; Alt+Arrow focus;
  -- Ctrl+Shift+W close focused pane.

  -- keys = {
  --   -- Typed actions are validated while loading and share built-in behavior:
  --   { key = "c", mods = "ctrl+shift", action = cervterm.action.CopySelection },
  --   { key = "k", mods = "ctrl", action = cervterm.action.ScrollPage(1) },
  --   { key = "equal", mods = "ctrl", action = cervterm.action.Zoom(1) },
  --   { key = "d", mods = "alt+shift", action = cervterm.action.SplitPane("columns") },
  --   { key = "m", mods = "ctrl+shift", action = cervterm.action.Multiple({
  --     cervterm.action.FocusPane("left"), cervterm.action.ClosePane,
  --   }) },
  --   -- Function callbacks remain supported and run through the one-second watchdog:
  --   {
  --     key = "p",
  --     mods = "ctrl+shift",
  --     label = "Send greeting",
  --     action = function(term)
  --       term:write("echo hola desde lua\r")
  --       term:notify("saludo enviado")
  --     end,
  --   },
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
`, cfg.Window.Width, cfg.Window.Height, cfg.Window.PaddingX, cfg.Window.PaddingY, cfg.Window.DynamicTitle, cfg.Window.Opacity, cfg.Window.Blur,
		cfg.Font.Family, cfg.Font.Size, cfg.Font.Ligatures,
		cfg.Colors.Foreground, cfg.Colors.Background, cfg.Colors.Cursor, cfg.Colors.SelectionBackground,
		cfg.Colors.ChromeBackground, cfg.Colors.ChromeMuted, cfg.Colors.Accent, cfg.Colors.Split, cfg.Colors.SearchMatch, cfg.Colors.Error,
		cfg.Colors.ANSI[0], cfg.Colors.ANSI[1], cfg.Colors.ANSI[2], cfg.Colors.ANSI[3],
		cfg.Colors.ANSI[4], cfg.Colors.ANSI[5], cfg.Colors.ANSI[6], cfg.Colors.ANSI[7],
		cfg.Colors.ANSI[8], cfg.Colors.ANSI[9], cfg.Colors.ANSI[10], cfg.Colors.ANSI[11],
		cfg.Colors.ANSI[12], cfg.Colors.ANSI[13], cfg.Colors.ANSI[14], cfg.Colors.ANSI[15],
		cfg.Scrolling.History, cfg.Scrolling.WheelMultiplier, cfg.Scrolling.HideCursorWhenScrolled,
		cfg.Scrollbar.Enabled, cfg.Scrollbar.ReservedWidthPX, cfg.Scrollbar.WidthPX, cfg.Scrollbar.MarginPX, cfg.Scrollbar.RadiusPX, cfg.Scrollbar.MinThumbPX,
		cfg.Scrollbar.TrackColor, cfg.Scrollbar.ThumbColor, cfg.Scrollbar.ThumbHoverColor, cfg.Scrollbar.ThumbPressColor,
		cfg.Scrollbar.AutoHideDelayMS, cfg.Scrollbar.FadeMS, cfg.Scrollbar.PageStep, cfg.Scrollbar.TrackClick,
		cfg.Cursor.Shape, cfg.Cursor.Blink, cfg.Cursor.BlinkIntervalMS, cfg.Cursor.Thickness,
		cfg.Clipboard.OSC52,
		cfg.Render.TextRaster, cfg.Render.TextGamma, cfg.Render.TextDarken, cfg.Render.StatsHotkey, cfg.Render.ZoomInHotkey, cfg.Render.ZoomOutHotkey, cfg.Render.ZoomResetHotkey, cfg.Render.VSync, cfg.Render.Redraw, cfg.Render.Damage, cfg.Render.Bidi,
		cfg.Shell.Program, cfg.Shell.WorkingDirectory)
}
