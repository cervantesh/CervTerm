//go:build glfw

package glfwgl

import (
	"fmt"
	"math"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func DetectContentScale() string {
	if err := glfw.Init(); err != nil {
		return "unavailable (" + err.Error() + ")"
	}
	defer glfw.Terminate()
	monitor := glfw.GetPrimaryMonitor()
	if monitor == nil {
		return "unavailable (no primary monitor)"
	}
	x, y := monitor.GetContentScale()
	return fmt.Sprintf("%.2fx%.2f (effective DPI %.0f)", x, y, effectiveDPI(x, y))
}

func (a *App) applyScale(scaleX, scaleY float32) {
	a.contentScaleX, a.contentScaleY = scaleX, scaleY
	// Derive uiScale from the same clamped factor as the glyph DPI so chrome
	// never grows out of proportion with text past the 4x DPI clamp.
	a.uiScale = float32(effectiveDPI(scaleX, scaleY) / 96)
	// Snap the text-grid origin to whole pixels. cellW/cellH are integers, so an
	// integer origin keeps every glyph quad pixel-aligned; a fractional padding
	// (e.g. 18*1.25 = 22.5) would draw glyphs on half-pixels and the LINEAR atlas
	// filter would blur them.
	a.paddingX = float32(math.Round(float64(a.cfg.Window.PaddingX) * float64(a.uiScale)))
	a.paddingY = float32(math.Round(float64(a.cfg.Window.PaddingY) * float64(a.uiScale)))
}

func (a *App) rebuildForContentScale(scaleX, scaleY float32) {
	if a.contentScaleX == scaleX && a.contentScaleY == scaleY {
		return
	}
	a.applyScale(scaleX, scaleY)
	if a.atlas == nil {
		return
	}
	if a.mux == nil || len(a.mux.PaneIDs()) == 0 {
		cellW, cellH, _, ok := a.atlas.useSpec(a.fontSpec(a.cfg.Font.Size, scaleX, scaleY), a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if ok {
			a.cellW, a.cellH = float32(cellW), float32(cellH)
		}
		return
	}
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		gridChanged, applied := a.applyPaneFontVisual(id, state.font.fontSize, scaleX, scaleY)
		state.font.ptyDirty = state.font.ptyDirty || (applied && gridChanged)
	}
	// Content-scale changes are one-shot window transitions. Settle each pane
	// independently and arm the same bounded retry policy used by pane zoom.
	now := time.Now()
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		if !state.font.ptyDirty {
			continue
		}
		if a.applyPanePTYResize(id) {
			state.font.ptyDirty = false
			continue
		}
		state.font.pending = true
		state.font.pendingTarget = state.font.fontSize
		state.font.resizeAttempt = 1
		state.font.deadline = now.Add(paneZoomResizeRetryInitial)
	}
	a.restoreFocusedFontProjection()
}
