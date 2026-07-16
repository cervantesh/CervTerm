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
- `cwd(term, dir)`: fires when the program reports a new working directory with
  OSC 7. Invalid or empty OSC 7 payloads are ignored.
- `bell(term)`: fires when a `BEL` control executes.
- `resize(term, cols, rows)`: fires when the terminal grid dimensions change,
  including the initial size and any `term:set_font_size` rebuild.
- `focus(term, focused)`: fires when the window gains (`true`) or loses (`false`)
  focus.
- `scroll(term, offset)`: fires when the viewport scroll offset changes, with the
  post-clamp offset (rows above the live bottom). Wheel bursts are coalesced, so
  the handler fires once per loop iteration with the final offset, not once per
  tick, and it never fires from inside a frame draw.

With native panes, `output`, `title`, `cwd`, `bell`, `resize`, and `scroll` receive a `term` handle permanently bound to the pane that produced the event for that callback. Reads, writes, search, scrolling and title changes through that handle cannot jump to a focused sibling. Keybindings, timers, and window `focus` events use the pane focused when dispatch begins. Background pane title changes still fire `title`, but only the focused pane controls the OS window title.

```lua
return {
  events = {
    output = function(term, data)
      if data:find("error") then term:notify("saw an error") end
    end,
    title = function(term, title) term:notify("title: " .. title) end,
    cwd = function(term, dir) term:notify("cwd: " .. dir) end,
    bell = function(term) term:notify("ding") end,
    resize = function(term, cols, rows) term:notify(cols .. "x" .. rows) end,
    focus = function(term, focused) term:notify(focused and "focused" or "blurred") end,
    scroll = function(term, offset) term:notify("scroll " .. offset) end,
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
| `term:cwd()` | Returns the last working directory reported by OSC 7, or `""` before one is received. |
| `term:set_title(s)` | Sets the terminal title. A later OSC 0/2 title from the running program may replace it. |
| `term:line(n)` | Returns visible row `n` with trailing blanks trimmed, or `""` when out of range. |
| `term:line_wrapped(n)` | Returns whether visible row `n` wraps into the next row; returns `false` when out of range. |
| `term:font_size()` | Returns the active font size in points. |
| `term:set_font_size(pts)` | Sets the font size (clamped to 6..72), rebuilding the glyph atlas and reflowing the grid. |
| `term:search(query)` | Jumps to the first (bottom-most) case-insensitive match for `query` across scrollback and the live screen, scrolls it into view and highlights it; returns whether a match was found. Non-interactive counterpart to the search bar. An empty query is a no-op and returns `false`. |

### Interactive search

Press `ctrl+shift+f` to open the scrollback search bar (a bottom overlay). Type a
query, then press Enter to jump to the next match upward (the first jump starts
from the bottom of the live screen); the match row scrolls into view and is
highlighted. Backspace edits the query and Escape closes the bar and returns to
the terminal. While the bar is open, no keystrokes reach the shell. The hotkey is
a fixed `ctrl+shift+f` chord in v1 (configurable in a later release). Search is a
plain, case-insensitive substring match within a single physical row; a match
that straddles a wrapped-line boundary is not found in v1.

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

## Timers

The `cervterm` module (the same one that provides Teal types) exposes three
module-level timer functions. Require it once at the top of your config:

```lua
local cervterm = require("cervterm")
```

- `cervterm.after(ms, fn)`: runs `fn(term)` once after `ms` milliseconds and
  returns a timer id. It removes itself after firing.
- `cervterm.every(ms, fn)`: runs `fn(term)` every `ms` milliseconds and returns a
  timer id. Each tick reschedules from the moment it fired (deadlines do not
  accumulate drift).
- `cervterm.cancel(id)`: cancels the timer with that id. A repeating handler may
  cancel its own id from inside the callback.

Timers run on the main loop thread under the same watchdog as keybindings and
events. They integrate with the on-demand render loop: a scheduled timer bounds
the event wait exactly like the cursor blink does, so a `cervterm.every` clock
keeps ticking while the terminal is idle without any busy polling — and with no
timers scheduled, an idle terminal still draws ~0 fps. If an `every` handler
errors or times out repeatedly, its `script error:` notice is shown once (not
once per tick) until the handler next succeeds.

This status clock rewrites the window title every second, even when nothing else
is happening:

```lua
local cervterm = require("cervterm")

cervterm.every(1000, function(term)
  term:set_title(os.date("%H:%M:%S"))
end)

return {
  font = { family = "Go Mono", size = 14 },
}
```

## Status segments

`cervterm.status(id, text)` sets a persistent status segment in the top-right
status band. Segments remain in first-registration order and are joined with
` · `. Setting an existing id replaces its text without moving it; setting its
text to an empty string removes it. The band is hidden when no segments remain
and long content is truncated to the window width with a leading `…`.

Combine status segments with timers for information that updates while the
terminal is otherwise idle. This example adds a clock and removes it later:

```lua
local cervterm = require("cervterm")

cervterm.every(1000, function(term)
  cervterm.status("clock", os.date("%H:%M:%S"))
end)

cervterm.after(60000, function(term)
  cervterm.status("clock", "")
end)

return {}
```

## Overlays

`cervterm.overlay(id)` returns a retained, cell-addressed display list drawn on
top of the terminal. Scripts build primitives into a pending list and publish
them atomically with `commit`; the render pass is pure Go over the committed
scene, so an idle overlay costs nothing and Lua never runs inside a frame.

Handle methods:

- `ov:rect(col, row, w, h, color)`: filled rectangle spanning `w`×`h` cells.
- `ov:text(col, row, s, color)`: single line; wide/emoji runes span the same
  cells they would as terminal text.
- `ov:hline(col, row, w, color)`: thin horizontal rule along the top of the row,
  spanning `w` cells.
- `ov:vline(col, row, h, color)`: thin vertical rule along the left of the
  column, spanning `h` cells.
- `ov:clear()`: empty the pending list.
- `ov:commit()`: atomically swap the pending list into view — nothing shows
  half-built.
- `ov:show()` / `ov:hide()`: toggle visibility without rebuilding.
- `ov:destroy()`: remove the overlay and repaint the cells beneath it.

Coordinates are 1-based cells, so overlays survive zoom and resize. Colors are
`#RRGGBB` or `#RRGGBBAA` (alpha last; omitted means opaque). Primitives outside
the grid are clipped silently; an invalid color or coordinate drops that
primitive with a single notice and never breaks the scene. Each overlay holds up
to 512 primitives.

This example paints a small git panel and refreshes it every second:

```lua
local cervterm = require("cervterm")

local panel = cervterm.overlay("git-panel")

cervterm.every(1000, function(term)
  local cols = term:size()
  local left = cols - 19

  panel:clear()
  panel:rect(left, 1, 20, 4, "#10141CF0")     -- translucent card
  panel:hline(left, 1, 20, "#2A6377")         -- top accent rule
  panel:text(left + 1, 2, "rama: main", "#60E8F0")
  panel:text(left + 1, 3, "+12 -3", "#8AE234")
  panel:commit()
end)

return {}
```

Toggle it from a keybinding:

```lua
local cervterm = require("cervterm")

local shown = true
return {
  keys = {
    { key = "g", mods = "ctrl+shift", action = function()
        local panel = cervterm.overlay("git-panel")
        shown = not shown
        if shown then panel:show() else panel:hide() end
      end },
  },
}
```

## Zoom

`term:set_font_size` rebuilds the glyph atlas and reflows the grid live, so it is
the building block for zoom keybindings. Font sizes are clamped to 6..72.

```lua
return {
  keys = {
    { key = "equal", mods = "ctrl", action = function(term)
        term:set_font_size(term:font_size() + 1)
      end },
    { key = "minus", mods = "ctrl", action = function(term)
        term:set_font_size(term:font_size() - 1)
      end },
  },
}
```

## Working directory tracking

CervTerm learns the shell's current directory from OSC 7. Add this to your
PowerShell profile (`$PROFILE`):

```powershell
function prompt {
  $location = $ExecutionContext.SessionState.Path.CurrentLocation
  $uri = [Uri]::new($location.Path).AbsoluteUri
  Write-Host "`e]7;$uri`a" -NoNewline
  "PS $location> "
}
```

`[Uri]::new(...).AbsoluteUri` produces a real `file:///C:/...` URI and
percent-encodes spaces and UTF-8 path characters. For bash, add this to
`~/.bashrc`:

```bash
__cervterm_uri_path() {
  local LC_ALL=C input=$1 output= char hex i
  for ((i = 0; i < ${#input}; i++)); do
    char=${input:i:1}
    case $char in
      [a-zA-Z0-9._~/:+-]) output+=$char ;;
      *) printf -v hex '%%%02X' "'$char"; output+=$hex ;;
    esac
  done
  printf '%s' "$output"
}

__cervterm_osc7() {
  printf '\033]7;file://%s\007' "$(__cervterm_uri_path "$PWD")"
}
PROMPT_COMMAND=__cervterm_osc7
```

If `PROMPT_COMMAND` already contains hooks, compose `__cervterm_osc7` with them
instead of replacing them. The byte-oriented encoder percent-escapes spaces,
reserved characters, and UTF-8 path bytes before emitting the OSC sequence.

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
bytes shown. This slice does not add hot reload, command palettes, key repeat
dispatch, multiple handlers per event, or multi-chord sequences.
