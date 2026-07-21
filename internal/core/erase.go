package core

func (t *Terminal) ClearLine(row int) {
	if row < 0 || row >= t.rows {
		return
	}
	if t.imageSidecars != nil {
		t.eraseImageLiveRect(row, row+1, 0, t.cols)
	}
	blank := t.blank()
	start := row * t.cols
	for c := 0; c < t.cols; c++ {
		t.cells[start+c] = blank
	}
	t.rowWrapped[row] = false
}

func (t *Terminal) ClearToEndOfLine() {
	blank := t.blank()
	start := t.cursorRow*t.cols + t.cursorCol
	end := (t.cursorRow + 1) * t.cols
	if t.imageSidecars != nil {
		t.eraseImageLiveRect(t.cursorRow, t.cursorRow+1, t.cursorCol, t.cols)
	}
	for i := start; i < end; i++ {
		t.cells[i] = blank
	}
	t.rowWrapped[t.cursorRow] = false
}

func (t *Terminal) ClearToBeginningOfLine() {
	blank := t.blank()
	start := t.cursorRow * t.cols
	end := start + t.cursorCol
	if t.imageSidecars != nil {
		t.eraseImageLiveRect(t.cursorRow, t.cursorRow+1, 0, t.cursorCol+1)
	}
	for i := start; i <= end; i++ {
		t.cells[i] = blank
	}
	if t.cursorCol == t.cols-1 {
		t.rowWrapped[t.cursorRow] = false
	}
}

// EraseChars replaces n character positions at and after the cursor with blanks
// without shifting the remaining cells or moving the cursor (ECMA-48 ECH).
func (t *Terminal) EraseChars(n int) {
	if n <= 0 {
		n = 1
	}
	startCol := t.cursorCol
	endCol := min(t.cols, startCol+n)
	if startCol >= endCol {
		return
	}
	startCol, endCol = t.expandWideCellRange(t.cursorRow, startCol, endCol)
	if t.imageSidecars != nil {
		t.eraseImageLiveRect(t.cursorRow, t.cursorRow+1, startCol, endCol)
	}

	blank := Cell{Rune: ' ', Attr: t.attr}
	rowStart := t.cursorRow * t.cols
	for col := startCol; col < endCol; col++ {
		t.cells[rowStart+col] = blank
	}
	t.repairWideCells(t.cursorRow, blank)
	if endCol == t.cols {
		t.rowWrapped[t.cursorRow] = false
	}
	t.wrapNext = false
}

func (t *Terminal) ClearToEndOfScreen() {
	t.ClearToEndOfLine()
	for row := t.cursorRow + 1; row < t.rows; row++ {
		t.ClearLine(row)
	}
}

func (t *Terminal) ClearToBeginningOfScreen() {
	for row := 0; row < t.cursorRow; row++ {
		t.ClearLine(row)
	}
	t.ClearToBeginningOfLine()
}

func (t *Terminal) ClearScrollback() {
	if t.imageSidecars != nil {
		t.imagesClearScrollback()
	}
	t.scrollback = nil
	t.scrollbackWrapped = nil
	t.scrollbackStart = 0
	t.scrollbackRows = 0
	t.displayOffset = 0
}
