package glfwgl

import "cervterm/internal/core"

const (
	ligatureMinRunCells = 2
	ligatureMaxRunCells = 8
)

// isLigatureRune reports whether r participates in programming ligatures. The
// alphabet is the symbol set that ligating fonts (Fira Code, Cascadia Code,
// JetBrains Mono) combine; letters and digits are excluded on purpose so
// ordinary text is never shaped as a run and runs stay short.
func isLigatureRune(r rune) bool {
	switch r {
	case '!', '#', '$', '%', '&', '*', '+', '-', '.', '/',
		':', ';', '<', '=', '>', '?', '@', '\\', '^', '_', '|', '~':
		return true
	}
	return false
}

type ligatureRun struct {
	Text     string
	CellSpan int
}

// detectLigatureRun returns the maximal ligature-candidate run beginning at col
// on the row's cells, or ok=false if col does not start one. A run is 2..8
// consecutive cells whose runes are all in the ligature alphabet, share
// identical render attrs (inverse included, so an inverse-mixed boundary splits
// the run), and carry no combining marks, wide glyphs, or continuation cells.
//
// cursorCol is the cursor's column on this row, or -1 when the cursor is
// elsewhere. A run covering the cursor cell is rejected so the user always sees
// the exact character under the cursor (§4); the row repaints on cursor move
// because cursor rows are always damaged.
func detectLigatureRun(cells []core.Cell, col, cursorCol int) (ligatureRun, bool) {
	if col < 0 || col >= len(cells) {
		return ligatureRun{}, false
	}
	first := cells[col]
	if !ligatureCandidateCell(first) {
		return ligatureRun{}, false
	}
	end := col + 1
	for end < len(cells) && end-col < ligatureMaxRunCells {
		next := cells[end]
		if !ligatureCandidateCell(next) || next.Attr != first.Attr {
			break
		}
		end++
	}
	span := end - col
	if span < ligatureMinRunCells {
		return ligatureRun{}, false
	}
	if cursorCol >= col && cursorCol < end {
		return ligatureRun{}, false
	}
	runes := make([]rune, 0, span)
	for i := col; i < end; i++ {
		runes = append(runes, cells[i].Rune)
	}
	return ligatureRun{Text: string(runes), CellSpan: span}, true
}

func ligatureCandidateCell(cell core.Cell) bool {
	if cell.WideContinuation || len(cell.Combining) > 0 {
		return false
	}
	if core.RuneWidth(cell.Rune) != 1 {
		return false
	}
	return isLigatureRune(cell.Rune)
}
