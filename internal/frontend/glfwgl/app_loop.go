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
func (a *App) requestRedraw() {
	a.needsRedraw = true
	if a.controller != nil {
		a.controller.markDamage(a.windowID)
	}
}

// wakeMainLoop nudges the event wait from the reader goroutine. PostEmptyEvent
// is the only GLFW call safe from a non-main thread, and only while GLFW is
// initialized — wakeReady bounds it to that window.
func (a *App) wakeMainLoop() {
	if a.wakeReady.Load() {
		glfw.PostEmptyEvent()
	}
}

// runLoop owns the process-wide OS-thread scheduler. GLFW is pumped once, mux
// ingress is drained once, and process timers/reload are ticked once per cycle.
// Projection-local state and presentation remain independent.
func (a *App) runLoop(_ *glfw.Window) error {
	startRuntimeMetricsProbe(a)
	return a.runProcessLoop(a.cfg.Render.Redraw == "continuous")
}

func (a *App) runContinuousLoop(_ *glfw.Window) error { return a.runProcessLoop(true) }

func (a *App) runOnDemandLoop(_ *glfw.Window) error { return a.runProcessLoop(false) }

type projectionCycleScheduler interface {
	projectionIDs() []termmux.WindowID
	shouldClose(termmux.WindowID) bool
	closeRuntimeProjection(termmux.WindowID) (termmux.CloseWindowResult, error)
}

func runProjectionCycle(s projectionCycleScheduler, frame func(termmux.WindowID) error) error {
	for _, id := range s.projectionIDs() {
		if s.shouldClose(id) {
			if _, err := s.closeRuntimeProjection(id); err != nil {
				return err
			}
			continue
		}
		if err := frame(id); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) runProcessLoop(continuous bool) error {
	for a.controller.projectionCount() > 0 {
		now := time.Now()
		if continuous {
			if err := a.controller.pollEvents(); err != nil {
				return err
			}
		} else if err := a.controller.waitEvents(a.processNextWakeTimeout(now)); err != nil {
			return err
		}

		events := a.controller.drainMux(256)
		a.controller.dispatch(events)
		now = time.Now()
		if activeID := a.controller.active; activeID != 0 {
			if err := a.controller.withCurrent(activeID, func() {
				if active := a.controller.activeProjectionApp(); active != nil {
					active.fireDueTimers(now)
				}
			}); err != nil {
				return err
			}
		}
		if a.controller.projectionApp(a.windowID) != nil {
			if err := a.controller.withCurrent(a.windowID, func() { a.pollConfigReload(now); a.applyPendingConfigReload() }); err != nil {
				return err
			}
		}
		if err := a.controller.syncSharedProjectionState(a); err != nil {
			return err
		}

		if err := runProjectionCycle(a.controller, func(id termmux.WindowID) error {
			projection := a.controller.projectionApp(id)
			if projection == nil {
				return nil
			}
			if !a.controller.projectionVisible(id) {
				return nil
			}
			drew := false
			if err := a.controller.withCurrent(id, func() {
				projection.tickRenderProjection()
				now = time.Now()
				if !continuous && !projection.renderReady(now) {
					return
				}
				if continuous {
					projection.throttleRender(now)
				}
				projection.draw()
				projection.endRenderFrame()
				drew = true
			}); err != nil {
				return err
			}
			if drew {
				a.controller.clearDamage(id)
				projection.presentation.record(time.Now())
				projection.meter.AddFrame()
				projection.needsRedraw = false
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) tickRenderProjection() {
	a.tickProjection()
}

func (a *App) renderReady(now time.Time) bool {
	return a.shouldRedraw(now)
}

func (a *App) throttleRender(now time.Time) {
	if wait := a.presentation.wait(now, a.cfg.Render.MaxFPS); wait > 0 {
		time.Sleep(wait)
	}
}

func (a *App) endRenderFrame() {
	a.r.EndFrame()
}

func (a *App) tickProjection() {
	recordRuntimeMetricsWake(a)
	a.processTermEvents(false)
	a.catchUpBellEvents()
	a.applyPendingZoom()
	a.applyPendingDividerResize()
	a.resizeToWindow()
	a.fireLifecycleEvents()
	a.syncStatusSegments()
	a.syncOverlays()
	if a.accessibilityRuntime != nil {
		if err := a.accessibilityRuntime.Refresh(); err != nil {
			a.failAccessibilityRuntime(err)
		}
	}
}

func (a *App) processNextWakeTimeout(now time.Time) time.Duration {
	wake := maxWake
	if a.mux != nil {
		if deadline, ok := a.mux.NextImageDeadline(); ok {
			candidate := max(minWake, deadline.Sub(now))
			if candidate < wake {
				wake = candidate
			}
		}
	}
	for _, id := range a.controller.projectionIDs() {
		projection := a.controller.projectionApp(id)
		if projection == nil || !a.controller.projectionVisible(id) {
			continue
		}
		candidate := projection.nextWakeTimeout(now)
		if candidate > 0 && candidate < wake {
			wake = candidate
		}
	}
	return wake
}

// drainIncoming advances pane-addressed mux ingress on the main thread.
func (a *App) drainIncoming() bool {
	events := a.drainMuxEvents(256)
	return a.dispatchMuxEvents(events)
}

// processTermEvents drains synthetic events produced by main-thread Host calls.
func (a *App) processTermEvents(_ bool) {
	if len(a.pendingMuxEvents) == 0 {
		a.syncFocusedProjection()
		return
	}
	events := a.pendingMuxEvents
	a.pendingMuxEvents = nil
	for i := range events {
		if events[i].Window == 0 {
			events[i].Window = a.windowID
		}
	}
	a.dispatchMuxEvents(events)
}

// redrawWanted reports whether visible state currently demands a presentation.
// Cache retries request a frame only when a visible failed generation reaches
// its fixed retry deadline; an empty/idle cache contributes no cadence.
func (a *App) redrawWanted(now time.Time) bool {
	if a.needsRedraw {
		return true
	}
	if a.terminalImageDamage.pending() {
		return true
	}
	if a.terminalImageCache.retryDue(now) {
		return true
	}
	if a.blinkActive() && a.blinkPhaseAt(now) != a.lastBlinkPhase {
		return true
	}
	if a.notice != "" && now.After(a.noticeUntil) {
		return true // one repaint to clear the expired notice
	}
	if !a.bellVisualUntil.IsZero() && !now.Before(a.bellVisualUntil) {
		return true // one repaint to clear the visual bell
	}
	if a.showStats && now.Sub(a.lastStatsDraw) >= 500*time.Millisecond {
		return true
	}
	return a.scrollbarNeedsRedraw(now)
}

// shouldRedraw applies the presentation cap without clearing redraw demand.
func (a *App) shouldRedraw(now time.Time) bool {
	return a.redrawWanted(now) && a.presentation.ready(now, a.cfg.Render.MaxFPS)
}

// nextWakeTimeout bridges the pure nextWake helper to App state. A redraw
// already pending (e.g. the first frame before any OS event) short-circuits to
// minWake so the wait does not stall the paint for up to maxWake.
func (a *App) nextWakeTimeout(now time.Time) time.Duration {
	presentationWait := a.presentation.wait(now, a.cfg.Render.MaxFPS)
	if a.redrawWanted(now) {
		return max(minWake, presentationWait)
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
	// A debounced zoom must wake for the earliest pane-local settlement without
	// delaying an earlier blink, timer, notice, or sibling zoom deadline.
	if deadline, ok := a.earliestPendingZoomDeadline(); ok {
		zoomWake := max(minWake, deadline.Sub(now))
		if wake <= 0 || zoomWake < wake {
			wake = zoomWake
		}
	}
	if deadline, ok := a.terminalImageCache.nextRetryDeadline(); ok {
		retryWake := max(minWake, deadline.Sub(now))
		if wake <= 0 || retryWake < wake {
			wake = retryWake
		}
	}
	if a.divider.settlePending {
		dividerWake := max(minWake, a.divider.settleAt.Sub(now))
		if wake <= 0 || dividerWake < wake {
			wake = dividerWake
		}
	}
	if scrollbarWake, ok := a.scrollbarWake(now); ok {
		scrollbarWake = max(minWake, scrollbarWake)
		if wake <= 0 || scrollbarWake < wake {
			wake = scrollbarWake
		}
	}
	if !a.bellVisualUntil.IsZero() {
		bellWake := max(minWake, a.bellVisualUntil.Sub(now))
		if wake <= 0 || bellWake < wake {
			wake = bellWake
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
	eventsRect := a.muxContentBounds(fbW, fbH)
	_, pane, events, err := a.mux.Bootstrap(termmux.SpawnSpec{Options: ptyio.Options{
		ShellProgram: a.cfg.Shell.Program, ShellArgs: a.cfg.Shell.Args,
		WorkingDirectory: a.cfg.Shell.WorkingDirectory, Env: a.cfg.Shell.Env,
	}}, eventsRect, a.muxMetrics())
	a.setFocusedPane(pane)
	a.dispatchMuxEvents(events)
	a.syncFocusedProjection()
	a.markResizeEvent(a.cols, a.rows)
	if err != nil {
		a.Notify("PTY unavailable: " + err.Error())
	}
}

func (a *App) outerInsets() OuterInsets {
	return OuterInsets{
		Left: float64(a.cfg.Window.PaddingLeft), Right: float64(a.cfg.Window.PaddingRight),
		Top: float64(a.cfg.Window.PaddingTop), Bottom: float64(a.cfg.Window.PaddingBottom),
	}
}

func (a *App) windowGeometry(width, height int) WindowGeometry {
	return resolveWindowGeometryWithTabBar(width, height, a.insets, a.scrollbarReservedWidth(), a.effectiveTabBarHeight(), a.cfg.TabBar.Position)
}

func (a *App) muxContentBounds(width, height int) termmux.PixelRect {
	content := a.windowGeometry(width, height).Content
	return termmux.PixelRect{X: content.X, Y: content.Y, Width: content.Width, Height: content.Height}
}

// gridSize maps a framebuffer size to the terminal grid through the same content
// rectangle used by mux layout and PTY sizing.
func (a *App) gridSize(w, h int) (cols, rows int) {
	content := a.windowGeometry(w, h).Content
	cols = max(2, int(float32(content.Width)/a.cellW))
	rows = max(1, int(float32(content.Height)/a.cellH))
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
	events, err := a.mux.ResizeBounds(a.muxContentBounds(w, h))
	if err != nil {
		a.Notify("resize: " + err.Error())
		return false
	}
	changed := a.dispatchMuxEvents(events)
	if a.syncFocusedProjection() {
		a.markResizeEvent(a.cols, a.rows)
	}
	return changed
}

// resizePTYToGrid applies each pane's latest desired size at a settlement
// boundary and reports whether every pane accepted it. Window-driven failures
// enter the bounded pane retry path; divider settlement owns its own retries.
func (a *App) resizePTYToGrid() bool { return a.resizePTYToGridWithRetry(true, true) }

func (a *App) resizePTYToGridReporting(reportFailure bool) bool {
	return a.resizePTYToGridWithRetry(reportFailure, false)
}

func (a *App) resizePTYToGridWithRetry(reportFailure, scheduleRetry bool) bool {
	succeeded := true
	now := time.Now()
	for _, id := range a.mux.PaneIDs() {
		events, err := a.mux.ApplyResize(id)
		if err == nil || reportFailure {
			a.dispatchMuxEvents(events)
		}
		if err != nil {
			succeeded = false
			if scheduleRetry {
				a.schedulePanePTYResizeRetry(id, now)
			}
		}
	}
	return succeeded
}
