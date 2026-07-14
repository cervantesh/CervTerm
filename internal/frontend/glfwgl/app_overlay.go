//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/core"
	"cervterm/internal/script"

	"github.com/go-gl/gl/v2.1/gl"
)

// overlayRender mirrors the committed script overlay scenes into the frontend.
// scenes is refreshed only when seq changes (a real commit/show/hide/destroy),
// so steady frames reuse the cached slice and idle terminals pay nothing.
type overlayRender struct {
	seq    int
	scenes []script.OverlayScene
}

// syncOverlays runs after all script handlers/timers for the loop pass, before
// shouldRedraw, mirroring syncStatusSegments. It surfaces any deduped build-time
// notices and re-fetches the committed scenes on a seq change, requesting a
// repaint so the new scene lands promptly on an on-demand frame.
func (a *App) syncOverlays() {
	if a.scriptRT == nil {
		return
	}
	for _, msg := range a.scriptRT.DrainOverlayNotices() {
		a.Notify("script error: " + msg)
	}
	seq := a.scriptRT.OverlaySeq()
	if seq == a.overlays.seq {
		return
	}
	a.overlays.seq = seq
	a.overlays.scenes = a.scriptRT.Overlays()
	a.requestRedraw()
}

// markOverlayDamage marks every visible overlay's covered rows damaged. Called
// each frame from prepareDamage: a translucent overlay must recomposite over a
// freshly painted terminal row rather than blend onto its own prior output, so
// the rows beneath it repaint even when the terminal content there is unchanged
// (trap 1). This never pins a full-frame redraw — only the seq change does, and
// only for the frame it changes.
func (a *App) markOverlayDamage(damaged []bool, rows int) {
	for i := range a.overlays.scenes {
		sc := &a.overlays.scenes[i]
		if !sc.Visible {
			continue
		}
		first, last, any := script.CoveredRows(sc.Prims, rows)
		if !any {
			continue
		}
		for row := first; row <= last; row++ {
			markDamagedRow(damaged, row)
		}
	}
}

// drawOverlays renders the committed scenes in creation order on top of the
// terminal cells and cursor, but beneath system UI (HUD, status band, search
// bar). Pure Go over the committed snapshot — Lua never runs here (trap 2).
func (a *App) drawOverlays() {
	if len(a.overlays.scenes) == 0 {
		return
	}
	cols, rows := a.snap.Cols, a.snap.Rows
	gl.Disable(gl.TEXTURE_2D)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	for i := range a.overlays.scenes {
		sc := &a.overlays.scenes[i]
		if !sc.Visible {
			continue
		}
		for _, p := range sc.Prims {
			a.drawOverlayPrim(p, cols, rows)
		}
	}
}

func (a *App) drawOverlayPrim(p script.OverlayPrim, cols, rows int) {
	c := color.RGBA{R: p.R, G: p.G, B: p.B, A: p.A}
	switch p.Kind {
	case script.OverlayRect:
		if x0, y0, x1, y1, ok := script.ClipCellRect(p.Col, p.Row, p.W, p.H, cols, rows); ok {
			x := a.paddingX + float32(x0)*a.cellW
			y := a.paddingY + float32(y0)*a.cellH
			fillRect(x, y, float32(x1-x0+1)*a.cellW, float32(y1-y0+1)*a.cellH, c)
		}
	case script.OverlayHLine:
		if x0, y0, x1, _, ok := script.ClipCellRect(p.Col, p.Row, p.W, 1, cols, rows); ok {
			x := a.paddingX + float32(x0)*a.cellW
			y := a.paddingY + float32(y0)*a.cellH
			fillRect(x, y, float32(x1-x0+1)*a.cellW, max(1, a.uiScale), c)
		}
	case script.OverlayVLine:
		if x0, y0, _, y1, ok := script.ClipCellRect(p.Col, p.Row, 1, p.H, cols, rows); ok {
			x := a.paddingX + float32(x0)*a.cellW
			y := a.paddingY + float32(y0)*a.cellH
			fillRect(x, y, max(1, a.uiScale), float32(y1-y0+1)*a.cellH, c)
		}
	case script.OverlayText:
		a.drawOverlayText(p, cols, rows, c)
	}
}

// drawOverlayText draws a single line, advancing per rune by the grid's real
// cell measurement so wide/emoji runes occupy the same span the terminal gives
// them (trap 3). Runes whose cell falls outside the grid are skipped but still
// advance the cursor, so a partially off-grid line keeps its alignment.
func (a *App) drawOverlayText(p script.OverlayPrim, cols, rows int, c color.RGBA) {
	if p.Row < 1 || p.Row > rows {
		return
	}
	y := a.paddingY + float32(p.Row-1)*a.cellH
	cell := p.Col - 1
	for _, r := range p.Text {
		w := core.RuneWidth(r)
		if w > 0 && cell >= 0 && cell < cols {
			x := a.paddingX + float32(cell)*a.cellW
			a.drawRune(r, x, y, c, 1, 0)
		}
		if w > 0 {
			cell += w
		}
	}
}
