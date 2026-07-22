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
    -- Set both to 0 for the existing 1100x720 pixel startup, or both 10..1000.
    initial_rows = %d,
    initial_cols = %d,
    -- Native creation controls: decorations = "system"|"none"; titlebar = "system"|"dark".
    decorations = %q,
    titlebar = %q,
    padding_x = %d,
    padding_y = %d,
    -- Per-side values override padding_x/padding_y in config_version = 2.
    padding_left = %d,
    padding_right = %d,
    padding_top = %d,
    padding_bottom = %d,
    dynamic_title = %t,
    -- Whole-window opacity requires an opaque background (#RRGGBB or alpha FF).
    -- Blur is best-effort; unsupported platforms keep transparency without blur.
    opacity = %.2f,
    -- These affect terminal glyph/background content, not cursor or chrome.
    text_opacity = %.2f,
    background_opacity = %.2f,
    blur = %t,
  },
  -- Layers replace as one list across includes/profiles. Image paths resolve
  -- relative to the source that supplied the winning list.
  -- background = {
  --   layers = {
  --     { kind = "linear_gradient", colors = { "#080B12", "#182040" }, angle = 90 },
  --     { kind = "image", path = "background.png", fit = "cover", opacity = 0.25 },
  --   },
  -- },
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
    -- OpenType feature tags merge by key across config layers. Explicit entries
    -- override the ligatures projection; cervterm.config.unset reveals it again.
    -- Values are integers 0..65535 and at most 64 effective tags are allowed.
    -- features = { liga = 0, clig = 0, calt = 0, dlig = 1, ss01 = 1 },
    -- Fixed-grid metric projection; scale bounds are 0.5..3.0 and pixel offsets
    -- are -64..64. All changes require restart and never alter per-glyph advances.
    line_height = %.2f,
    cell_width = %.2f,
    baseline_offset = %.2f,
    glyph_offset_x = %.2f,
    glyph_offset_y = %.2f,
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
    -- mode: always, hover, scrolling, or never. In v2 it shadows enabled.
    enabled = %t,
    mode = %q,
    -- Stable reserves the gutter; false overlays chrome on terminal content.
    stable_gutter = %t,
    -- Active fades are sampled at this cadence; idle adds no periodic wake.
    animation_fps = %d,
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
  tab_bar = {
    -- Visibility: multiple (only with 2+ tabs), always, or hidden.
    mode = %q,
    position = %q,
    height_px = %d,
    min_width_px = %d,
    max_width_px = %d,
    padding_x = %d,
    show_new_button = %t,
    show_close_button = %t,
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
  ime = {
    -- Native Windows IME/preedit integration; restart required. Disabled by default.
    enabled = %t,
  },
	accessibility = {
		-- Windows UI Automation integration; restart required. Disabled by default.
		enabled = %t,
		scope = %q, -- visible is the only supported privacy scope
	},
	graphics = {
		kitty = {
			-- Experimental direct-data Kitty subset; default off and restart required.
			enabled = %t,
		},
		sixel = {
			-- Dormant Phase 14 intent only; frontend activation is not wired yet.
			enabled = %t,
		},
		iterm = {
			-- Dormant Phase 14 intent only; frontend activation is not wired yet.
			enabled = %t,
		},
		limits = {
			encoded_bytes_per_pane = %d,
			decoded_bytes_per_pane = %d,
			image_count_per_pane = %d,
			placement_count_per_pane = %d,
			gpu_bytes_per_context = %d,
		},
	},
  bell = {
    -- Sink effects are disabled by default; Lua bell callbacks remain lossless.
    mode = %q, -- disabled, audible, visual, or taskbar
    focus = %q, -- always or unfocused
    throttle_ms = %d,
    visual_duration_ms = %d,
  },
  notification = {
    -- Explicit consent; terminal output cannot enable this policy.
    enabled = %t,
    focus = %q, -- always or unfocused
    rate_limit_ms = %d,
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
    -- Vsync still applies independently. max_fps = 0 disables CervTerm's explicit
    -- presentation cap; a positive value limits completed frames and is live reloadable.
    vsync = %t,
    max_fps = %d,
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
  --   { key = "o", mods = "ctrl+shift", action = cervterm.action.CopySemanticZone("output") },
  --   { key = "i", mods = "ctrl+shift", action = cervterm.action.SelectSemanticZone("input") },
  --   { key = "p", mods = "ctrl+shift", action = cervterm.action.ActivateCommandPalette },
  --   { key = "q", mods = "ctrl+shift", action = cervterm.action.ActivateQuickSelect },
  --   { key = "l", mods = "ctrl+shift", action = cervterm.action.ActivateLaunchMenu },
  --   { key = "k", mods = "ctrl", action = cervterm.action.ScrollPage(1) },
  --   { key = "up", mods = "ctrl+shift", action = cervterm.action.ScrollToPrompt(-1) },
  --   { key = "equal", mods = "ctrl", action = cervterm.action.Zoom(1) },
  --   { key = "d", mods = "alt+shift", action = cervterm.action.SplitPane("columns") },
  --   { key = "r", mods = "alt+shift", action = cervterm.action.ResizePane("right", 3) },
  --   { key = "s", mods = "alt+shift", action = cervterm.action.SwapPane("left") },
  --   { key = "v", mods = "alt+shift", action = cervterm.action.MovePane("down") },
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
  -- leader = { key = "a", mods = "ctrl", timeout_ms = 1000 },
  -- key_tables = {
  --   { name = "pane", one_shot = true, timeout_ms = 1500, keys = {
  --     { key = "h", action = cervterm.action.FocusPane("left") },
  --   } },
  -- },
  -- mouse_bindings = {
  --   { event = "press", button = "right", mods = "shift", action = cervterm.action.PasteClipboard },
  -- },
  -- quick_select = {
  --   rules = {
  --     { id = "issue", pattern = "[A-Z]+-[0-9]+", action = "copy", priority = 10 },
  --   },
  -- },
  -- launch_menu = {
  --   { id = "powershell", label = "PowerShell", program = "pwsh", args = { "-NoLogo" }, cwd = "", env = {} },
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
`, cfg.Window.Width, cfg.Window.Height, cfg.Window.InitialRows, cfg.Window.InitialCols, cfg.Window.Decorations, cfg.Window.Titlebar, cfg.Window.PaddingX, cfg.Window.PaddingY,
		cfg.Window.PaddingLeft, cfg.Window.PaddingRight, cfg.Window.PaddingTop, cfg.Window.PaddingBottom,
		cfg.Window.DynamicTitle, cfg.Window.Opacity, cfg.Window.TextOpacity, cfg.Window.BackgroundOpacity, cfg.Window.Blur,
		cfg.Font.Family, cfg.Font.Size, cfg.Font.Ligatures, cfg.Font.LineHeight, cfg.Font.CellWidth, cfg.Font.BaselineOffset, cfg.Font.GlyphOffsetX, cfg.Font.GlyphOffsetY,
		cfg.Colors.Foreground, cfg.Colors.Background, cfg.Colors.Cursor, cfg.Colors.SelectionBackground,
		cfg.Colors.ChromeBackground, cfg.Colors.ChromeMuted, cfg.Colors.Accent, cfg.Colors.Split, cfg.Colors.SearchMatch, cfg.Colors.Error,
		cfg.Colors.ANSI[0], cfg.Colors.ANSI[1], cfg.Colors.ANSI[2], cfg.Colors.ANSI[3],
		cfg.Colors.ANSI[4], cfg.Colors.ANSI[5], cfg.Colors.ANSI[6], cfg.Colors.ANSI[7],
		cfg.Colors.ANSI[8], cfg.Colors.ANSI[9], cfg.Colors.ANSI[10], cfg.Colors.ANSI[11],
		cfg.Colors.ANSI[12], cfg.Colors.ANSI[13], cfg.Colors.ANSI[14], cfg.Colors.ANSI[15],
		cfg.Scrolling.History, cfg.Scrolling.WheelMultiplier, cfg.Scrolling.HideCursorWhenScrolled,
		cfg.Scrollbar.Enabled, cfg.Scrollbar.Mode, cfg.Scrollbar.StableGutter, cfg.Scrollbar.AnimationFPS,
		cfg.Scrollbar.ReservedWidthPX, cfg.Scrollbar.WidthPX, cfg.Scrollbar.MarginPX, cfg.Scrollbar.RadiusPX, cfg.Scrollbar.MinThumbPX,
		cfg.Scrollbar.TrackColor, cfg.Scrollbar.ThumbColor, cfg.Scrollbar.ThumbHoverColor, cfg.Scrollbar.ThumbPressColor,
		cfg.Scrollbar.AutoHideDelayMS, cfg.Scrollbar.FadeMS, cfg.Scrollbar.PageStep, cfg.Scrollbar.TrackClick,
		cfg.TabBar.Mode, cfg.TabBar.Position, cfg.TabBar.HeightPX, cfg.TabBar.MinWidthPX, cfg.TabBar.MaxWidthPX, cfg.TabBar.PaddingX, cfg.TabBar.ShowNewButton, cfg.TabBar.ShowCloseButton,
		cfg.Cursor.Shape, cfg.Cursor.Blink, cfg.Cursor.BlinkIntervalMS, cfg.Cursor.Thickness,
		cfg.Clipboard.OSC52,
		cfg.IME.Enabled,
		cfg.Accessibility.Enabled, cfg.Accessibility.Scope,
		cfg.Graphics.Kitty.Enabled, cfg.Graphics.Sixel.Enabled, cfg.Graphics.ITerm.Enabled,
		cfg.Graphics.Limits.EncodedBytesPerPane, cfg.Graphics.Limits.DecodedBytesPerPane, cfg.Graphics.Limits.ImageCountPerPane, cfg.Graphics.Limits.PlacementCountPerPane, cfg.Graphics.Limits.GPUBytesPerContext,
		cfg.Bell.Mode, cfg.Bell.Focus, cfg.Bell.ThrottleMS, cfg.Bell.VisualDurationMS,
		cfg.Notification.Enabled, cfg.Notification.Focus, cfg.Notification.RateLimitMS,
		cfg.Render.TextRaster, cfg.Render.TextGamma, cfg.Render.TextDarken, cfg.Render.StatsHotkey, cfg.Render.ZoomInHotkey, cfg.Render.ZoomOutHotkey, cfg.Render.ZoomResetHotkey, cfg.Render.VSync, cfg.Render.MaxFPS, cfg.Render.Redraw, cfg.Render.Damage, cfg.Render.Bidi,
		cfg.Shell.Program, cfg.Shell.WorkingDirectory)
}
