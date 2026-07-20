return {
  unknown_root = "ignored",
  window = {
    width = 123.9,
    height = "wrong type",
    padding_x = 9,
    unknown_window = true,
  },
  scrolling = {
    history = 42.8,
    wheel_multiplier = "wrong type",
  },
  shell = {
    args = { "pwsh", 7, "-NoLogo" },
    env = {
      GOOD = "yes",
      BAD_VALUE = 7,
      [9] = "bad key",
    },
  },
}
