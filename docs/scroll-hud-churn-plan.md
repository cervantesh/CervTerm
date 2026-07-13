# Fix: clamped scroll and per-frame HUD strings churn allocations

## Symptoms (user-observed via the HUD)

Wheel-scrolling at the bottom boundary (where the viewport cannot move)
makes `mallocs` climb fast. Heap rises and is collected (churn, not a leak).

## Causes

1. **Clamped scroll still redraws.** `ScrollViewport` (core/terminal.go:467)
   clamps silently; the scroll callback (glfwgl/app.go:341) cannot tell that
   nothing changed, so every wheel tick at the boundary produces damage →
   full frame (displayOffset unchanged means row-mode, but the HUD was
   visible = full-frame) → wasted frames at 60fps.
2. **HUD strings rebuild every drawn frame.** `drawHUD` composes its two
   rows with `fmt.Sprintf` (+ slices) on each draw. While anything animates
   at 60fps with the HUD open, the HUD itself generates most of the garbage
   it is measuring.

## Fix

1. `ScrollViewport(lines int) bool` — return whether `displayOffset`
   actually changed. The scroll callback only requests a redraw (implicitly
   via damage/needsRedraw) when it did:
   ```go
   a.mu.Lock()
   moved := a.term.ScrollViewport(rows)
   a.mu.Unlock()
   if moved { a.requestRedraw() }
   ```
   Audit: the callback today relies on the wheel EVENT waking the loop and
   the offset change being caught by damage state — after this change a
   clamped tick wakes the loop, drains nothing, damages nothing, draws
   nothing. Also update any other ScrollViewport callers (grep; scripting
   API `term:scroll` if present) to ignore or use the bool.
2. Cache HUD row strings on `App` (`hudLines []string`, `hudColors`),
   rebuilt only inside the existing 500ms stats window (`lastStats` /
   `lastStatsDraw` timing) or when `a.notice` changes; `drawHUD` renders the
   cached strings. The FPS/rows numbers already only need 500ms freshness —
   this is the same cadence the meter snapshot uses.

## Traps

1. `ScrollViewport` signature change: fix ALL callers (core tests included)
   or keep a void wrapper for compatibility — prefer fixing callers, the
   API is internal.
2. The scroll callback must still request a redraw when `moved` — today the
   damage system catches displayOffset changes via `prepareDamage`
   stateChanged, which only runs inside draw(); the draw only happens if
   needsRedraw was set by SOMETHING. Verify the current trigger chain and
   keep it airtight: moved → requestRedraw explicitly.
3. HUD cache invalidation: notice text change, stats toggle, cols/rows
   change, fps window rollover — rebuild on `lastStatsDraw` update (already
   per 500ms) AND when notice content differs from what was cached.
4. Do not cache across `showStats` toggles in a way that flashes stale
   numbers: toggling on forces an immediate rebuild.

## Tests

- Core: `ScrollViewport` returns false at both clamps, true on real moves
  (table test).
- Frontend: none practical beyond gates (GL-bound), rely on E2E.

## Verify E2E

1. HUD open, wheel-scroll hard at the bottom boundary 10s: `mallocs` delta
   near zero (vs thousands before), fps drops to ~stats cadence because no
   frames are drawn.
2. Real scrolling up/down still repaints live.
3. Notice appearance/expiry still renders correctly with cached HUD lines.

## Flow

Branch `fix/scroll-hud-churn` off main AFTER fix/resize-blank-scrollback
lands (both touch internal/core/terminal.go). Implementer: Opus. Reviewer:
Fable + E2E + PR.
