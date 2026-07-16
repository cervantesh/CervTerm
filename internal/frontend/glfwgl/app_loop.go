//go:build glfw

package glfwgl

import (
	"time"

	termmux "cervterm/internal/mux"
	ptyio "cervterm/internal/pty"

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
		a.applyPendingDividerResize()
		a.resizeToWindow()
		a.fireDueTimers(time.Now())
		a.fireLifecycleEvents()
		a.syncStatusSegments()
		a.syncOverlays()
		a.draw()
		a.r.EndFrame()
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
		a.applyPendingDividerResize()
		a.resizeToWindow()
		a.fireDueTimers(time.Now())
		a.fireLifecycleEvents()
		a.syncStatusSegments()
		a.syncOverlays()
		if a.shouldRedraw(time.Now()) {
			a.draw()
			a.r.EndFrame()
			a.meter.AddFrame()
			a.needsRedraw = false
		}
	}
	return nil
}

// drainIncoming advances pane-addressed mux ingress on the main thread.
func (a *App) drainIncoming() bool {
	events := a.mux.Drain(256)
	return a.handleMuxEvents(events)
}

// processTermEvents drains synthetic events produced by main-thread Host calls.
func (a *App) processTermEvents(_ bool) {
	if len(a.pendingMuxEvents) == 0 {
		a.syncFocusedProjection()
		return
	}
	events := a.pendingMuxEvents
	a.pendingMuxEvents = nil
	a.handleMuxEvents(events)
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
	if a.divider.settlePending {
		dividerWake := max(minWake, a.divider.settleAt.Sub(now))
		if wake <= 0 || dividerWake < wake {
			wake = dividerWake
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

// spawnInitialPTY bootstraps the implicit mux tab at the real initial grid.
func (a *App) spawnInitialPTY(w *glfw.Window) {
	fbW, fbH := w.GetFramebufferSize()
	eventsRect := termmux.PixelRect{Width: fbW, Height: fbH}
	_, pane, events, err := a.mux.Bootstrap(termmux.SpawnSpec{Options: ptyio.Options{
		ShellProgram: a.cfg.Shell.Program, ShellArgs: a.cfg.Shell.Args,
		WorkingDirectory: a.cfg.Shell.WorkingDirectory, Env: a.cfg.Shell.Env,
	}}, eventsRect, a.muxMetrics())
	a.focusedPane = pane
	a.handleMuxEvents(events)
	a.syncFocusedProjection()
	a.markResizeEvent(a.cols, a.rows)
	if err != nil {
		a.Notify("PTY unavailable: " + err.Error())
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
	events, err := a.mux.ResizeGrid(termmux.PixelRect{Width: w, Height: h}, a.muxMetrics())
	if err != nil {
		a.Notify("resize: " + err.Error())
		return false
	}
	changed := a.handleMuxEvents(events)
	if a.syncFocusedProjection() {
		a.markResizeEvent(a.cols, a.rows)
	}
	return changed
}

// resizePTYToGrid applies each pane's latest desired size at a settlement
// boundary and reports whether every pane accepted it.
func (a *App) resizePTYToGrid() bool {
	succeeded := true
	for _, id := range a.mux.PaneIDs() {
		events, err := a.mux.ApplyResize(id)
		a.handleMuxEvents(events)
		if err != nil {
			succeeded = false
			a.Notify("resize: " + err.Error())
		}
	}
	return succeeded
}
