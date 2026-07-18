//go:build glfw

package glfwgl

import (
	"fmt"
	"time"

	"cervterm/internal/fontglyph"
	termmux "cervterm/internal/mux"
)

// paneFontState keeps renderer-neutral font metrics and zoom settlement state
// attached to the pane that originated the gesture.
type paneFontState struct {
	fontSize      float64
	cellW         float32
	cellH         float32
	baseline      int
	pendingTarget float64
	pending       bool
	ptyDirty      bool
	resizeAttempt int
	deadline      time.Time
}

const (
	zoomDebounce               = 70 * time.Millisecond
	paneZoomResizeMaxAttempts  = 5
	paneZoomResizeRetryInitial = 100 * time.Millisecond
)

func (a *App) initialPaneFontState() paneFontState {
	size := a.cfg.Font.Size
	if size == 0 {
		size = a.zoom.base
	}
	state := paneFontState{fontSize: size, cellW: a.cellW, cellH: a.cellH}
	if a.atlas != nil {
		state.baseline = a.atlas.baseline
	}
	if state.cellW <= 0 {
		state.cellW = 1
	}
	if state.cellH <= 0 {
		state.cellH = 1
	}
	return state
}

func (a *App) inheritPaneFontState(target, source termmux.PaneID) {
	if target == 0 {
		return
	}
	inherited := a.initialPaneFontState()
	if sourceState := a.paneUI[source]; sourceState != nil {
		inherited.fontSize = sourceState.font.fontSize
		inherited.cellW = sourceState.font.cellW
		inherited.cellH = sourceState.font.cellH
		inherited.baseline = sourceState.font.baseline
	}
	state := a.ensurePaneUI(target)
	state.font = inherited
}

func clampZoomFontSize(pts float64) float64 {
	return min(zoomFontMax, max(zoomFontMin, pts))
}

func (a *App) focusedFontPane() (termmux.PaneID, bool) {
	if a.mux == nil {
		return 0, false
	}
	if a.focusedPane != 0 {
		if _, ok := a.mux.PaneView(a.focusedPane); ok {
			return a.focusedPane, true
		}
	}
	id, ok := a.mux.FocusedPane()
	if !ok {
		return 0, false
	}
	_, ok = a.mux.PaneView(id)
	return id, ok
}

func (a *App) paneFontSize(id termmux.PaneID) float64 {
	if id == 0 {
		return a.cfg.Font.Size
	}
	return a.ensurePaneUI(id).font.fontSize
}

func (a *App) paneZoomTarget(id termmux.PaneID) float64 {
	if id == 0 {
		return a.cfg.Font.Size
	}
	state := a.ensurePaneUI(id)
	if state.font.pending {
		return state.font.pendingTarget
	}
	return state.font.fontSize
}

func (a *App) zoomTarget() float64 {
	id, ok := a.focusedFontPane()
	if !ok {
		return a.cfg.Font.Size
	}
	return a.paneZoomTarget(id)
}

// applyFontSize records a focused-pane zoom target. The visual grid is updated
// by applyPendingZoom on the main loop; the PTY is updated only at settlement.
func (a *App) applyFontSize(pts float64) {
	id, ok := a.focusedFontPane()
	if !ok {
		return
	}
	state := a.ensurePaneUI(id)
	state.font.pendingTarget = clampZoomFontSize(pts)
	state.font.pending = true
	state.font.resizeAttempt = 0
	state.font.deadline = time.Now().Add(zoomDebounce)
	a.requestRedraw()
}

func (a *App) fontSpec(size float64, scaleX, scaleY float32) fontglyph.Spec {
	return fontglyph.Spec{
		Family:     a.cfg.Font.Family,
		Size:       size,
		DPI:        effectiveDPI(scaleX, scaleY),
		TextRaster: a.effectiveTextRaster(),
	}
}

func (a *App) metricsForCells(cellW, cellH float32) termmux.CellMetrics {
	return termmux.CellMetrics{
		CellWidth:  max(1, int(cellW)),
		CellHeight: max(1, int(cellH)),
	}
}

// applyPaneFontVisual selects/acquires the pane's atlas context, updates the
// mux terminal grid first, and leaves PTY notification to the caller.
func (a *App) applyPaneFontVisual(id termmux.PaneID, size float64, scaleX, scaleY float32) (bool, bool) {
	if a.atlas == nil || a.mux == nil {
		return false, false
	}
	state := a.ensurePaneUI(id)
	layout, layoutErr := a.mux.Layout()
	if layoutErr != nil {
		a.Notify(fmt.Sprintf("pane %d: unable to resolve visible font contexts", id))
		return false, false
	}
	pins := a.visibleFontContextKeys(layout, id, size)
	admission, ok := a.atlas.prepareSpecWithPins(a.fontSpec(size, scaleX, scaleY), a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, pins)
	if !ok {
		a.Notify(fmt.Sprintf("pane %d: unable to load font size %.1f", id, size))
		return false, false
	}
	cellW, cellH, baseline := admission.context.cellW, admission.context.cellH, admission.context.baseline
	before, exists := a.mux.PaneView(id)
	if !exists {
		a.atlas.abortContextAdmission(admission)
		return false, false
	}
	events, err := a.mux.ResizePaneGrid(id, a.metricsForCells(float32(cellW), float32(cellH)))
	if err != nil {
		a.atlas.abortContextAdmission(admission)
		a.Notify(fmt.Sprintf("pane %d resize: %v", id, err))
		return false, false
	}
	a.atlas.commitContextAdmission(admission)
	a.handleMuxEvents(events)
	after, _ := a.mux.PaneView(id)
	state.font.fontSize = size
	state.font.cellW = float32(cellW)
	state.font.cellH = float32(cellH)
	state.font.baseline = baseline
	a.requestRedraw()
	return before.DesiredSize != after.DesiredSize, true
}

func (a *App) applyPanePTYResize(id termmux.PaneID) bool {
	if a.mux == nil {
		return false
	}
	events, err := a.mux.ApplyResize(id)
	a.handleMuxEvents(events)
	return err == nil
}

func (a *App) schedulePanePTYResizeRetry(id termmux.PaneID, now time.Time) {
	state := a.ensurePaneUI(id)
	state.font.ptyDirty = true
	if state.font.pending {
		return
	}
	state.font.pending = true
	state.font.pendingTarget = state.font.fontSize
	state.font.resizeAttempt = 1
	state.font.deadline = now.Add(paneZoomResizeRetryInitial)
}

func (a *App) setPaneFontSize(id termmux.PaneID, pts float64) {
	state := a.ensurePaneUI(id)
	state.font.pending = false
	state.font.resizeAttempt = 0
	gridChanged, applied := a.applyPaneFontVisual(id, clampZoomFontSize(pts), a.contentScaleX, a.contentScaleY)
	if applied && gridChanged {
		state.font.ptyDirty = !a.applyPanePTYResize(id)
		if state.font.ptyDirty {
			a.schedulePanePTYResizeRetry(id, time.Now())
		}
	}
	a.restoreFocusedFontProjection()
}

// applyPendingZoom advances every pane with an in-flight zoom. Focus changes do
// not retarget work because the pending state is keyed by the original pane ID.
func (a *App) applyPendingZoom() {
	now := time.Now()
	for id, state := range a.paneUI {
		if state == nil || !state.font.pending {
			continue
		}
		if state.font.fontSize != state.font.pendingTarget {
			gridChanged, ok := a.applyPaneFontVisual(id, state.font.pendingTarget, a.contentScaleX, a.contentScaleY)
			if !ok {
				state.font.pending = false
				continue
			}
			state.font.ptyDirty = state.font.ptyDirty || gridChanged
		}
		if now.Before(state.font.deadline) {
			continue
		}
		if !state.font.ptyDirty {
			state.font.pending = false
			state.font.resizeAttempt = 0
			continue
		}
		if a.applyPanePTYResize(id) {
			state.font.pending = false
			state.font.ptyDirty = false
			state.font.resizeAttempt = 0
			continue
		}
		state.font.resizeAttempt++
		if state.font.resizeAttempt >= paneZoomResizeMaxAttempts {
			state.font.pending = false
			state.font.ptyDirty = false
			a.Notify(fmt.Sprintf("pane %d: resize retries exhausted", id))
			continue
		}
		state.font.deadline = now.Add(paneZoomResizeRetryInitial << (state.font.resizeAttempt - 1))
	}
	a.restoreFocusedFontProjection()
}

func (a *App) earliestPendingZoomDeadline() (time.Time, bool) {
	var earliest time.Time
	for _, state := range a.paneUI {
		if state == nil || !state.font.pending {
			continue
		}
		if earliest.IsZero() || state.font.deadline.Before(earliest) {
			earliest = state.font.deadline
		}
	}
	return earliest, !earliest.IsZero()
}

func (a *App) activatePaneFont(id termmux.PaneID) bool {
	state := a.ensurePaneUI(id)
	if a.atlas != nil {
		cellW, cellH, baseline, ok := a.atlas.useSpec(a.fontSpec(state.font.fontSize, a.contentScaleX, a.contentScaleY), a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if !ok {
			return false
		}
		state.font.cellW, state.font.cellH = float32(cellW), float32(cellH)
		state.font.baseline = baseline
		a.ligaturesActive = a.atlas.supportsLigatures(a.cfg.Font.Ligatures)
	}
	a.cellW, a.cellH = state.font.cellW, state.font.cellH
	return true
}

func (a *App) restoreFocusedFontProjection() {
	if id, ok := a.focusedFontPane(); ok {
		a.activatePaneFont(id)
	}
}

func (a *App) visibleFontContextKeys(layout termmux.Layout, overridePane termmux.PaneID, overrideSize float64) map[atlasFontKey]struct{} {
	return a.visibleFontContextKeysForRaster(layout, overridePane, overrideSize, "")
}

func (a *App) visibleFontContextKeysForRaster(layout termmux.Layout, overridePane termmux.PaneID, overrideSize float64, textRaster string) map[atlasFontKey]struct{} {
	keep := make(map[atlasFontKey]struct{}, len(layout.Panes))
	for _, geometry := range layout.Panes {
		state := a.paneUI[geometry.Pane]
		if state == nil {
			continue
		}
		size := state.font.fontSize
		if overrideSize > 0 && geometry.Pane == overridePane {
			size = overrideSize
		}
		spec := a.fontSpec(size, a.contentScaleX, a.contentScaleY)
		if textRaster != "" {
			spec.TextRaster = textRaster
		}
		key, err := a.atlas.fontKey(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if err == nil {
			keep[key] = struct{}{}
		}
	}
	return keep
}

func (a *App) retainVisibleFontContexts(layout termmux.Layout) bool {
	if a.atlas == nil {
		return false
	}
	return a.atlas.retainContexts(a.visibleFontContextKeys(layout, 0, 0))
}
