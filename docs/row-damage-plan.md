# Row-Level Damage Plan

## Honest framing

On-demand rendering already made idle cost ~0. Row damage only helps the
frames we *do* draw: while typing, one keystroke currently redraws the whole
grid (~cols×rows immediate-mode GL calls); cursor blink redraws everything to
toggle one cell. This slice cuts those frames to the changed rows. The win is
input latency and per-frame CPU during localized updates — modest but real.
Full-output streaming (scrolling) damages every row and stays full-frame.

## The double-buffer problem (core design constraint)

`SwapBuffers` on WGL gives **no buffer-age guarantee**: after a swap the back
buffer formally holds undefined content (in practice: the frame from 2 swaps
ago on exchange-mode drivers, or the previous frame on copy-mode). Partial
redraw is only correct if every pixel we *don't* redraw already holds correct
content in the back buffer.

Design: **2-frame damage union with full-frame fallback**.

- Track per-row content hashes of the current capture (`fnv1a` over the row's
  cells — attrs included) in `App.rowHashes`, plus the hash sets of the last
  two *drawn* frames (`prevHashes`, `prevPrevHashes`).
- A row needs repaint if its hash differs from EITHER of the last two drawn
  frames (buffer-age-2 assumption).
- On ANY global change → full-frame redraw and reset both history sets:
  resize, content-scale, scroll (viewport or alt-screen switch), selection
  active or changed, BiDi enabled, HUD visible, notice visible, background
  color change. Cursor move only damages old+new cursor rows.
- **Empirical safety gate**: WGL swap mode is driver-dependent. First frame
  after startup and any frame where `prevPrevHashes` is empty → full redraw.
  Config `render.damage = "rows" | "frame"`, default **"rows"**; "frame"
  is the escape hatch. README notes the flag if artifacts ever appear.

## Mechanics

- `draw()` gains a damage prologue: compute row hashes from the fresh
  snapshot (cheap: ~cols×rows byte hashing, no alloc — reuse buffers);
  decide `fullRedraw bool` per the fallback list; otherwise build the
  damaged-row set (hash mismatch vs either history + cursor rows old/new).
- Clear: full redraw keeps `glClear`. Row mode skips `glClear` and instead
  fills each damaged row's rect with the background color before drawing its
  cells (`fillRect(0, y, w, cellH, background)` — full window width including
  padding margins of that strip).
- Cursor: always treat old and new cursor rows as damaged (blink toggles,
  moves).
- The existing per-row draw body is already row-scoped — extract it as
  `drawRow(r int, ...)` and loop over damaged rows only. No GL state changes
  beyond what each row already sets.
- History update after a drawn frame: `prevPrev, prev = prev, current`
  (swap the backing arrays, no alloc).
- Hashing lives in `internal/render/damage.go` (pure, no GL):
  `HashRows(dst []uint64, cells []core.Cell, cols int)` + unit tests
  (change one cell → that row's hash changes; attr-only change counts;
  identical rows equal).

## Files

| File | Change |
|---|---|
| `internal/render/damage.go` (+test) | new: pure row hashing |
| `internal/frontend/glfwgl/app_draw.go` | damage prologue, drawRow extraction, per-row background fill, history bookkeeping |
| `internal/frontend/glfwgl/app.go` | fields: rowHashes/prevHashes/prevPrevHashes, lastCursorRow |
| `internal/config/*` | `render.damage` enum + validation + template |
| `README.md` | one line |

Size gates: app_draw.go is at ~351 lines; the extraction may push it over 500
if combined with damage logic — if so, move `drawRow` into `app_row.go`
(`//go:build glfw`).

## Correctness traps

1. Buffer-age: never trust the back buffer beyond 2 swaps — the union must
   cover BOTH previous drawn frames, and any skipped-draw iteration must NOT
   rotate history (history describes *drawn* frames, not loop iterations).
2. Selection renders row-spanning state → any active selection forces full
   frame (v1; per-row selection damage is not worth the risk).
3. HUD/notice overlays draw over arbitrary rows → visible overlay forces full
   frame.
4. Padding strips (left/right margins) belong to no row; on row mode they are
   never repainted — correct only because they are permanent background, but
   a background color change must force full redraw (it's in the fallback
   list).
5. Wide glyphs/BiDi reordering stay inside one row — row granularity is safe;
   but `render.bidi = true` forces full frame in v1 (VisualOrder cost is
   per-row anyway).
6. FPS meter semantics unchanged (frame = swap, regardless of rows drawn).

## E2E verify

1. Type characters into an idle shell: no visual artifacts (compare
   screenshots), only cursor row redrawn (instrument via a debug counter in
   the HUD: "rows N/M").
2. Cursor blink while idle: damage = 1 row per phase flip.
3. `type` a large file: automatic full-frame behavior (all rows damaged),
   throughput within noise of v0.3.1 (~1s for 20k lines).
4. Selection drag, scroll, resize, HUD toggle: full redraw, no artifacts.
5. `render.damage = "frame"` reproduces today's behavior.
6. Soak: 5 minutes of mixed typing/scrolling, zero visual corruption.

## Flow

Branch `feat/row-damage` off main. Implementer: **Codex (gpt-5.6-sol)**.
Reviewer: **Fable** (this plan's traps as checklist) + E2E + PR.
