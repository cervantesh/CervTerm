# On-Demand (Damage-Driven) Rendering Plan

## Problem

The main loop in `internal/frontend/glfwgl/app.go` renders unconditionally:

```go
for !w.ShouldClose() {
    glfw.PollEvents()      // non-blocking
    a.drainIncoming()
    a.resizeToWindow()
    a.draw()               // full grid redraw, every iteration
    w.SwapBuffers()        // vsync=true blocks ~16ms; vsync=false spins
    a.meter.AddFrame()
}
```

Consequences:

- **Idle cost.** A completely idle terminal draws 60 fps forever (vsync on) or
  thousands of fps (vsync off): full snapshot capture, full grid walk, full GL
  submission, every frame. Alacritty/WezTerm draw ~0 frames when idle.
- **Battery/thermals.** Constant GPU submission and a never-sleeping main
  thread.
- **The FPS/mallocs HUD numbers are dominated by wasted frames**, not real work.

## Goal

Redraw **only when something visible changed**, and block the main thread in
the OS event wait otherwise. Idle CervTerm should draw ~2 fps with a blinking
cursor (one frame per phase flip), and ~0 fps with blink disabled and no HUD.

Frame-level damage only: when *anything* changed, redraw the *whole* frame.
Row-level/cell-level damage is explicitly out of scope (a possible later
slice; frame-level is what makes idle cost ~zero, which is the actual win).

## Design

### 1. Event-driven loop

Replace the poll loop with a wait loop:

```go
for !w.ShouldClose() {
    glfw.WaitEventsTimeout(a.nextWakeTimeout(time.Now()).Seconds())
    a.drainIncoming()          // sets needsRedraw if it consumed data
    a.resizeToWindow()         // sets needsRedraw if geometry changed
    if a.shouldRedraw(time.Now()) {
        a.draw()
        w.SwapBuffers()
        a.meter.AddFrame()
        a.needsRedraw = false
    }
}
```

- `WaitEventsTimeout` blocks on the OS event queue (main thread — required by
  GLFW, already guaranteed by `runtime.LockOSThread`). Any input event (key,
  char, mouse, resize, focus) wakes it naturally.
- `meter.AddFrame()` now counts **real** drawn frames, so the HUD FPS becomes
  an honest "how often am I actually drawing" number (idle ≈ blink rate).

### 2. Dirty flag + cross-thread wake

New `App` fields (main-thread only, no lock needed):

```go
needsRedraw bool
```

`func (a *App) requestRedraw() { a.needsRedraw = true }` — called from main
thread paths. The **PTY reader goroutine** cannot touch it; instead it calls
`glfw.PostEmptyEvent()` **after** each successful channel send.
`PostEmptyEvent` is one of the few GLFW functions documented safe from any
thread. The woken main loop runs `drainIncoming`, which observes the data and
sets the flag itself:

```go
// drainIncoming: if it consumed >=1 chunk → a.requestRedraw()
```

Ordering note for review: post-after-send means a send into a full channel
(main loop busy) blocks without a wake pending — this self-heals because the
wait always has a bounded timeout (see §3, max 500 ms), and in practice the
main loop is already awake when the channel is full. Do not use an unbounded
`glfw.WaitEvents()`.

### 3. Wake timeout: pure, testable helper

The only time-driven redraws are: cursor blink, transient notice expiry, and
the stats HUD refresh. Compute the next deadline in a **pure function** placed
in an untagged file (no glfw/gl imports) so the default `go test ./...` suite
covers it:

```go
// internal/frontend/glfwgl/wake.go  (NO build tag)

// nextWake returns how long the event wait may sleep before a time-driven
// redraw is due. Bounded to [minWake, maxWake] so a missed PostEmptyEvent
// self-heals and a tight blink never busy-loops.
func nextWake(now time.Time, blinkActive bool, blinkStart time.Time,
    blinkPeriod time.Duration, noticeUntil time.Time, statsShown bool) time.Duration
```

Rules:

- `blinkActive` → time until the next half-period boundary of
  `(now - blinkStart) % blinkPeriod`. `blinkActive` mirrors the existing
  `drawCursor` logic: config `cursor.blink` **or** a DECSCUSR blinking style
  (snapshot `CursorStyle` 1/3/5), and only while the cursor is visible and the
  viewport is not scrolled away.
- `noticeUntil` in the future → time until it (to clear the notice).
- `statsShown` → 500 ms (HUD numbers refresh; matches the FPS window).
- Otherwise → `maxWake` = **500 ms** (idle heartbeat; self-heal bound).
- Clamp result to `[1ms, 500ms]`.

An idle terminal with blink+HUD off therefore wakes 2×/s, checks
`shouldRedraw` (false), and goes back to sleep **without drawing**. Waking is
microseconds of CPU; drawing is what we're eliminating.

### 4. `shouldRedraw`

```go
func (a *App) shouldRedraw(now time.Time) bool {
    if a.needsRedraw { return true }
    if a.blinkActive() && blinkPhaseChanged(since last draw) { return true }
    if a.notice != "" && now.After(a.noticeUntil) { return true }  // clear it
    if a.showStats && now.Sub(a.lastStatsDraw) >= 500*time.Millisecond { return true }
    return false
}
```

Track `lastBlinkPhase bool` + `lastStatsDraw time.Time` on `App`, updated in
`draw()`.

### 5. Redraw triggers (audit of every mutation)

| Source | Where | Action |
|---|---|---|
| PTY output | reader goroutine → `drainIncoming` | `PostEmptyEvent` (reader); `requestRedraw` (drain) |
| Key/char input | GLFW callbacks | event already wakes the loop; echo comes back as PTY output. `requestRedraw()` **not** needed for the write itself |
| Local echo (no-PTY fallback `writeInput*`) | main thread | `requestRedraw()` after `parser.Advance` |
| Scroll wheel (viewport) | scroll callback | `requestRedraw()` |
| Selection begin/drag/release | mouse callbacks | `requestRedraw()` on any selection-state change |
| Resize / framebuffer change | `resizeToWindow` | `requestRedraw()` when cols/rows changed; also add `SetFramebufferSizeCallback` → `requestRedraw()` so pixel-size changes without col/row changes (padding remainder) still repaint |
| Content-scale change | existing callback | `requestRedraw()` after rebuild |
| `Notify` (script notices) | main thread | `requestRedraw()` |
| Stats HUD toggle | key callback | `requestRedraw()` |
| Script `term:write` | runs on main thread via dispatch | covered by PTY echo or local-echo path above |
| Bell/title events | `draw()` today | **move firing out of `draw()`** (see §6) |

### 6. Move event dispatch out of `draw()`

Today `draw()` fires title changes and bell events as a side effect of
capturing the snapshot. Under on-demand rendering `draw()` runs rarely, which
would delay bells/titles until the next repaint. Move the
capture-and-fire block (title compare, `lastBellCount` loop) from `draw()`
into a small `a.processTermEvents()` called every loop iteration after
`drainIncoming()` — cheap (one snapshot capture) but only when
`drainIncoming` consumed data. `draw()` then only renders the already-captured
snapshot. This also removes a hidden ordering dependency (events fired only
when drawing) that was always a latent wart.

Snapshot capture cost note: capture currently happens once per drawn frame.
After the change it happens once per *data* batch, which under heavy output is
the same or less than today.

### 7. Config

- `render.vsync` keeps its meaning (swap interval) and its `true` default.
  With on-demand rendering, vsync=false no longer spins — it just means
  "swap immediately when we do draw".
- New `render.redraw = "on_demand" | "continuous"`, default `"on_demand"`.
  `"continuous"` preserves today's unconditional loop for benchmarking
  (vsync=false + continuous ≈ current uncapped mode) and as an escape hatch if
  a damage bug ever ships. Validate the enum in `config.Validate`; template
  comment explains both.

### 8. Files touched

| File | Change |
|---|---|
| `internal/frontend/glfwgl/app.go` | loop rewrite; `needsRedraw`/`lastBlinkPhase`/`lastStatsDraw` fields; callback `requestRedraw` calls; reader `PostEmptyEvent`. **app.go is at 497 lines** — extract the loop + `shouldRedraw` + `processTermEvents` into a new `app_loop.go` (`//go:build glfw`) to stay under the 500-line gate |
| `internal/frontend/glfwgl/wake.go` | new, untagged: `nextWake` pure helper |
| `internal/frontend/glfwgl/wake_test.go` | new, untagged: table tests (blink boundary math incl. mid-phase, notice sooner than blink, stats cap, clamps, zero-period guard) |
| `internal/frontend/glfwgl/app_draw.go` | remove title/bell firing from `draw()`; record `lastBlinkPhase`/`lastStatsDraw` |
| `internal/config/config.go` | `RenderConfig.Redraw string` + default + validation |
| `internal/config/lua.go` | `redraw` field |
| `internal/config/template.go` | `redraw` line + comment |
| `README.md` | one line on on-demand rendering + `render.redraw` |

### 9. Correctness traps (for implementation + pair review)

1. `WaitEventsTimeout`/`PollEvents` **main thread only**; `PostEmptyEvent` is
   the only wake API other threads may use.
2. Never use unbounded `WaitEvents()` — the 500 ms cap is the self-heal for
   any lost wake.
3. `needsRedraw` is main-thread state; the reader goroutine must not set it.
4. Blink boundary math must not drift: compute the phase from
   `(now - blinkStart) % period` (as `cursorBlinkPhase` does), don't
   accumulate deltas.
5. During interactive window resize/drag Windows runs a modal loop and
   `WaitEventsTimeout` won't return until it ends — same as today's behavior
   with `PollEvents`; do not attempt draw-from-callback in this slice.
6. `drainIncoming` must keep firing `on_output` outside `a.mu` (existing
   deadlock comment) — the restructure must not move it inside the lock.
7. `processTermEvents` fires scripts on the main thread like today; keep the
   monotonic `lastBellCount` catch-up loop.
8. Don't break the no-PTY fallback path (parser fed directly): local typing
   must still repaint via the local-echo `requestRedraw`.
9. `continuous` mode must reproduce today's loop exactly (Poll + always draw)
   so benchmarks stay comparable.

### 10. Verification (E2E, on Windows)

1. **Idle CPU**: launch, wait 10 s, sample process CPU time delta over 10 s
   (`Get-Process ... | Select CPU` twice). Expect near-zero (<1% core) with
   blink on; compare against `redraw = "continuous"` baseline.
2. **HUD**: with blink on and HUD open, FPS should read ≈ stats-refresh rate
   (~2), not 60. `mallocs` growth while idle ≈ flat.
3. **Interactivity**: typing latency unchanged (`echo hola`, arrow keys in a
   TUI); run `type` on a large file — smooth continuous output (drain batches
   still coalesce).
4. **Blink**: cursor visibly blinks at the configured interval while idle.
5. **Notice**: `term:notify` from a keybinding appears and disappears on
   schedule (~2.5 s) without input.
6. **Selection**: click-drag highlights live while the mouse moves.
7. **Resize**: drag-resize reflows; cols/rows update in HUD.
8. **Bell/title**: `printf '\a'` fires `on_bell` promptly while idle;
   `printf '\033]0;t\007'` retitles promptly (both without pressing a key).
9. Full gates: `go build ./...`, `go build -tags glfw ./...`, `go test ./...`,
   `go vet`, maturity gates.

### 11. Branch / flow

Stacked slice: `feat/on-demand-render` on top of `feat/clean-chrome`
(PR #22), PR based on `feat/clean-chrome`. Same flow as previous slices:
Codex (gpt-5.6-sol) implements this plan solo → Opus pair review (focus:
wake/dirty races, blink math, event-dispatch move) → Fable E2E verify + PR.
