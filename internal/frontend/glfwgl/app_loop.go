//go:build glfw

package glfwgl

import (
	"time"

	ptyio "cervterm/internal/pty"
	"cervterm/internal/render"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// requestRedraw marks the next frame dirty. Main-thread only: the PTY reader
// goroutine must never call this — it wakes the loop with glfw.PostEmptyEvent
// and lets drainIncoming set the flag.
func (a *App) requestRedraw() { a.needsRedraw = true }

// wakeMainLoop nudges the event wait from the reader goroutine. PostEmptyEvent
// is the only GLFW call safe from a non-main thread, and only while GLFW is
// initialized — wakeReady bounds it to that window.
func (a *App) wakeMainLoop() {
	if a.wakeReady.Load() {
		glfw.PostEmptyEvent()
	}
}

// runLoop dispatches to the configured render loop. "continuous" reproduces the
// historical poll-and-always-draw loop for benchmarking; the default
// "on_demand" loop blocks in the OS event wait and draws only on damage.
func (a *App) runLoop(w *glfw.Window) error {
	if a.cfg.Render.Redraw == "continuous" {
		return a.runContinuousLoop(w)
	}
	return a.runOnDemandLoop(w)
}

// runContinuousLoop is the pre-on-demand loop: poll, drain, always draw. Event
// dispatch moved out of draw() into processTermEvents, so it is called here too
// to keep bells/titles firing with identical observable behavior.
func (a *App) runContinuousLoop(w *glfw.Window) error {
	for !w.ShouldClose() {
		glfw.PollEvents()
		consumed := a.drainIncoming()
		a.processTermEvents(consumed)
		a.applyPendingZoom()
		a.resizeToWindow()
		a.fireDueTimers(time.Now())
		a.fireLifecycleEvents()
		a.syncStatusSegments()
		a.syncOverlays()
		a.draw()
		w.SwapBuffers()
		a.meter.AddFrame()
	}
	return nil
}

// runOnDemandLoop blocks in glfw.WaitEventsTimeout until an OS event, a
// PostEmptyEvent wake, or the next time-driven deadline (nextWakeTimeout), then
// draws only when shouldRedraw reports damage.
func (a *App) runOnDemandLoop(w *glfw.Window) error {
	for !w.ShouldClose() {
		glfw.WaitEventsTimeout(a.nextWakeTimeout(time.Now()).Seconds())
		consumed := a.drainIncoming()
		a.processTermEvents(consumed)
		a.applyPendingZoom()
		a.resizeToWindow()
		a.fireDueTimers(time.Now())
		a.fireLifecycleEvents()
		a.syncStatusSegments()
		a.syncOverlays()
		if a.shouldRedraw(time.Now()) {
			a.draw()
			w.SwapBuffers()
			a.meter.AddFrame()
			a.needsRedraw = false
		}
	}
	return nil
}

// drainIncoming applies every queued PTY chunk. It returns whether it consumed
// any data and requests a redraw in that case.
func (a *App) drainIncoming() bool {
	consumed := false
	for {
		select {
		case data := <-a.incoming:
			a.mu.Lock()
			a.parser.Advance(a.term, data)
			a.mu.Unlock()
			a.flushReplies()
			// Fire on_output outside the lock: a handler may call term:write,
			// which re-enters writeInput and would deadlock on a.mu.
			if a.scriptRT != nil && a.scriptRT.WantsOutput() {
				if err := a.scriptRT.FireOutput(a, string(data)); err != nil {
					a.Notify("script error: " + err.Error())
				}
			}
			a.meter.AddBytes(len(data))
			consumed = true
		default:
			if consumed {
				a.requestRedraw()
			}
			return consumed
		}
	}
}

// processTermEvents fires title/cwd/bell handlers on the main thread. It runs every
// loop iteration but only captures a snapshot when the parser advanced — via
// drainIncoming or via the no-PTY fallback (termEventsPending) — so
// bells/titles/cwd changes fire promptly even when draw() is skipped by on-demand
// rendering. draw() renders the already-captured snapshot. The pending flag is
// cleared before handlers run: a handler's term:write re-arms it for the next
// iteration instead of re-entering dispatch.
func (a *App) processTermEvents(consumed bool) {
	if a.termEventsPending {
		consumed = true
		a.termEventsPending = false
	}
	if !consumed {
		return
	}
	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	a.mu.Unlock()
	if a.snap.Title != a.lastTitle {
		a.lastTitle = a.snap.Title
		if a.cfg.Window.DynamicTitle && a.snap.Title != "" {
			a.window.SetTitle("CervTerm · " + a.snap.Title)
		} else {
			a.window.SetTitle("CervTerm")
		}
		a.fireScriptEvent(func() error { return a.scriptRT.FireTitle(a, a.snap.Title) })
	}
	if a.snap.Cwd != a.lastCwd {
		a.lastCwd = a.snap.Cwd
		a.fireScriptEvent(func() error { return a.scriptRT.FireCwd(a, a.snap.Cwd) })
	}
	// BellCount is monotonic; fire once per bell so bursts are not collapsed.
	for a.lastBellCount < a.snap.BellCount {
		a.lastBellCount++
		a.fireScriptEvent(func() error { return a.scriptRT.FireBell(a) })
	}
}

// shouldRedraw reports whether the frame must be repainted now: an explicit
// damage request, a blink phase flip, an expiring notice, or the stats HUD
// refresh window elapsing.
func (a *App) shouldRedraw(now time.Time) bool {
	if a.needsRedraw {
		return true
	}
	if a.blinkActive() && a.blinkPhaseAt(now) != a.lastBlinkPhase {
		return true
	}
	if a.notice != "" && now.After(a.noticeUntil) {
		return true // one repaint to clear the expired notice
	}
	if a.showStats && now.Sub(a.lastStatsDraw) >= 500*time.Millisecond {
		return true
	}
	return false
}

// nextWakeTimeout bridges the pure nextWake helper to App state. A redraw
// already pending (e.g. the first frame before any OS event) short-circuits to
// minWake so the wait does not stall the paint for up to maxWake.
func (a *App) nextWakeTimeout(now time.Time) time.Duration {
	if a.needsRedraw {
		return minWake
	}
	// A pending timer bounds the wait. Zero (no timers, or no runtime) leaves
	// nextWake unchanged, so an idle terminal with no timers still costs nothing.
	var timerDeadline time.Time
	if a.scriptRT != nil {
		if deadline, ok := a.scriptRT.NextTimerDeadline(); ok {
			timerDeadline = deadline
		}
	}
	wake := nextWake(now, a.blinkActive(), a.blinkStart, a.blinkPeriod(), a.noticeUntil, a.showStats, timerDeadline)
	// A debounced zoom must wake the loop when its deadline arrives so the coalesced
	// rebuild fires even if no other event does — but never past an earlier deadline
	// (blink, timers, notice), so those stay on time.
	if a.zoom.pendingSet {
		zoomWake := max(minWake, a.zoom.deadline.Sub(now))
		if wake <= 0 || zoomWake < wake {
			wake = zoomWake
		}
	}
	return wake
}

// blinkPeriod is the full cursor blink period; the phase flips every half.
func (a *App) blinkPeriod() time.Duration {
	interval := a.cfg.Cursor.BlinkIntervalMS
	if interval <= 0 {
		interval = 1000
	}
	return time.Duration(interval) * time.Millisecond
}

// blinkPhaseAt is the raw time-based on/off phase, computed from the elapsed
// time modulo the period (never accumulated) so it cannot drift.
func (a *App) blinkPhaseAt(now time.Time) bool {
	period := a.blinkPeriod()
	return now.Sub(a.blinkStart)%period < period/2
}

// blinkActive mirrors drawCursor: a blink is animating only while the cursor is
// visible (and the viewport is not scrolled away, which CursorVisible already
// encodes) and either the config enables blink or a DECSCUSR blinking style
// (1/3/5) is set. Styles 2/4/6 force steady.
func (a *App) blinkActive() bool {
	if !a.snap.CursorVisible {
		return false
	}
	if b, ok := a.snap.CursorStyle.Blink(); ok {
		return b
	}
	return a.cfg.Cursor.Blink
}

// spawnInitialPTY sizes the terminal to the real initial grid and starts the
// PTY, so terminal and ConPTY agree from byte zero and no startup resize
// repaints the shell banner. Called from runWindow once cellW/cellH are final.
// The terminal resize holds a.mu, but startPTY runs without it (its reader
// goroutine and the failure-path parser feed both take a.mu). Seeding
// a.cols/a.rows makes the loop's first resizeToWindow a no-op.
func (a *App) spawnInitialPTY(w *glfw.Window) {
	fbW, fbH := w.GetFramebufferSize()
	cols, rows := a.gridSize(fbW, fbH)
	a.mu.Lock()
	a.term.Resize(cols, rows)
	a.mu.Unlock()
	a.cols, a.rows = cols, rows
	// Fire events.resize for the initial grid; the first loop iteration drains it.
	a.markResizeEvent(cols, rows)
	if err := a.startPTY(); err != nil {
		a.parser.Advance(a.term, []byte("\x1b[96mCervTerm\x1b[0m\r\n\r\n"))
		a.parser.Advance(a.term, []byte("Local PTY unavailable on this platform/build.\r\n"))
		a.parser.Advance(a.term, []byte(err.Error()+"\r\n\r\n"))
		a.parser.Advance(a.term, []byte("Type to test the renderer and parser.\r\n"))
	}
}

// gridSize maps a framebuffer size (in pixels) to the terminal grid, applying
// the padding and cell metrics. Shared by resizeToWindow and the initial PTY
// spawn in runWindow so the two sites cannot drift.
func (a *App) gridSize(w, h int) (cols, rows int) {
	cols = max(2, int((float32(w)-2*a.paddingX)/a.cellW))
	rows = max(1, int((float32(h)-2*a.paddingY)/a.cellH))
	return cols, rows
}

// resizeToWindow reflows the grid when the framebuffer maps to a new col/row
// count and requests a repaint. It reflows the local grid and notifies ConPTY
// together — the right coupling for window resizes, which the OS already
// rate-limits. Zoom takes the two halves separately (resizeGridToWindow every
// step for frame-by-frame feedback, resizePTYToGrid once at burst settle).
func (a *App) resizeToWindow() {
	if a.resizeGridToWindow() {
		a.resizePTYToGrid()
	}
}

// resizeGridToWindow reflows the local terminal grid to the framebuffer's
// current col/row count and requests a repaint. It does NOT touch the PTY.
// Returns whether the grid dimensions changed.
func (a *App) resizeGridToWindow() bool {
	w, h := a.window.GetFramebufferSize()
	cols, rows := a.gridSize(w, h)
	if cols == a.cols && rows == a.rows {
		return false
	}
	a.cols, a.rows = cols, rows
	a.mu.Lock()
	a.term.Resize(cols, rows)
	a.mu.Unlock()
	a.markResizeEvent(cols, rows)
	a.requestRedraw()
	return true
}

// resizePTYToGrid notifies ConPTY of the current grid dimensions. Kept separate
// from the local reflow so zoom can coalesce it to one call per burst: ConPTY
// repaints its viewport asynchronously on every resize, and several in flight at
// once interleave over the next grid → duplicated/garbled scrollback.
func (a *App) resizePTYToGrid() {
	if a.pty != nil {
		_ = a.pty.Resize(ptyio.Size{Rows: uint16(a.rows), Cols: uint16(a.cols)})
	}
}
