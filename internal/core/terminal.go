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
	if t.alternateScreen {
		t.resizeAlt(cols, rows)
		return
	}
	t.resizePrimary(cols, rows)
}

// resizePrimary reflows the primary screen. It reflows the COMBINED stream
// (history + live screen) so wrapped logical lines rejoin at any width — the
// scrollback ring never stores a permanent cut, so lines that wrapped across the
// history/live boundary heal on a later widen instead of staying shredded.
//
// The history/live split is then made at a logical boundary anchor (where the
// shell's screen began), NOT at "the bottom `rows` rows": that is what keeps a
// grow from pulling history into the viewport (where ConPTY's repaint would
// overwrite it → the loss bug this whole path fixes). When a logical line
// straddles that boundary, the straddling row is split at the exact char so
// history cells never enter the viewport and live cells never freeze into
// history (→ duplication).
func (t *Terminal) resizePrimary(cols, rows int) {
	oldCols, oldOffset := t.cols, t.displayOffset
	sbCount := t.scrollbackRows

	combined, combinedW := t.physicalRows() // history + live, real wrap flags (no cut)

	// Cursor anchor within the LIVE group (the shell owns it): logical line+char
	// measured from the top of the live screen, so it survives the reflow.
	livePre, livePreW := combined[sbCount:], combinedW[sbCount:]
	cLine, cStart := physicalAnchor(livePre, livePreW, t.cursorRow)
	cChar := cStart + t.cursorCol

	// Boundary anchor (where the live screen begins) and, if scrolled up, the
	// viewport-top anchor — in COMBINED logical coordinates.
	boundLine, boundChar := physicalAnchor(combined, combinedW, sbCount)
	anchored := oldOffset > 0
	var topLine, topChar int
	if anchored {
		topLine, topChar = physicalAnchor(combined, combinedW, sbCount-oldOffset)
	}

	reflowed, reflowedW := combined, combinedW
	if cols != oldCols {
		reflowed, reflowedW = reflowLogicalRows(logicalRowsFromPhysical(combined, combinedW), cols)
	}

	// Split reflowed into history / live at the boundary anchor.
	b := physicalForAnchor(reflowed, reflowedW, boundLine, boundChar)
	sb := append([][]Cell(nil), reflowed[:b]...)
	sbW := append([]bool(nil), reflowedW[:b]...)
	live := append([][]Cell(nil), reflowed[b:]...)
	liveW := append([]bool(nil), reflowedW[b:]...)
	straddle := false
	if b < len(reflowed) {
		if bLine, bStart := physicalAnchor(reflowed, reflowedW, b); bLine == boundLine && bStart < boundChar {
			// A line straddles the boundary: split row b at the char. The head goes to
			// history, kept wrapped so it rejoins the tail on a later reflow, and padded
			// with ZERO cells (Rune==0) so its ring-width padding is dropped on re-read
			// instead of spliced mid-word (real alignment spaces are Rune==' ').
			straddle = true
			head, tail := splitRowAt(reflowed[b], boundChar-bStart)
			headFull := make([]Cell, cols)
			copy(headFull, head)
			sb = append(sb, headFull)
			sbW = append(sbW, true)
			live[0] = tail
		}
	}
	if straddle {
		// Re-reflow the live group so the short tail merges into clean rows: a short
		// wrapped grid row would otherwise splice its space padding mid-word on the
		// next reflow. Only on a straddle — otherwise live is already clean chunks and
		// re-reflowing would drop edge-case flags (e.g. a blank wrapped row).
		live, liveW = reflowLogicalRows(logicalRowsFromPhysical(live, liveW), cols)
	}

	// Map the cursor anchor into the (re-reflowed) live group.
	curRow := physicalForAnchor(live, liveW, cLine, cChar)
	_, curRowStart := physicalAnchor(live, liveW, curRow)
	curCol := cChar - curRowStart
	if curRow < 0 {
		curRow = 0
	}
	if curCol < 0 {
		curCol = 0
	}

	// Drop trailing all-blank rows below the cursor so they don't spill to history.
	keep := curRow + 1
	for len(live) > keep && !liveW[len(liveW)-1] && isBlankRow(live[len(live)-1]) {
		live = live[:len(live)-1]
		liveW = liveW[:len(liveW)-1]
	}

	// Shrink: live content that no longer fits spills into history, top-first,
	// keeping its natural wrap flags so the lines heal on a later widen. Grow:
	// push <= 0, nothing moves, the live group stays top-anchored.
	if push := len(live) - rows; push > 0 {
		sb = append(sb, live[:push]...)
		sbW = append(sbW, liveW[:push]...)
		live, liveW = live[push:], liveW[push:]
		curRow -= push
	}

	t.rebuildScreen(cols, rows, sb, sbW, live, liveW)

	t.cursorRow = max(0, min(rows-1, curRow))
	t.cursorCol = max(0, min(cols-1, curCol))
	t.wrapNext = false
	t.resetScrollRegion()
	t.resizeTabStops(oldCols, cols)

	if anchored {
		newTop := physicalForAnchor(concatRows(sb, live), concatBools(sbW, liveW), topLine, topChar)
		t.displayOffset = max(0, min(len(sb)-newTop, t.ScrollbackLines()))
	} else {
		t.displayOffset = 0
	}
}

// resizeAlt resizes the alternate screen: a top-anchored crop/extend with no
// reflow and no scrollback (the alt screen has none). Full-screen apps (vim,
// less) repaint after the resize, so preserving the exact old cells matters less
// than never fabricating scrollback here — which the old shared path did.
func (t *Terminal) resizeAlt(cols, rows int) {
	oldCols, oldRows := t.cols, t.rows
	oldCells, oldWrapped := t.cells, t.rowWrapped
	t.cols, t.rows = cols, rows
	t.cells = make([]Cell, cols*rows)
	t.rowWrapped = make([]bool, rows)
	t.fillBlank(t.cells)

	copyRows, copyCols := min(rows, oldRows), min(cols, oldCols)
	for r := 0; r < copyRows; r++ {
		copy(t.cells[r*cols:r*cols+copyCols], oldCells[r*oldCols:r*oldCols+copyCols])
		if copyCols == oldCols && r < len(oldWrapped) {
			t.rowWrapped[r] = oldWrapped[r]
		}
	}
	t.cursorRow = min(t.cursorRow, rows-1)
	t.cursorCol = min(t.cursorCol, cols-1)
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
