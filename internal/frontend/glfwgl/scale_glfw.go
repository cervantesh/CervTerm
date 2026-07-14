//go:build glfw

package glfwgl

import (
	"fmt"
	"math"

	"cervterm/internal/fontglyph"

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
	a.rebuildAtlasAndGrid(scaleX, scaleY)
}

// rebuildAtlasAndGrid rebuilds the glyph atlas and cell metrics for the current
// cfg.Font.Size at the given content scale, then reflows the grid (which
// resizes the PTY and requests a full repaint). It touches GL, so it must run
// on the main thread with the context current. Both callers satisfy that: the
// content-scale GLFW callback runs on the loop thread, and term:set_font_size is
// dispatched from a key or timer handler, which also run on the loop thread
// between frames — never inside draw().
func (a *App) rebuildAtlasAndGrid(scaleX, scaleY float32) {
	spec := fontglyph.Spec{Family: a.cfg.Font.Family, Size: a.cfg.Font.Size, DPI: effectiveDPI(scaleX, scaleY), TextRaster: a.cfg.Render.TextRaster}
	if a.atlas != nil {
		// Reuse the existing atlas (and its GL textures) instead of allocating a
		// fresh one every zoom step; only the font backend and glyph cache change.
		if !a.atlas.reconfigure(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken) {
			return
		}
	} else {
		atlas, err := newGlyphAtlasWithSpec(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if err != nil {
			return
		}
		a.atlas = atlas
	}
	// The atlas re-probes the shaper (and drops the run caches with it).
	a.ligaturesActive = a.cfg.Font.Ligatures && a.atlas.supportsLigatures()
	a.cellW = float32(a.atlas.cellW)
	a.cellH = float32(a.atlas.cellH)
	a.applyScale(scaleX, scaleY)
	a.cols, a.rows = 0, 0
	a.resizeToWindow()
}
