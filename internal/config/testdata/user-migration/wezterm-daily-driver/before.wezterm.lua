-- Sanitized owner-provided WezTerm daily-driver configuration.
local wezterm = require "wezterm"
local config = wezterm.config_builder()

config.color_scheme = "shades-of-purple"
config.window_background_opacity = 0.98
config.text_background_opacity = 0.98
config.window_decorations = "TITLE|RESIZE"
config.window_padding = { left = 12, right = 12, top = 0, bottom = 0 }
config.font = wezterm.font_with_fallback {
  "JetBrainsMono Nerd Font",
  "CaskaydiaCove Nerd Font",
  "Cascadia Mono",
  "Consolas",
}
config.font_size = 12.5
config.line_height = 1.08
config.default_cursor_style = "SteadyBar"
config.cursor_blink_rate = 0
config.enable_tab_bar = true
config.hide_tab_bar_if_only_one_tab = false
config.tab_bar_at_bottom = true
config.tab_max_width = 28
config.scrollback_lines = 10000
config.audible_bell = "Disabled"
config.initial_cols = 120
config.initial_rows = 32
config.enable_scroll_bar = true
config.colors = { scrollbar_thumb = "#a599e9" }
config.max_fps = 60

return config
