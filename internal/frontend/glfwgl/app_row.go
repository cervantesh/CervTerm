//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/core"
	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
)

func (a *App) drawRow(r int, background, selectionColor, defaultFG color.RGBA) []int {
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
		x := a.paddingX + float32(visualCol)*a.cellW
		y := a.paddingY + float32(r)*a.cellH
		if cell.Attr.BG != core.DefaultBG {
			fillRect(x, y, a.cellW, a.cellH, rgb(cell.Attr.BG))
		}
		if a.selectionActive && termsel.Contains(termsel.Range{Start: a.selectionStart, End: a.selectionEnd}, termsel.Point{Row: r, Col: logicalCol}) {
			fillRect(x, y, a.cellW, a.cellH, selectionColor)
		}
		if a.searchHasMatch && r == a.searchViewRow &&
			logicalCol >= a.searchMatchCol && logicalCol < a.searchMatchCol+a.searchMatchLen {
			fillRect(x, y, a.cellW, a.cellH, searchHighlightColor)
		}
		fg := defaultFG
		if cell.Attr.FG != core.DefaultFG {
			fg = rgb(cell.Attr.FG)
		}
		bg := background
		if cell.Attr.BG != core.DefaultBG {
			bg = rgb(cell.Attr.BG)
		}
		if cell.Attr.Inverse {
			fg, bg = bg, fg
			fillRect(x, y, a.cellW, a.cellH, bg)
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
		if rects, ok := render.BoxGlyph(cell.Rune, a.cellW, a.cellH); ok {
			for _, rc := range rects {
				c := fg
				if rc.Alpha < 1 {
					c.A = uint8(float32(fg.A) * rc.Alpha)
				}
				fillRect(x+rc.X, y+rc.Y, rc.W, rc.H, c)
			}
			drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
			continue
		}
		skew := float32(0)
		if cell.Attr.Italic {
			skew = 0.2 * a.cellH
		}
		// A ligature hit draws the whole span once (bold doubling + decorations
		// over the span) and marks the covered cells; a miss falls through to the
		// per-cell path. Per-cell backgrounds/selection already painted above.
		if ligate {
			if run, ok := detectLigatureRun(rowCells, logicalCol, cursorCol); ok {
				if a.drawRunGlyph(run.Text, run.CellSpan, x, y, fg, 1, skew) {
					if cell.Attr.Bold {
						a.drawRunGlyph(run.Text, run.CellSpan, x+1, y, fg, 1, skew)
					}
					drawTextDecorations(x, y, a.cellW*float32(run.CellSpan), a.cellH, fg, cell.Attr)
					for i := 1; i < run.CellSpan && logicalCol+i < a.snap.Cols; i++ {
						skippedGlyph[logicalCol+i] = true
					}
					continue
				}
			}
		}
		if cluster, ok := collectRenderCluster(a.snap.Cells, a.snap.Cols, r, logicalCol); ok {
			if a.drawCluster(cluster.Text, cluster.CellSpan, x, y, fg, 1, skew) {
				if cell.Attr.Bold {
					a.drawCluster(cluster.Text, cluster.CellSpan, x+1, y, fg, 1, skew)
				}
				drawTextDecorations(x, y, a.cellW*float32(cluster.CellSpan), a.cellH, fg, cell.Attr)
				for i := 1; i < cluster.CellSpan && logicalCol+i < a.snap.Cols; i++ {
					skippedGlyph[logicalCol+i] = true
				}
				continue
			}
		}
		a.drawRune(cell.Rune, x, y, fg, 1, skew)
		if cell.Attr.Bold {
			a.drawRune(cell.Rune, x+1, y, fg, 1, skew)
		}
		for _, combining := range cell.Combining() {
			a.drawRune(combining, x, y, fg, 1, skew)
			if cell.Attr.Bold {
				a.drawRune(combining, x+1, y, fg, 1, skew)
			}
		}
		drawTextDecorations(x, y, a.cellW, a.cellH, fg, cell.Attr)
	}
	return order
}
