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
    osc52 = %q,
  },
  render = {
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
  -- },
}
`, cfg.Window.Width, cfg.Window.Height, cfg.Window.PaddingX, cfg.Window.PaddingY, cfg.Window.DynamicTitle,
		cfg.Font.Family, cfg.Font.Size,
		cfg.Colors.Foreground, cfg.Colors.Background, cfg.Colors.Cursor, cfg.Colors.SelectionBackground,
		cfg.Scrolling.History, cfg.Scrolling.WheelMultiplier, cfg.Scrolling.HideCursorWhenScrolled,
		cfg.Cursor.Shape, cfg.Cursor.Blink, cfg.Cursor.BlinkIntervalMS, cfg.Cursor.Thickness,
		cfg.Clipboard.OSC52,
		cfg.Render.Bidi,
		cfg.Shell.Program, cfg.Shell.WorkingDirectory)
}
