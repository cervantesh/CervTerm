# Fix: shrinking the terminal pushes blank rows into scrollback

## Symptom

After the PTY-initial-size fix, a fresh launch still shows blank lines above
the banner when scrolling up. Instrumentation confirms no ConPTY resize
happens anymore; the blank rows come from the terminal core.

## Cause

`core.Terminal.Resize` builds `physicalRows()` (scrollback + ALL live rows,
trailing blanks included) and keeps the bottom `rows` visible:
`visibleStart = max(0, len(physicalRows)-rows)` — everything above becomes
scrollback. Shrinking the startup terminal (constructed 100x32, resized to
the real ~77x23 before the PTY spawns) therefore pushes 9 all-blank rows
into history. The same happens whenever a user shrinks a window whose bottom
rows are blank: phantom blank history.

## Fix (standard xterm/WezTerm behavior)

On shrink, drop trailing blank rows before scrolling anything into history.
In `Resize` (internal/core/terminal.go), after obtaining/reflowing
`physicalRows, wrappedRows` and before computing `visibleStart`:

```go
// Trailing all-blank rows below the cursor are dropped rather than letting
// them force content into scrollback; only real content scrolls to history.
keep := oldCursorGlobal + 1           // never trim the cursor's row or above
for len(physicalRows) > keep && isBlankRow(physicalRows[len(physicalRows)-1]) {
    physicalRows = physicalRows[:len(physicalRows)-1]
    wrappedRows = wrappedRows[:len(wrappedRows)-1]
}
```

- `isBlankRow`: every cell equals the blank cell (rune ' ' or 0, default
  FG/BG, no attrs) — put it next to `blank()` in screen.go.
- `oldCursorGlobal` is already computed (`scrollbackRows + cursorRow`). For
  the `cols != oldCols` reflow path the pre-reflow global row is an
  approximation, but blank rows reflow 1:1 (a blank logical row stays one
  physical row), so clamping `keep` to `len(physicalRows)` keeps it safe.
- The trim applies on ANY resize (grow included) — trailing blank rows above
  history are never worth preserving; `rebuildFromPhysicalRows` re-pads the
  live screen to `rows`.
- Cursor mapping: the existing `cursorRow` recompute uses `visibleStart`,
  which after the trim is smaller — the cursor can only move UP or stay,
  never clip below, because `keep` protects its row.

## Traps

1. Never trim at or above the cursor's row — a prompt at the bottom must not
   be eaten.
2. Trim BEFORE `visibleStart` is computed, and keep `wrappedRows` in sync
   (same pops).
3. Wrapped continuation rows: a blank row whose `wrapped` flag is true is
   part of a logical line — do NOT trim it (guard: stop at the first
   trailing row that is blank but wrapped).
4. Alt screen: `Resize` handles the primary grid here; verify the alt-screen
   resize path (if separate) is unaffected — alt screen has no scrollback.
5. `DisplayOffset` clamping after shrink already goes through
   `rebuildFromPhysicalRows(..., oldDisplayOffset)` — unchanged.

## Tests (internal/core)

1. Fresh empty terminal 100x32 → Resize(77, 23): `ScrollbackRows() == 0`
   (the startup case).
2. One prompt line at row 0, rest blank, shrink rows: scrollback stays 0,
   prompt still at top.
3. Content filling all rows, shrink: unchanged current behavior (top rows go
   to history) — regression guard.
4. Cursor on last row with content above, shrink by N: cursor row preserved,
   no content lost.
5. Blank-but-wrapped trailing row is not trimmed.
6. Grow after shrink round-trips without inventing or losing rows.

## Verify E2E

Fresh cmd.exe launch → aggressive wheel-up → the banner stays at the top of
the view; zero rows above it (`ScrollbackRows()==0` equivalent visually).

## Flow

Branch `fix/resize-blank-scrollback` off main. Implementer: Codex
(gpt-5.6-sol) — core-only slice. Reviewer: Fable (traps) + E2E + PR.
