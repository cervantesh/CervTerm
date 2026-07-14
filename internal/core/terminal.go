package core

func NewTerminal(cols, rows int) *Terminal {
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	t := &Terminal{cols: cols, rows: rows, attr: Attr{FG: DefaultFG, BG: DefaultBG}, scrollBottom: rows - 1, cursorVisible: true, autoWrap: true}
	t.cells = make([]Cell, cols*rows)
	t.rowWrapped = make([]bool, rows)
	t.resetTabStops()
	t.Clear()
	return t
}

func (t *Terminal) Cols() int         { return t.cols }
func (t *Terminal) Rows() int         { return t.rows }
func (t *Terminal) CursorRow() int    { return t.cursorRow }
func (t *Terminal) CursorCol() int    { return t.cursorCol }
func (t *Terminal) Title() string     { return t.title }
func (t *Terminal) SetTitle(s string) { t.title = s }

// BellCount reports how many BEL controls have executed. It is monotonic for the
// terminal's lifetime (unaffected by Reset) so observers can detect new bells by
// watching for increases.
func (t *Terminal) BellCount() int { return t.bellCount }
func (t *Terminal) Bell()          { t.bellCount++ }

func (t *Terminal) Cells() []Cell { return t.cells }

func (t *Terminal) Resize(cols, rows int) {
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	if cols == t.cols && rows == t.rows {
		return
	}
	oldCols := t.cols
	oldCursorGlobal := t.scrollbackRows + t.cursorRow
	oldDisplayOffset := t.displayOffset
	physicalRows, wrappedRows := t.physicalRows()

	// Width change while scrolled up: remember the logical anchor at the viewport
	// top so reflow doesn't make the content jump. At the bottom (offset 0) the
	// viewport keeps following the prompt, so it is not anchored.
	anchor := cols != oldCols && oldDisplayOffset > 0
	anchorLine, anchorChar := 0, 0
	if anchor {
		anchorLine, anchorChar = physicalAnchor(physicalRows, wrappedRows, t.scrollbackRows-oldDisplayOffset)
	}

	if cols != oldCols {
		logicalRows := logicalRowsFromPhysical(physicalRows, wrappedRows)
		physicalRows, wrappedRows = reflowLogicalRows(logicalRows, cols)
	}
	if !t.alternateScreen {
		// Trailing all-blank rows below the cursor are dropped rather than letting
		// them force content into scrollback; only real content scrolls to history.
		keep := min(oldCursorGlobal+1, len(physicalRows))
		for len(physicalRows) > keep && !wrappedRows[len(wrappedRows)-1] && isBlankRow(physicalRows[len(physicalRows)-1]) {
			physicalRows = physicalRows[:len(physicalRows)-1]
			wrappedRows = wrappedRows[:len(wrappedRows)-1]
		}
	}
	visibleStart := max(0, len(physicalRows)-rows)
	newDisplayOffset := oldDisplayOffset
	if anchor {
		newDisplayOffset = anchoredDisplayOffset(physicalRows, wrappedRows, anchorLine, anchorChar, rows)
	}
	t.rebuildFromPhysicalRows(cols, rows, physicalRows, wrappedRows, newDisplayOffset)

	if cols == oldCols {
		t.cursorRow = max(0, min(rows-1, oldCursorGlobal-visibleStart))
	}
	if t.cursorCol >= cols {
		t.cursorCol = cols - 1
	}
	t.wrapNext = false
	t.resetScrollRegion()
	t.resizeTabStops(oldCols, cols)
}

func (t *Terminal) Clear() {
	blank := t.blank()
	for i := range t.cells {
		t.cells[i] = blank
	}
	if len(t.rowWrapped) != t.rows {
		t.rowWrapped = make([]bool, t.rows)
	} else {
		for i := range t.rowWrapped {
			t.rowWrapped[i] = false
		}
	}
	t.cursorRow, t.cursorCol, t.wrapNext = 0, 0, false
}

func (t *Terminal) Reset() {
	t.Clear()
	t.ResetAttr()
	t.ResetScrollRegion()
	t.SetCursorVisible(true)
	t.SetAutoWrapMode(true)
	t.SetBracketedPasteMode(false)
	t.SetApplicationCursorMode(false)
	t.SetApplicationKeypadMode(false)
	t.SetMouseMode(1000, false)
	t.SetMouseMode(1002, false)
	t.SetMouseMode(1006, false)
	t.SetMouseMode(1003, false)
	t.SetMouseMode(1004, false)
	t.SetOriginMode(false)
	t.SetInsertMode(false)
	t.DesignateCharset(0, CharsetASCII)
	t.DesignateCharset(1, CharsetASCII)
	t.SelectCharset(0)
	t.SetCursorStyle(0)
	t.resetTabStops()
	t.SetCwd("")
}

func (t *Terminal) ClearLine(row int) {
	if row < 0 || row >= t.rows {
		return
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
	for i := start; i < end; i++ {
		t.cells[i] = blank
	}
	t.rowWrapped[t.cursorRow] = false
}

func (t *Terminal) ClearToBeginningOfLine() {
	blank := t.blank()
	start := t.cursorRow * t.cols
	end := start + t.cursorCol
	for i := start; i <= end; i++ {
		t.cells[i] = blank
	}
	if t.cursorCol == t.cols-1 {
		t.rowWrapped[t.cursorRow] = false
	}
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
	t.scrollback = nil
	t.scrollbackWrapped = nil
	t.scrollbackStart = 0
	t.scrollbackRows = 0
	t.displayOffset = 0
}

func (t *Terminal) PutRune(r rune) {
	if r == 0 || r == '\uFFFD' {
		return
	}
	r = t.translateCharset(r)
	width := RuneWidth(r)
	if width == 0 {
		t.addCombiningRune(r)
		return
	}
	if t.wrapNext && t.autoWrap {
		t.rowWrapped[t.cursorRow] = true
		t.cursorCol = 0
		t.NewLine()
		t.wrapNext = false
	}
	if width == 2 && t.cursorCol == t.cols-1 && t.autoWrap {
		t.rowWrapped[t.cursorRow] = true
		t.cursorCol = 0
		t.NewLine()
	}

	idx := t.cursorRow*t.cols + t.cursorCol
	if t.insertMode {
		t.InsertChars(width)
	}
	t.cells[idx] = Cell{Rune: r, Attr: t.attr}
	if width == 2 && t.cursorCol+1 < t.cols {
		t.cells[idx+1] = Cell{Attr: t.attr, WideContinuation: true}
	}

	if t.cursorCol+width >= t.cols {
		t.wrapNext = t.autoWrap
		t.cursorCol = t.cols - 1
		return
	}
	t.cursorCol += width
}

func (t *Terminal) addCombiningRune(r rune) {
	row, col := t.cursorRow, t.cursorCol-1
	if col < 0 {
		if row == 0 {
			return
		}
		row--
		col = t.cols - 1
	}
	idx := row*t.cols + col
	if t.cells[idx].WideContinuation && col > 0 {
		idx--
	}
	if t.cells[idx].Rune == 0 || t.cells[idx].Rune == ' ' {
		return
	}
	t.cells[idx].AppendCombining(r)
}

func (t *Terminal) NewLine() {
	t.wrapNext = false
	if t.cursorRow == t.scrollBottom {
		t.scrollUpRegion(t.scrollTop, t.scrollBottom, 1)
		return
	}
	if t.cursorRow < t.rows-1 {
		t.cursorRow++
	}
	t.wrapNext = false
}

func (t *Terminal) CarriageReturn() { t.cursorCol, t.wrapNext = 0, false }

func (t *Terminal) Backspace() {
	if t.cursorCol > 0 {
		t.cursorCol--
	}
	t.wrapNext = false
}

func (t *Terminal) Tab() {
	next := t.cols - 1
	for col := t.cursorCol + 1; col < t.cols; col++ {
		if col < len(t.tabStops) && t.tabStops[col] {
			next = col
			break
		}
	}
	t.cursorCol = next
	t.wrapNext = false
}

func (t *Terminal) MoveCursor(rowDelta, colDelta int) {
	t.SetCursor(t.cursorRow+rowDelta, t.cursorCol+colDelta)
}

func (t *Terminal) SetCursor(row, col int) {
	if t.originMode {
		row += t.scrollTop
		if row < t.scrollTop {
			row = t.scrollTop
		}
		if row > t.scrollBottom {
			row = t.scrollBottom
		}
	}
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if row >= t.rows {
		row = t.rows - 1
	}
	if col >= t.cols {
		col = t.cols - 1
	}
	t.cursorRow, t.cursorCol = row, col
	t.wrapNext = false
}

func (t *Terminal) SetScrollRegion(top, bottom int) {
	if top < 0 {
		top = 0
	}
	if bottom >= t.rows {
		bottom = t.rows - 1
	}
	if bottom <= top {
		t.resetScrollRegion()
		t.SetCursor(0, 0)
		return
	}
	t.scrollTop = top
	t.scrollBottom = bottom
	if t.originMode {
		t.SetCursor(0, 0)
		return
	}
	t.SetCursor(0, 0)
}

func (t *Terminal) ResetScrollRegion() {
	t.resetScrollRegion()
	t.SetCursor(0, 0)
}

func (t *Terminal) ScrollRegion() (int, int) { return t.scrollTop, t.scrollBottom }

func (t *Terminal) InsertChars(n int) {
	if n <= 0 {
		n = 1
	}
	if n > t.cols-t.cursorCol {
		n = t.cols - t.cursorCol
	}
	if n <= 0 {
		return
	}
	rowStart := t.cursorRow * t.cols
	start := rowStart + t.cursorCol
	end := rowStart + t.cols
	copy(t.cells[start+n:end], t.cells[start:end-n])
	blank := t.blank()
	for i := start; i < start+n; i++ {
		t.cells[i] = blank
	}
	t.rowWrapped[t.cursorRow] = false
	t.wrapNext = false
}

func (t *Terminal) DeleteChars(n int) {
	if n <= 0 {
		n = 1
	}
	if n > t.cols-t.cursorCol {
		n = t.cols - t.cursorCol
	}
	if n <= 0 {
		return
	}
	rowStart := t.cursorRow * t.cols
	start := rowStart + t.cursorCol
	end := rowStart + t.cols
	copy(t.cells[start:end-n], t.cells[start+n:end])
	blank := t.blank()
	for i := end - n; i < end; i++ {
		t.cells[i] = blank
	}
	t.rowWrapped[t.cursorRow] = false
	t.wrapNext = false
}

func (t *Terminal) InsertLines(n int) {
	if n <= 0 {
		n = 1
	}
	if t.cursorRow < t.scrollTop || t.cursorRow > t.scrollBottom {
		return
	}
	bottom := t.scrollBottom
	if n > bottom-t.cursorRow+1 {
		n = bottom - t.cursorRow + 1
	}
	t.scrollDownRegion(t.cursorRow, bottom, n)
	t.wrapNext = false
}

func (t *Terminal) DeleteLines(n int) {
	if n <= 0 {
		n = 1
	}
	if t.cursorRow < t.scrollTop || t.cursorRow > t.scrollBottom {
		return
	}
	bottom := t.scrollBottom
	if n > bottom-t.cursorRow+1 {
		n = bottom - t.cursorRow + 1
	}
	t.scrollUpRegion(t.cursorRow, bottom, n)
	t.wrapNext = false
}

func (t *Terminal) ScrollUp(lines int) {
	if lines <= 0 {
		lines = 1
	}
	t.scrollUpRegion(t.scrollTop, t.scrollBottom, lines)
}

func (t *Terminal) ScrollDown(lines int) {
	if lines <= 0 {
		lines = 1
	}
	t.scrollDownRegion(t.scrollTop, t.scrollBottom, lines)
}

func (t *Terminal) SaveCursor() {
	t.savedCursorRow = t.cursorRow
	t.savedCursorCol = t.cursorCol
	t.savedWrapNext = t.wrapNext
	t.hasSavedCursor = true
}

func (t *Terminal) RestoreCursor() {
	if !t.hasSavedCursor {
		t.SetCursor(0, 0)
		return
	}
	t.SetCursor(t.savedCursorRow, t.savedCursorCol)
	t.wrapNext = t.savedWrapNext
}

func (t *Terminal) SetAlternateScreenMode(enabled bool) {
	t.SetAlternateScreenModeWithOptions(enabled, true, true, false)
}

func (t *Terminal) SetAlternateScreenModeWithOptions(enabled, saveCursor, clearOnEnter, clearOnExit bool) {
	if enabled == t.alternateScreen {
		return
	}
	if enabled {
		if saveCursor {
			t.SaveCursor()
		}
		t.primaryScreen = t.snapshotScreen()
		t.alternateScreen = true
		t.cells = make([]Cell, t.cols*t.rows)
		t.rowWrapped = make([]bool, t.rows)
		t.scrollback = nil
		t.scrollbackWrapped = nil
		t.scrollbackStart = 0
		t.scrollbackRows = 0
		t.displayOffset = 0
		if clearOnEnter {
			t.Clear()
		} else {
			t.fillBlank(t.cells)
			t.cursorRow, t.cursorCol, t.wrapNext = 0, 0, false
		}
		t.resetScrollRegion()
		return
	}

	cols, rows := t.cols, t.rows
	primary := t.primaryScreen
	t.alternateScreen = false
	t.primaryScreen = nil
	if primary == nil {
		t.Clear()
		t.resetScrollRegion()
		return
	}
	if clearOnExit {
		t.fillBlank(t.cells)
	}
	t.restoreScreen(primary)
	if t.cols != cols || t.rows != rows {
		t.Resize(cols, rows)
	}
	if saveCursor {
		t.RestoreCursor()
	}
}
func (t *Terminal) ScrollbackLines() int { return t.scrollbackRows }
func (t *Terminal) DisplayOffset() int   { return t.displayOffset }
