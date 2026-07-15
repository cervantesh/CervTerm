//go:build glfw

package glfwgl

import (
	"strings"

	"cervterm/internal/core"
	"cervterm/internal/unicodecluster"
)

const zeroWidthJoiner = unicodepropsCompatZeroWidthJoiner

type renderCluster struct {
	Text     string
	CellSpan int
}

func collectRenderCluster(cells []core.Cell, cols int, row int, col int) (renderCluster, bool) {
	if cols <= 0 || row < 0 || col < 0 || col >= cols {
		return renderCluster{}, false
	}
	idx := row*cols + col
	if idx < 0 || idx >= len(cells) {
		return renderCluster{}, false
	}
	cell := cells[idx]
	if cell.Rune == 0 || cell.Rune == ' ' || cell.WideContinuation {
		return renderCluster{}, false
	}

	shouldCluster := cell.HasCombining() || unicodecluster.ShouldShapeRune(cell.Rune)
	if !shouldCluster {
		return renderCluster{}, false
	}
	// A shapeable ASCII rune (digit, '#', '*') with no combining marks can't begin
	// a keycap or ZWJ sequence — those attach as width-0 combining marks, caught
	// by HasCombining above. So a bare ASCII digit is a lone glyph: render it via
	// the fast per-rune path instead of allocating a cluster string every frame
	// (this is the bulk of a number-heavy screen's per-frame churn).
	if !cell.HasCombining() && cell.Rune < 0x80 {
		return renderCluster{}, false
	}

	var b strings.Builder
	writeCellText(&b, cell)
	endCol := col + cellColumnSpan(cell)
	regionalIndicators := 0
	if unicodecluster.IsRegionalIndicator(cell.Rune) {
		regionalIndicators = 1
	}

	for endCol < cols {
		joined := unicodecluster.ContainsZeroWidthJoiner(cell.Combining())
		nextIdx := row*cols + endCol
		if nextIdx < 0 || nextIdx >= len(cells) {
			break
		}
		next := cells[nextIdx]
		if next.Rune == 0 || next.Rune == ' ' || next.WideContinuation {
			break
		}

		consumeRegionalPair := regionalIndicators == 1 && unicodecluster.IsRegionalIndicator(next.Rune)
		if !joined && !consumeRegionalPair {
			break
		}

		writeCellText(&b, next)
		endCol += cellColumnSpan(next)
		cell = next
		if unicodecluster.IsRegionalIndicator(next.Rune) {
			regionalIndicators++
		}
		if consumeRegionalPair {
			break
		}
	}
	text := b.String()
	cellSpan := max(max(1, endCol-col), unicodecluster.DisplayWidthString(text))
	return renderCluster{Text: text, CellSpan: cellSpan}, true
}

func writeCellText(b *strings.Builder, cell core.Cell) {
	b.WriteRune(cell.Rune)
	for _, r := range cell.Combining() {
		b.WriteRune(r)
	}
}

func cellColumnSpan(cell core.Cell) int {
	return max(1, core.RuneWidth(cell.Rune))
}

const unicodepropsCompatZeroWidthJoiner = '\u200d'
