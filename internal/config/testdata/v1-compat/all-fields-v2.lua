return {
  config_version = 2,
  window = { width = 1200, height = 800, padding_x = 8, padding_y = 9, dynamic_title = false, opacity = 0.9, blur = false },
  font = { family = "JetBrains Mono", size = 15.5, ligatures = true },
  colors = { foreground = "#010203", background = "#040506FF", cursor = "#070809", selection_background = "#0A0B0C" },
  scrolling = { history = 3210, wheel_multiplier = 4, hide_cursor_when_scrolled = false },
  scrollbar = {
    enabled = true, reserved_width_px = 16, width_px = 10, margin_px = 3, radius_px = 5, min_thumb_px = 30,
    track_color = "#10111266", thumb_color = "#131415CC", thumb_hover_color = "#161718E6", thumb_press_color = "#191A1BFF",
    auto_hide_delay_ms = 900, fade_ms = 175, page_step = 0.75, track_click = "jump",
  },
  cursor = { shape = "beam", blink = false, blink_interval_ms = 700, thickness = 0.2 },
  clipboard = { osc52 = "write" },
  render = {
    bidi = true, text_gamma = 1.2, text_darken = 0.1, text_raster = "auto", stats_hotkey = "ctrl+shift+s",
    zoom_in_hotkey = "ctrl+equal", zoom_out_hotkey = "ctrl+minus", zoom_reset_hotkey = "ctrl+0",
    vsync = false, redraw = "continuous", damage = "frame",
  },
  shell = { program = "pwsh", args = {"-NoLogo"}, working_directory = "C:/work", env = {TERM_PROGRAM = "CervTerm"} },
}
