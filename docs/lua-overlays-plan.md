# Lua Overlay/Canvas API Plan (`cervterm.overlay`)

## Goal

Let scripts draw persistent, cell-addressed overlays on top of the terminal —
the step from configurable to programmable (Kitty-kittens model). Examples this
unlocks: git margin indicators, scrollback minimap, floating clocks/dashboards,
error-line highlighting driven by `events.output`.

## Core decision: retained scene, not immediate mode

A per-frame draw callback would break both architecture invariants: Lua never
runs inside `draw()`, and an idle terminal draws ~0 fps. Instead, scripts
mutate a **retained display list** from where Lua already runs (keybindings,
events, timers); CervTerm renders it as part of its frames. Unchanged scene =
zero cost — the same seq→damage pattern already proven by status segments.

## API

```lua
local cervterm = require("cervterm")
local ov = cervterm.overlay("git-panel")   -- create/get by id

ov:clear()
ov:rect(60, 1, 18, 4, "#10141CF0")         -- col, row, w, h in CELLS, #RRGGBB[AA]
ov:text(61, 2, "rama: main", "#60E8F0")    -- single line, fg color
ov:hline(61, 4, 16, "#2A6377")             -- and vline(col, row, h, color)
ov:commit()                                 -- atomic swap; nothing shows half-built

ov:show()  ov:hide()  ov:destroy()
```

- **Cell coordinates, 1-based** (matching `term:line`); they survive zoom and
  resize; out-of-grid primitives are clipped, never errors.
- **Primitives v1**: `rect` (alpha fill), `text`, `hline`, `vline`. No pixel
  paths, no curves.
- **Z-order**: terminal cells → cursor → overlays (creation order) → system UI
  (notices, search bar, stats HUD) — system UI always wins.
- **Budget**: max 512 primitives per overlay; exceeding = script-error notice,
  primitives beyond the cap dropped.
- **commit() atomicity**: old scene renders until the new one is complete.

## Implementation shape

- **script layer** (internal/script, new overlays.go + tests): main-thread
  overlay store keyed by id (mirror timers.go/status.go patterns) — each
  overlay holds two display lists (committed, building). Module function
  `cervterm.overlay(id)` returns a userdata/table with the methods above.
  Runtime exposes `OverlaySeq() int` (bumps on commit/show/hide/destroy) and
  `Overlays() []OverlayScene` (committed lists, creation-ordered).
- **frontend** (internal/frontend/glfwgl, new app_overlay.go): sync like
  syncStatusSegments (after handlers/timers, before shouldRedraw →
  requestRedraw on seq change); render pass after cursor, before notices;
  cell→pixel via paddingX/Y + cellW/H; color parse shared with configColor
  (extended for AA).
- **damage**: overlay seq + per-overlay covered-row ranges in damage state;
  visible overlays' rows marked damaged every frame (status-band pattern);
  seq change forces full redraw that frame only.

## Review traps

1. Damage: covered rows always damaged while visible; commit/hide/destroy
   forces full redraw that frame (rows beneath must repaint); never pins
   full-frame permanently.
2. Lua never runs inside draw(); scene render is pure Go.
3. `text` with wide/emoji runes: span via the grid's real cell measurement.
4. Invalid colors/coords: drop the primitive with one deduped notice — never
   break the scene or crash.
5. Budget enforcement can't allocate per frame (enforce at build time, not
   render time).
6. File-size gates (<500 lines/file); watchdog covers scene-building handlers.

## Tests

- script layer: build/commit atomicity (uncommitted invisible), seq bumps on
  commit/show/hide/destroy only, budget cap, destroy removes, per-id identity.
- pure: clip math, color parsing with AA, covered-rows computation.
- E2E: fake git panel + floating clock via every(1000) ticking IDLE at ~0%
  CPU; hide/destroy repaints rows beneath; zoom/resize keeps cell alignment.

## Flow

Branch `feat/lua-overlays` off main. Implementer: **Opus** (touches the draw
path; biggest slice of the series). Reviewer: Fable (traps checklist) + E2E +
PR. Docs: cervterm.d.tl types, scripting.md section with the git-panel
example. Release after merge.
