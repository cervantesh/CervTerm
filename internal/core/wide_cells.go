package core

// expandWideCellRange expands the half-open column range when either edge
// intersects a lead/continuation pair. Callers must keep the returned range on
// this row and clear every returned cell before writing replacement content.
func (t *Terminal) expandWideCellRange(row, startCol, endCol int) (int, int) {
	if startCol > 0 && t.widePairAt(row, startCol-1) {
		startCol--
	}
	if endCol < t.cols && t.widePairAt(row, endCol-1) {
		endCol++
	}
	return startCol, endCol
}

func (t *Terminal) widePairAt(row, leadCol int) bool {
	if leadCol < 0 || leadCol+1 >= t.cols {
		return false
	}
	idx := row*t.cols + leadCol
	return !t.cells[idx].WideContinuation && RuneWidth(t.cells[idx].Rune) == 2 && t.cells[idx+1].WideContinuation
}

func (t *Terminal) repairWideCells(row int, blank Cell) {
	rowStart := row * t.cols
	for col := 0; col < t.cols; col++ {
		idx := rowStart + col
		if t.cells[idx].WideContinuation {
			if col == 0 || !t.widePairAt(row, col-1) {
				t.cells[idx] = blank
			}
			continue
		}
		if RuneWidth(t.cells[idx].Rune) == 2 && !t.widePairAt(row, col) {
			t.cells[idx] = blank
		}
	}
}
