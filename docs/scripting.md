# CervTerm scripting

CervTerm can load `cervterm.lua` or `cervterm.tl` as both configuration and a
small extension runtime. The first supported extension point is user keybindings
that run Lua functions.

## Keybindings

Add a `keys` array to the returned config table. Each entry has:

- `key`: required string. Supported names are `a` through `z`, `0` through `9`,
  `f1` through `f12`, `enter`, `tab`, `escape`, `space`, `backspace`, `delete`,
  `insert`, `home`, `end`, `pageup`, `pagedown`, `up`, `down`, `left`, `right`,
  `minus`, `equal`, `comma`, `period`, `slash`, `backslash`, `semicolon`,
  `apostrophe`, and `grave`.
- `mods`: optional `+`-separated string. Supported modifiers are `ctrl`, `alt`,
  `shift`, and `super`. `cmd` and `win` are aliases for `super`.
- `action`: required function. CervTerm calls it with one `term` handle.

User keybindings run before built-in shortcuts. If a user binding matches, the
key is consumed. If the action fails, CervTerm shows a transient `script error:`
notice in the status area and still consumes the key.

```lua
return {
  font = { family = "Go Mono", size = 14 },
  keys = {
    {
      key = "p",
      mods = "ctrl+shift",
      action = function(term)
        term:write("echo hola desde lua\r")
        term:notify("saludo enviado")
      end,
    },
  },
}
```

## Term API

`term:write(s)` writes bytes to the PTY. In fallback renderer mode, it feeds the
same bytes to the terminal parser.

`term:notify(s)` shows a transient notice in the status line area for about four
seconds.

Both methods require strings.

## Teal

Teal configs are checked and generated before CervTerm loads them. Copy
`docs/examples/cervterm.d.tl` next to your `cervterm.tl` so
`require("cervterm")` has type definitions.

```lua
local cervterm = require("cervterm")

local config = {
  keys = {
    {
      key = "p",
      mods = "ctrl+shift",
      action = function(term: cervterm.Term)
        term:write("echo hola desde teal\r")
        term:notify("saludo enviado")
      end,
    },
  },
}
return config
```

## Runtime model

CervTerm executes the config file once in a persistent Lua state. Lua functions
stored in `keys` remain alive in that state and are dispatched from the GLFW main
loop thread. The runtime is single-owner and must not be called from other
goroutines.

Each action has a watchdog timeout. An erroring or timed-out action does not stop
the runtime; later keybindings can still run.

## Non-goals

User config is the user's own file and is not sandboxed. Filesystem and network
permission controls are future work for third-party plugin support.

This slice does not add output events, hot reload, command palettes, overlays,
key repeat dispatch, or multi-chord sequences.
