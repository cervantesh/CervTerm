# CervTerm scripting

CervTerm can load `cervterm.lua` or `cervterm.tl` as both configuration and a
small extension runtime. The supported extension points are user keybindings and
terminal event handlers that run Lua functions.

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

## Events

Add an `events` table to react to terminal activity. Every handler is optional
and receives the `term` handle first:

- `output(term, data)`: fires for each chunk of program output, with the raw
  bytes as a string. This runs on every output chunk, so keep it fast — a slow
  handler throttles rendering.
- `title(term, title)`: fires when the program changes the window title (OSC 0/2).
- `bell(term)`: fires when a `BEL` control executes.

```lua
return {
  events = {
    output = function(term, data)
      if data:find("error") then term:notify("saw an error") end
    end,
    title = function(term, title) term:notify("title: " .. title) end,
    bell = function(term) term:notify("ding") end,
  },
}
```

Handlers share the keybinding watchdog: an erroring or timed-out handler surfaces
a `script error:` notice and does not stop the runtime.

## Term API

All row and column numbers at the Lua boundary are 1-based.

| Method | Result or effect |
|---|---|
| `term:write(s)` | Writes bytes to the PTY. In fallback renderer mode, feeds the same bytes to the terminal parser. |
| `term:notify(s)` | Shows a transient notice in the status line area for about four seconds. |
| `term:selection()` | Returns the current selected text, or `""` when there is no selection. |
| `term:copy(s)` | Writes `s` to the OS clipboard. |
| `term:clipboard()` | Returns text from the OS clipboard. |
| `term:scroll(lines)` | Scrolls the viewport into history for positive values and toward the bottom for negative values; returns whether it moved. |
| `term:scroll_to_bottom()` | Returns the viewport to the live bottom. |
| `term:scrollback()` | Returns the number of history rows. |
| `term:size()` | Returns `cols, rows`. |
| `term:cursor()` | Returns the cursor `row, col`. |
| `term:title()` | Returns the current terminal title. |
| `term:set_title(s)` | Sets the terminal title. A later OSC 0/2 title from the running program may replace it. |
| `term:line(n)` | Returns visible row `n` with trailing blanks trimmed, or `""` when out of range. |
| `term:line_wrapped(n)` | Returns whether visible row `n` wraps into the next row; returns `false` when out of range. |

`write`, `notify`, `copy`, and `set_title` require string arguments. This
keybinding copies the current selection, falling back to the cursor line when
the selection is empty:

```lua
{
  key = "c",
  mods = "ctrl+shift",
  action = function(term)
    local text = term:selection()
    if text == "" then
      local row = select(1, term:cursor())
      text = term:line(row)
    end
    term:copy(text)
    term:notify("copied " .. #text .. " characters")
  end,
}
```

```lua
action = function(term)
  local _, rows = term:size()
  local row = select(1, term:cursor())
  term:notify("row " .. row .. "/" .. rows .. ": " .. term:line(row))
end
```

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
stored in `keys` and `events` remain alive in that state and are dispatched from
the GLFW main loop thread. The runtime is single-owner and must not be called
from other goroutines.

Each action and event handler has a watchdog timeout. An erroring or timed-out
call does not stop the runtime; later keybindings and events can still run.

## Non-goals

User config is the user's own file and is not sandboxed. Filesystem and network
permission controls are future work for third-party plugin support.

Event handlers observe only: an `output` handler cannot rewrite or suppress the
bytes shown. This slice does not add hot reload, command palettes, overlays, key
repeat dispatch, multiple handlers per event, or multi-chord sequences.
