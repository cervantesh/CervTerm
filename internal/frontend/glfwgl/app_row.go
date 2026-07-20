//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/core"
	"cervterm/internal/fontdesc"
	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
)

func (a *App) drawRow(r int, background, selectionColor, defaultFG color.RGBA, resolver *core.ColorResolver) []int {
	rowCells := a.snap.Cells[r*a.snap.Cols : (r+1)*a.snap.Cols]
	var order []int
	if a.cfg.Render.Bidi {
		order = render.VisualOrder(rowCells)
	}
	// Reuse a single scratch buffer across rows and frames instead of
	// allocating per row; at uncapped frame rates that alloc dominated churn.
	if cap(a.skippedGlyph) < a.snap.Cols {
		a.skippedGlyph = make([]bool, a.snap.Cols)
	}
	skippedGlyph := a.skippedGlyph[:a.snap.Cols]
	clear(skippedGlyph)
	// Ligatures work on logical order only, so they stay off for BiDi-reordered
	// rows (order != nil). cursorCol splits the run under the cursor (§4).
	ligate := a.ligaturesActive && order == nil
	cursorCol := -1
	if a.snap.CursorVisible && a.snap.CursorRow == r {
		cursorCol = a.snap.CursorCol
	}
	for visualCol := 0; visualCol < a.snap.Cols; visualCol++ {
		logicalCol := visualCol
		if order != nil {
			logicalCol = order[visualCol]
		}
		cell := rowCells[logicalCol]
		x := a.drawOriginX + float32(visualCol)*a.cellW
		y := a.drawOriginY + float32(r)*a.cellH
		// Explicit cell backgrounds composite over the canonical pane surface; the
		// multiplier changes source alpha once and blending is not a second multiplier.
		if cell.Attr.HasExplicitBG() {
			a.fillRect(x, y, a.cellW, a.cellH, applyOpacity(rgb(resolver.ResolveBG(cell.Attr.BG)), a.cfg.Window.BackgroundOpacity))
		}
		if a.selection.active && termsel.Contains(termsel.Range{Start: a.selection.start, End: a.selection.end}, termsel.Point{Row: r, Col: logicalCol}) {
			a.fillRect(x, y, a.cellW, a.cellH, selectionColor)
		}
		if a.search.hasMatch && r == a.search.viewRow &&
			logicalCol >= a.search.matchCol && logicalCol < a.search.matchCol+a.search.matchLen {
			a.fillRect(x, y, a.cellW, a.cellH, a.chrome.searchMatch)
		}
		fg := defaultFG
		if cell.Attr.HasExplicitFG() {
			fg = rgb(resolver.ResolveFG(cell.Attr.FG))
		}
		bg := background
		if cell.Attr.HasExplicitBG() {
			bg = rgb(resolver.ResolveBG(cell.Attr.BG))
		}
		if cell.Attr.Inverse {
			fg, bg = bg, fg
			// Inverse video promotes the resolved foreground into the background role.
			a.fillRect(x, y, a.cellW, a.cellH, applyOpacity(bg, a.cfg.Window.BackgroundOpacity))
		}
		if skippedGlyph[logicalCol] || cell.Rune == ' ' || cell.Rune == 0 || cell.WideContinuation {
			continue
		}
		if cell.Attr.Bold {
			fg = brighten(fg)
		}
		if cell.Attr.Dim {
			fg = dim(fg)
		}
		fg = applyOpacity(fg, a.cfg.Window.TextOpacity)
		if rects, ok := render.BoxGlyph(cell.Rune, a.cellW, a.cellH); ok {
			for _, rc := range rects {
				c := fg
				if rc.Alpha < 1 {
					c.A = uint8(float32(fg.A) * rc.Alpha)
				}
				a.fillRect(x+rc.X, y+rc.Y, rc.W, rc.H, c)
			}
			a.drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
			continue
		}
		request := fontdesc.RequestedFaceStyleFromAttributes(cell.Attr.Bold, cell.Attr.Italic)
		// A ligature hit draws the whole span once (bold doubling + decorations
		// over the span) and marks the covered cells; a miss falls through to the
		// per-cell path. Per-cell backgrounds/selection already painted above.
		if ligate {
			if run, ok := detectLigatureRun(rowCells, logicalCol, cursorCol); ok &&
				renderSpanMatchesStyle(rowCells, logicalCol, run.CellSpan, request) {
				_, synthetic := a.atlas.resolveClusterStyle(request, run.Text)
				duplicateBold, skew := styleDrawEffects(synthetic, a.cellH)
				if a.atlas.drawRunStyle(request, run.Text, run.CellSpan, x, y, fg, 1, skew) {
					if duplicateBold {
						a.atlas.drawRunStyle(request, run.Text, run.CellSpan, x+1, y, fg, 1, skew)
					}
					a.drawTextDecorations(x, y, a.cellW*float32(run.CellSpan), a.cellH, fg, cell.Attr)
					for i := 1; i < run.CellSpan && logicalCol+i < a.snap.Cols; i++ {
						skippedGlyph[logicalCol+i] = true
					}
					continue
				}
			}
		}
		if cluster, ok := collectRenderCluster(a.snap.Cells, a.snap.Cols, r, logicalCol); ok &&
			renderSpanMatchesStyle(rowCells, logicalCol, cluster.CellSpan, request) {
			_, synthetic := a.atlas.resolveClusterStyle(request, cluster.Text)
			duplicateBold, skew := styleDrawEffects(synthetic, a.cellH)
			if a.atlas.drawClusterStyle(request, cluster.Text, cluster.CellSpan, x, y, fg, 1, skew) {
				if duplicateBold {
					a.atlas.drawClusterStyle(request, cluster.Text, cluster.CellSpan, x+1, y, fg, 1, skew)
				}
				a.drawTextDecorations(x, y, a.cellW*float32(cluster.CellSpan), a.cellH, fg, cell.Attr)
				for i := 1; i < cluster.CellSpan && logicalCol+i < a.snap.Cols; i++ {
					skippedGlyph[logicalCol+i] = true
				}
				continue
			}
		}
		_, synthetic := a.atlas.resolveRuneStyle(request, cell.Rune)
		duplicateBold, skew := styleDrawEffects(synthetic, a.cellH)
		a.atlas.drawRuneStyle(request, cell.Rune, x, y, fg, 1, skew)
		if duplicateBold {
			a.atlas.drawRuneStyle(request, cell.Rune, x+1, y, fg, 1, skew)
		}
		for _, combining := range cell.Combining() {
			_, combiningSynthetic := a.atlas.resolveRuneStyle(request, combining)
			combiningBold, combiningSkew := styleDrawEffects(combiningSynthetic, a.cellH)
			a.atlas.drawRuneStyle(request, combining, x, y, fg, 1, combiningSkew)
			if combiningBold {
				a.atlas.drawRuneStyle(request, combining, x+1, y, fg, 1, combiningSkew)
			}
		}
		a.drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
	}
	return order
}

func styleDrawEffects(synthetic fontdesc.SyntheticMode, cellH float32) (duplicateBold bool, skew float32) {
	duplicateBold = synthetic&fontdesc.SyntheticBold != 0
	if synthetic&fontdesc.SyntheticItalic != 0 {
		skew = 0.2 * cellH
	}
	return duplicateBold, skew
}

func renderSpanMatchesStyle(cells []core.Cell, start, span int, request fontdesc.RequestedFaceStyle) bool {
	if start < 0 || span < 1 || start+span > len(cells) {
		return false
	}
	for i := start; i < start+span; i++ {
		attr := cells[i].Attr
		if fontdesc.RequestedFaceStyleFromAttributes(attr.Bold, attr.Italic) != request {
			return false
		}
	}
	return true
}
