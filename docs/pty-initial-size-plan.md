# Fix: duplicated shell banner on first open

## Symptom

Every fresh launch shows the shell banner twice: the visible one plus an
identical copy in scrollback.

## Cause

`RunWithOptions` starts the PTY before the window exists, sized to the
hardcoded `core.NewTerminal(100, 32)` grid. The shell prints its banner into
that 100x32 ConPTY. The loop's first `resizeToWindow` then computes the real
grid from the actual framebuffer and cell metrics and calls `pty.Resize` —
and ConPTY repaints on resize, re-emitting the visible screen. The original
banner scrolls into history; the user sees two.

## Fix

Start the PTY with the real initial grid so no startup resize happens:

1. Move the `startPTY` call from `RunWithOptions` into `runWindow`, after the
   window + atlas exist and `a.cellW/a.cellH` are final, and after computing
   the initial cols/rows with the same formula `resizeToWindow` uses
   (extract that formula into a small helper `gridSize(w, h int) (cols, rows
   int)` so the two sites cannot drift).
2. Resize `a.term` to that grid BEFORE `startPTY` so terminal and PTY agree
   from byte zero (`term.Resize(cols, rows)` under `a.mu`, then start).
   Update `a.cols/a.rows` so the loop's first `resizeToWindow` is a no-op.
3. The no-PTY failure banner (`startPTY` error → parser.Advance welcome text)
   moves with the call; it renders identically from inside runWindow. Set
   `termEventsPending`/`needsRedraw` after feeding it (the current code does
   this at startup — keep the behavior at the new call site).
4. `RunWithOptions` keeps constructing the App and terminal; only the PTY
   spawn moves. The deferred `pty.Close` in RunWithOptions must handle
   `a.pty == nil` until runWindow starts it (it already nil-checks).

## Traps

1. The PTY reader goroutine still must not post wakes before `wakeReady` —
   starting the PTY later actually shrinks that window; do not remove the
   gate.
2. `startPTY` uses `a.term.Rows()/Cols()` for the ConPTY size — after step 2
   those are the real values; don't pass stale locals.
3. Resize-before-start must hold `a.mu` for `term.Resize` but MUST NOT hold
   it across `startPTY` (reader goroutine + parser feed on failure path).
4. Keep the "Type to test the renderer" fallback text exactly as-is (tests
   may reference it; user-visible contract).

## Verify

1. Fresh launch (cmd.exe default): banner appears exactly once; scrollback
   above the banner is empty (mouse-wheel up shows nothing).
2. pwsh as shell: same.
3. No-PTY fallback (if forced): welcome text renders once.
4. Resize after launch still works (drag → reflow → ConPTY repaint is
   expected behavior there).
5. Full gates + maturity.

## Flow

Branch `fix/pty-initial-size` off main. Implementer: Opus. Reviewer: Fable
(traps checklist) + E2E + PR.
