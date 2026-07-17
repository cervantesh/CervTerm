local cervterm = require("cervterm")

local config = {
  config_version = 2,
  keys = {
    {
      key = "p",
      mods = "ctrl+shift",
      action = function(term: cervterm.Term)
        term:write("echo hola desde teal\r")
        term:notify("saludo enviado")
      end,
    },
    -- Runtime zoom: ctrl+= grows the font, ctrl+- shrinks it (clamped 6..72).
    -- {
    --   key = "equal",
    --   mods = "ctrl",
    --   action = function(term: cervterm.Term)
    --     term:set_font_size(term:font_size() + 1)
    --   end,
    -- },
    -- {
    --   key = "minus",
    --   mods = "ctrl",
    --   action = function(term: cervterm.Term)
    --     term:set_font_size(term:font_size() - 1)
    --   end,
    -- },
  },
}
return config
