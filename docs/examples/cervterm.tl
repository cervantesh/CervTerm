local cervterm = require("cervterm")

local _shades_of_purple: cervterm.ColorScheme = {
  foreground = "#FFFFFF",
  background = "#2D2B55",
  cursor = "#FAD000",
  selection_background = "#B362FF",
  accent = "#FAD000FF",
  search_match = "#6943FFCC",
  ansi = {
    "#000000", "#D90429", "#3AD900", "#FFE700",
    "#6943FF", "#FF2C70", "#80FCFF", "#C7C7C7",
    "#686868", "#F9555F", "#5CFF45", "#FFFF85",
    "#6871FF", "#FF77FF", "#79E8FB", "#FFFFFF",
  },
}

local config: cervterm.Config = {
  config_version = 2,
  window = {
    width = 1100,
    height = 720,
    padding_x = 6,
    padding_y = 6,
    dynamic_title = true,
    opacity = 1.0,
    blur = true,
  },
  font = {
    family = "Go Mono",
    size = 14.0,
    ligatures = false,
  },
  color_scheme = "Shades of Purple",
  color_schemes = { ["Shades of Purple"] = _shades_of_purple },
  scrolling = {
    history = 2000,
    wheel_multiplier = 3,
    hide_cursor_when_scrolled = true,
  },
  scrollbar = {
    enabled = true,
    reserved_width_px = 12,
    width_px = 8,
    margin_px = 2,
    radius_px = 4,
    min_thumb_px = 24,
    track_color = "#10172266",
    thumb_color = "#60E8F0CC",
    thumb_hover_color = "#7CF4F9E6",
    thumb_press_color = "#B6FAFFFF",
    auto_hide_delay_ms = 1000,
    fade_ms = 150,
    page_step = 0.9,
    track_click = "page",
  },
  cursor = {
    shape = "underline",
    blink = true,
    blink_interval_ms = 1000,
    thickness = 0.15,
  },
  clipboard = {
    osc52 = "off",
  },
  render = {
    bidi = false,
    text_gamma = 1.15,
    text_darken = 0.0,
    text_raster = "go",
    stats_hotkey = "ctrl+shift+i",
    zoom_in_hotkey = "ctrl+equal",
    zoom_out_hotkey = "ctrl+minus",
    zoom_reset_hotkey = "ctrl+0",
    vsync = true,
    redraw = "on_demand",
    damage = "rows",
  },
  shell = {
    program = "",
    args = {},
    working_directory = "",
    env = {},
  },
}

return config
