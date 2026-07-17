local cervterm = require("cervterm")

local config = {
  keys = {
    {
      key = "c",
      mods = "ctrl+shift",
      label = "Copy selection",
      action = cervterm.action.CopySelection,
    },
    {
      key = "k",
      mods = "ctrl",
      action = cervterm.action.ScrollPage(1),
    },
    {
      key = "d",
      mods = "alt+shift",
      action = cervterm.action.SplitPane("columns"),
    },
    {
      key = "m",
      mods = "ctrl+shift",
      action = cervterm.action.Multiple({
        cervterm.action.FocusPane("left"),
        cervterm.action.ClosePane,
      }),
    },
  },
}
return config
