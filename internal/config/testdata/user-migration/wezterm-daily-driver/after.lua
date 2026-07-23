-- CervTerm v2 translation of the sanitized WezTerm daily-driver example.
return {
  config_version = 2,
  window = {
    initial_cols = 120,
    initial_rows = 32,
    decorations = "system",
    titlebar = "dark",
    padding_left = 12,
    padding_right = 12,
    padding_top = 0,
    padding_bottom = 0,
    opacity = 0.98,
    text_opacity = 0.98,
    background_opacity = 0.98,
    blur = false,
  },
  font = {
    family = "JetBrainsMono Nerd Font",
    descriptors = {
      { family = "JetBrainsMono Nerd Font", weight = 400, style = "normal", stretch = 100, attribute_mode = "augment" },
    },
    fallback = {
      { family = "CaskaydiaCove Nerd Font" },
      { family = "Cascadia Mono" },
      { family = "Consolas" },
    },
    size = 12.5,
    ligatures = true,
    line_height = 1.08,
  },
  colors = {
    foreground = "#FFFFFF",
    background = "#1E1D40",
    cursor = "#FAD000",
    selection_background = "#B362FF",
  },
  scrolling = {
    history = 10000,
    wheel_multiplier = 3,
    hide_cursor_when_scrolled = true,
  },
  scrollbar = {
    enabled = true,
    mode = "always",
    stable_gutter = true,
    min_thumb_px = 40,
    thumb_color = "#A599E9CC",
  },
  tab_bar = {
    mode = "always",
    position = "bottom",
    max_width_px = 220,
  },
  cursor = {
    shape = "beam",
    blink = false,
    blink_interval_ms = 1000,
    thickness = 0.15,
  },
  bell = {
    mode = "disabled",
    focus = "unfocused",
    throttle_ms = 250,
    visual_duration_ms = 120,
  },
  render = {
    max_fps = 60,
  },
}
