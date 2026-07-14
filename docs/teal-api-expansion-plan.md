# Teal/Lua API Expansion Plan

## Where we are

`term`: write, notify, size, cursor, title, line. Events: output, title,
bell. Keybindings with chords. Persistent single-thread runtime + shared
watchdog. Typed via docs/examples/cervterm.d.tl.

## Goal

Make CervTerm genuinely scriptable as a personal product: read what the
terminal knows, act on the surface the user sees, and react to more of its
lifecycle — without breaking the two invariants: **all Lua runs on the main
thread** and **the watchdog bounds every handler**.

Two stacked slices, sequential (both touch script/api.go + runtime.go).

---

## Slice A — Term surface (read + act)   [Codex]

New `Host` methods + Lua bindings (1-based indices at the boundary, same as
`line`; every method safe to call from any handler):

| Lua | Host method | Semantics |
|---|---|---|
| `term:selection(): string` | `Selection() string` | current selection text; "" when none (reuses termsel.Text path) |
| `term:copy(text: string)` | `SetClipboard(text string)` | write to OS clipboard (window.SetClipboardString; respects nothing else — user config is trusted) |
| `term:clipboard(): string` | `Clipboard() string` | read OS clipboard |
| `term:scroll(lines: integer): boolean` | `Scroll(lines int) bool` | viewport scroll; returns moved (core ScrollViewport already reports) |
| `term:scroll_to_bottom()` | `ScrollToBottom()` | offset 0 |
| `term:scrollback(): integer` | `ScrollbackLen() int` | history depth in rows |
| `term:set_title(t: string)` | `SetTitle(t string)` | overrides the window title (same path as OSC; dynamic_title still applies afterwards) |
| `term:line_wrapped(row: integer): boolean` | `LineWrapped(row int) (bool, bool)` | whether the row continues logically (for scripts reconstructing logical lines); out of range → false |

Frontend `App` implements the new Host methods (main-thread only — all
callers are key/event handlers already on the main thread). Scroll/title
mutations must `requestRedraw()` (or rely on existing damage triggers —
verify each: scroll → damage via displayOffset stateChanged only inside
draw, so explicit requestRedraw on moved, same as the wheel path).

Docs: cervterm.d.tl, docs/scripting.md (table + one example: a keybinding
that copies the current selection or the cursor line when empty).

Traps:
1. Host interface change breaks the fake host in script tests — update it.
2. `term:copy`/`term:clipboard` need `a.window` nil-guards (headless tests).
3. Selection text uses the SAME snapshot capture as copySelectionToClipboard
   — do not capture under a.mu while a handler already runs on the main
   thread mid-draw... verify reentrancy: handlers never run inside draw();
   processTermEvents/dispatch happen between frames. State why in a comment.
4. 1-based/0-based conversions at the Lua boundary only (match `line`).

Tests: runtime_test additions driving each binding against the fake host
(selection empty/nonempty, scroll clamp bool, wrapped out-of-range, title
roundtrip). Teal type-check test (teal_test.go pattern) still passes with
the extended .d.tl.

---

## Slice B — Lifecycle events + timers + zoom   [Opus]

### B1. New events

| Event | Fires when | Signature |
|---|---|---|
| `events.resize` | grid cols/rows changed (resizeToWindow / initial spawn) | `function(term, cols, rows)` |
| `events.focus` | window focus gained/lost (existing FocusCallback) | `function(term, focused: boolean)` |
| `events.scroll` | viewport offset changed (wheel or term:scroll) | `function(term, offset: integer)` |

Wire like FireTitle/FireBell: `Fire*` methods on Runtime with the shared
watchdog, dispatched from the main thread at the existing call sites (NOT
from inside draw()). Frequency guard: scroll events coalesce per loop
iteration (fire once with the final offset, not per wheel tick).

### B2. Timers (the big one — integrates with the on-demand wake loop)

```lua
cervterm.after(ms, function(term) ... end)   -- one-shot, returns id
cervterm.every(ms, function(term) ... end)   -- repeating, returns id
cervterm.cancel(id)
```

- Runtime keeps a min-deadline table of timers (main-thread only).
- The loop's `nextWakeTimeout` takes `min(current, runtime.NextTimerDeadline)`
  — a due timer bounds the sleep exactly like blink/notice do. Extend the
  pure `nextWake` helper with a `timerDeadline time.Time` argument (+ tests).
- Loop iteration calls `runtime.FireDueTimers(now, host)` after
  `processTermEvents` — handlers run on the main thread under the watchdog;
  a repeating timer reschedules from `now` (no drift accumulation
  requirement; document the semantics).
- A handler calling `cervterm.cancel` on its own id mid-fire must work.
- Timer handlers can call every term method (they're main-thread).
- No timers → zero cost (deadline zero → nextWake unchanged).

### B3. Runtime zoom

```lua
term:font_size(): number
term:set_font_size(pts: number)   -- clamped 6..72; rebuilds atlas + grid
```

Frontend: reuses the content-scale rebuild path (`rebuildForContentScale`
already rebuilds atlas + metrics); triggers resizeToWindow + PTY resize +
full damage reset. The classic use: ctrl+plus/minus keybindings in the
default template (commented example).

Traps:
1. Timer deadlines and the wake loop: a timer set from within an output
   handler must shorten an ALREADY computed wait — PostEmptyEvent isn't
   needed (handlers run between waits on the main thread; the next
   nextWakeTimeout computation sees the new deadline). State the reasoning
   in a comment; do not add cross-thread paths.
2. Watchdog: `every` handlers that overrun get killed like any handler —
   the timer stays scheduled; repeated failure spams notices → dedupe the
   notice (show once per timer id until it succeeds).
3. set_font_size during a handler: the rebuild touches GL — handlers run on
   the main thread with the GL context current; verify rebuild path is
   callable outside draw() (rebuildForContentScale already is, from the
   content-scale callback).
4. cervterm.d.tl grows `after/every/cancel` at module level — the module
   record shape changes; keep teal_test green.
5. `events.scroll` must not fire from inside draw (offset reads happen in
   prepareDamage) — fire from the wheel callback / term:scroll with the
   post-clamp offset.

Tests: pure nextWake+timer tests; runtime timer table tests (due ordering,
cancel, self-cancel, repeat reschedule); resize/focus/scroll Fire* tests
via fake host; zoom clamp test.

---

## Docs & examples (both slices)

- cervterm.d.tl stays the single typed source of truth.
- docs/scripting.md: new sections + a worked example combining them
  (status-clock via `every`, zoom keybindings, copy-selection binding).
- docs/examples/cervterm-keys-example.tl gains zoom bindings (commented).

## Verification

- Full gates + maturity each slice.
- E2E slice A: keybinding that copies selection → paste into the terminal;
  scroll from Lua moves the view (visible).
- E2E slice B: `every(1000)` clock in the title (set_title) ticks while the
  terminal is IDLE (proves timer↔wake integration); idle CPU stays ~0 with
  no timers; ctrl+plus/minus zoom live.

## Flow

Branch `feat/teal-api-a` off main → Codex implements → Fable review+E2E+PR.
Then `feat/teal-api-b` stacked → Opus implements → Fable review+E2E+PR.
Release v0.4.0-beta.1 after both (API expansion = minor bump).
