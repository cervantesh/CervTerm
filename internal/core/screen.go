package core

import "strings"

func (t *Terminal) CopyView(dst []Cell) {
	if len(dst) < len(t.cells) {
		return
	}
	if t.displayOffset == 0 || len(t.scrollback) == 0 {
		copy(dst, t.cells)
		return
	}

	scrollbackRows := t.ScrollbackLines()
	startRow := t.ViewportTopGlobalRow()
	for row := 0; row < t.rows; row++ {
		globalRow := startRow + row
		if globalRow < scrollbackRows {
			sourceRow := (t.scrollbackStart + globalRow) % t.scrollbackCapacity
			copy(dst[row*t.cols:(row+1)*t.cols], t.scrollback[sourceRow*t.cols:(sourceRow+1)*t.cols])
			continue
		}
		currentRow := globalRow - scrollbackRows
		copy(dst[row*t.cols:(row+1)*t.cols], t.cells[currentRow*t.cols:(currentRow+1)*t.cols])
	}
}

// ViewportTopGlobalRow is the global (physical-row) index of the first row
// visible in the viewport — the same startRow CopyView copies from. The viewport
// shows global rows [ViewportTopGlobalRow(), ViewportTopGlobalRow()+Rows()-1].
// Global indices match physicalRows and Resize: index 0 is the oldest scrollback
// row, scrollbackRows+rows-1 is the last live row. This is the one place the
// "scrollback minus display offset" arithmetic lives; search scroll-to-match and
// the draw highlight both derive their viewport rows from it instead of
// re-computing it by hand.
func (t *Terminal) ViewportTopGlobalRow() int {
	return t.scrollbackRows - t.displayOffset
}

// GlobalRowToViewport translates a global (physical-row) index to a 0-based
// viewport row, returning ok=false when the row falls outside the visible
// window. It is the inverse of ViewportTopGlobalRow.
func (t *Terminal) GlobalRowToViewport(g int) (row int, ok bool) {
	row = g - t.ViewportTopGlobalRow()
	if row < 0 || row >= t.rows {
		return 0, false
	}
	return row, true
}

// LineWrapped reports whether a row in the current viewport wraps into the
// next row. Row indices are 0-based.
func (t *Terminal) LineWrapped(row int) (bool, bool) {
	if row < 0 || row >= t.rows {
		return false, false
	}

	globalRow := t.ViewportTopGlobalRow() + row
	if globalRow < t.scrollbackRows {
		sourceRow := (t.scrollbackStart + globalRow) % t.scrollbackCapacity
		if len(t.scrollbackWrapped) != t.scrollbackCapacity {
			return false, true
		}
		return t.scrollbackWrapped[sourceRow], true
	}

	currentRow := globalRow - t.scrollbackRows
	if currentRow >= len(t.rowWrapped) {
		return false, true
	}
	return t.rowWrapped[currentRow], true
}

func (t *Terminal) PlainText() string {
	var b strings.Builder
	b.Grow(t.rows * (t.cols + 1))
	for r := 0; r < t.rows; r++ {
		start := r * t.cols
		b.WriteString(RowText(t.cells[start : start+t.cols]))
		if r != t.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (t *Terminal) blank() Cell {
	return Cell{Rune: ' ', Attr: Attr{FG: DefaultColor(), BG: DefaultColor()}}
}

func isBlankRow(row []Cell) bool {
	blankAttr := Attr{FG: DefaultColor(), BG: DefaultColor()}
	for _, cell := range row {
		if (cell.Rune != 0 && cell.Rune != ' ') || cell.HasCombining() || cell.Attr != blankAttr || cell.HyperlinkID != 0 || cell.SemanticKind != SemanticNone || cell.WideContinuation {
			return false
		}
	}
	return true
}

func (t *Terminal) physicalRows() ([][]Cell, []bool) {
	count := t.scrollbackRows + t.rows
	rows := make([][]Cell, 0, count)
	wrapped := make([]bool, 0, count)

	for row := 0; row < t.scrollbackRows; row++ {
		sourceRow := (t.scrollbackStart + row) % t.scrollbackCapacity
		start := sourceRow * t.cols
		rows = append(rows, cloneCellRow(t.scrollback[start:start+t.cols]))
		if len(t.scrollbackWrapped) == t.scrollbackCapacity {
			wrapped = append(wrapped, t.scrollbackWrapped[sourceRow])
		} else {
			wrapped = append(wrapped, false)
		}
	}
	for row := 0; row < t.rows; row++ {
		start := row * t.cols
		rows = append(rows, cloneCellRow(t.cells[start:start+t.cols]))
		wrapped = append(wrapped, row < len(t.rowWrapped) && t.rowWrapped[row])
	}

	return rows, wrapped
}

func (t *Terminal) snapshotScreen() *screenState {
	return &screenState{
		cols:                    t.cols,
		rows:                    t.rows,
		cells:                   cloneCellRow(t.cells),
		rowWrapped:              cloneBoolRow(t.rowWrapped),
		scrollback:              cloneCellRow(t.scrollback),
		scrollbackWrapped:       cloneBoolRow(t.scrollbackWrapped),
		scrollbackStart:         t.scrollbackStart,
		scrollbackRows:          t.scrollbackRows,
		scrollbackCapacity:      t.scrollbackCapacity,
		displayOffset:           t.displayOffset,
		cursorRow:               t.cursorRow,
		cursorCol:               t.cursorCol,
		wrapNext:                t.wrapNext,
		savedCursorRow:          t.savedCursorRow,
		savedCursorCol:          t.savedCursorCol,
		savedWrapNext:           t.savedWrapNext,
		hasSavedCursor:          t.hasSavedCursor,
		scrollTop:               t.scrollTop,
		scrollBottom:            t.scrollBottom,
		charsets:                t.charsets,
		activeCharset:           t.activeCharset,
		hyperlinks:              t.hyperlinks.clone(),
		semanticKind:            t.semanticKind,
		semanticBoundaryPending: t.semanticBoundaryPending,
	}
}

func (t *Terminal) restoreScreen(s *screenState) {
	t.cols = s.cols
	t.rows = s.rows
	t.cells = cloneCellRow(s.cells)
	t.rowWrapped = cloneBoolRow(s.rowWrapped)
	t.scrollback = cloneCellRow(s.scrollback)
	t.scrollbackWrapped = cloneBoolRow(s.scrollbackWrapped)
	t.scrollbackStart = s.scrollbackStart
	t.scrollbackRows = s.scrollbackRows
	t.scrollbackCapacity = s.scrollbackCapacity
	t.displayOffset = min(s.displayOffset, s.scrollbackRows)
	t.cursorRow = min(s.cursorRow, max(0, s.rows-1))
	t.cursorCol = min(s.cursorCol, max(0, s.cols-1))
	t.wrapNext = s.wrapNext
	t.savedCursorRow = s.savedCursorRow
	t.savedCursorCol = s.savedCursorCol
	t.savedWrapNext = s.savedWrapNext
	t.hasSavedCursor = s.hasSavedCursor
	t.scrollTop = min(s.scrollTop, max(0, t.rows-1))
	t.scrollBottom = min(s.scrollBottom, max(0, t.rows-1))
	t.charsets = s.charsets
	t.activeCharset = s.activeCharset
	t.hyperlinks = s.hyperlinks.clone()
	t.semanticKind = s.semanticKind
	t.semanticBoundaryPending = s.semanticBoundaryPending
	if t.scrollBottom <= t.scrollTop {
		t.resetScrollRegion()
	}
}

func cloneCellRow(row []Cell) []Cell {
	out := make([]Cell, len(row))
	copy(out, row)
	// Give each cell an independent combining pointer so the clone never aliases
	// the source's marks. (Appends copy-on-write, so this is belt-and-braces for
	// the alt-screen save path, which is not hot.)
	for i := range out {
		if marks := out[i].CloneCombining(); marks != nil {
			out[i].combining = &marks
		} else {
			out[i].combining = nil
		}
	}
	return out
}

func cloneBoolRow(row []bool) []bool {
	out := make([]bool, len(row))
	copy(out, row)
	return out
}

func isBlankCell(cell Cell) bool {
	return cell.HyperlinkID == 0 && cell.SemanticKind == SemanticNone && (cell.WideContinuation || cell.Rune == 0 || cell.Rune == ' ')
}

func writeCellText(b *strings.Builder, cell Cell) {
	if cell.WideContinuation || cell.Rune == 0 {
		return
	}
	b.WriteRune(cell.Rune)
	for _, r := range cell.Combining() {
		b.WriteRune(r)
	}
}

func trimmedCellRow(row []Cell) []Cell {
	last := len(row) - 1
	for last >= 0 && isBlankCell(row[last]) {
		last--
	}
	if last < 0 {
		return nil
	}
	out := make([]Cell, last+1)
	copy(out, row[:last+1])
	return out
}

// wrappedContentLen is the number of chars a WRAPPED physical row contributes to
// its logical line: its full width minus any trailing synthetic Rune==0 padding
// that a char-split boundary head left when padded to the ring width. Real
// alignment spaces (Rune==' ') are kept. logicalRowsFromPhysical, physicalAnchor
// and physicalForAnchor all use this so they agree on where chars fall — a
// mismatch shifts the history/live boundary by a row on a later resize.
func wrappedContentLen(row []Cell) int {
	n := len(row)
	for n > 0 && row[n-1].Rune == 0 && !row[n-1].WideContinuation {
		n--
	}
	return n
}

func logicalRowsFromPhysical(rows [][]Cell, wrappedRows []bool) [][]Cell {
	logicalRows := make([][]Cell, 0, len(rows))
	var current []Cell
	for i, row := range rows {
		wrapped := i < len(wrappedRows) && wrappedRows[i]
		if wrapped {
			// A wrapped row continues onto the next: keep its interior alignment
			// spaces but drop trailing char-split padding (see wrappedContentLen).
			current = append(current, row[:wrappedContentLen(row)]...)
			continue
		}
		// Last row of the logical line: its trailing blanks are display padding.
		current = append(current, trimmedCellRow(row)...)
		logicalRows = append(logicalRows, cloneCellRow(current))
		current = nil
	}
	if current != nil {
		logicalRows = append(logicalRows, current)
	}
	return logicalRows
}

func reflowLogicalRows(logicalRows [][]Cell, cols int) ([][]Cell, []bool) {
	physicalRows := make([][]Cell, 0, len(logicalRows))
	wrappedRows := make([]bool, 0, len(logicalRows))
	for _, row := range logicalRows {
		if len(row) == 0 {
			physicalRows = append(physicalRows, nil)
			wrappedRows = append(wrappedRows, false)
			continue
		}
		for len(row) > 0 {
			take := min(cols, len(row))
			chunk := make([]Cell, take)
			copy(chunk, row[:take])
			row = row[take:]
			physicalRows = append(physicalRows, chunk)
			wrappedRows = append(wrappedRows, len(row) > 0)
		}
	}
	return physicalRows, wrappedRows
}

// physicalAnchor maps a global physical row to the logical line index and the
// character offset within that line that begins it. Used to remember the scroll
// position before a reflow so it can be restored afterwards.
func physicalAnchor(physicalRows [][]Cell, wrappedRows []bool, target int) (line, char int) {
	logicalLine, accum := 0, 0
	for i := range physicalRows {
		if i == target {
			return logicalLine, accum
		}
		// A wrapped row contributes its full width to the logical line (its trailing
		// cells are interior alignment, kept by logicalRowsFromPhysical); a
		// non-wrapped row ends the line, so its char count is irrelevant (accum
		// resets). Counting the effective width keeps this consistent with the reflow.
		if i < len(wrappedRows) && wrappedRows[i] {
			accum += wrappedContentLen(physicalRows[i])
		} else {
			logicalLine++
			accum = 0
		}
	}
	return logicalLine, accum
}

// physicalForAnchor is the inverse of physicalAnchor: it returns the global
// physical row that renders the given logical line + character offset in a
// (possibly reflowed) layout. Clamps to the last row when the anchor is past
// the end.
func physicalForAnchor(physicalRows [][]Cell, wrappedRows []bool, line, char int) int {
	logicalLine, accum := 0, 0
	for i := range physicalRows {
		wrapped := i < len(wrappedRows) && wrappedRows[i]
		segLen := len(trimmedCellRow(physicalRows[i]))
		if wrapped {
			segLen = wrappedContentLen(physicalRows[i]) // consistent with physicalAnchor
		}
		if logicalLine == line && (char < accum+segLen || !wrapped) {
			return i
		}
		if logicalLine > line {
			return i
		}
		accum += segLen
		if !wrapped {
			logicalLine++
			accum = 0
		}
	}
	if len(physicalRows) == 0 {
		return 0
	}
	return len(physicalRows) - 1
}

// ScrollViewport shifts the visible window by lines (positive scrolls back into
// history), clamping to the available scrollback. It reports whether the display
// offset actually changed so callers can skip a redraw when a wheel tick lands
// on a clamp and moves nothing.
func (t *Terminal) ScrollViewport(lines int) bool {
	maxOffset := t.ScrollbackLines()
	prev := t.displayOffset
	t.displayOffset += lines
	if t.displayOffset < 0 {
		t.displayOffset = 0
	}
	if t.displayOffset > maxOffset {
		t.displayOffset = maxOffset
	}
	return t.displayOffset != prev
}

func paddedCellRow(row []Cell, cols int, blank Cell) []Cell {
	out := make([]Cell, cols)
	for i := range out {
		out[i] = blank
	}
	copy(out, row)
	return out
}

func (t *Terminal) scrollUp(lines int) {
	t.scrollUpRegion(0, t.rows-1, lines)
}

func (t *Terminal) resetScrollRegion() {
	t.scrollTop = 0
	t.scrollBottom = max(0, t.rows-1)
}

func (t *Terminal) scrollUpRegion(top, bottom, lines int) {
	if lines <= 0 {
		lines = 1
	}
	if top < 0 {
		top = 0
	}
	if bottom >= t.rows {
		bottom = t.rows - 1
	}
	if bottom < top {
		return
	}
	height := bottom - top + 1
	if lines > height {
		lines = height
	}
	fullScreen := top == 0 && bottom == t.rows-1
	if fullScreen {
		if t.alternateScreen && t.imageSidecars != nil {
			t.imagesScrollUp(top, bottom, lines)
		}
		for line := 0; line < lines; line++ {
			rowStart := line * t.cols
			t.appendScrollbackLine(t.cells[rowStart:rowStart+t.cols], len(t.rowWrapped) > line && t.rowWrapped[line])
		}
	} else if t.imageSidecars != nil {
		t.imagesScrollUp(top, bottom, lines)
	}
	start := top * t.cols
	copyStart := (top + lines) * t.cols
	end := (bottom + 1) * t.cols
	copy(t.cells[start:end-lines*t.cols], t.cells[copyStart:end])
	copy(t.rowWrapped[top:bottom-lines+1], t.rowWrapped[top+lines:bottom+1])
	blankStart := (bottom - lines + 1) * t.cols
	t.fillBlank(t.cells[blankStart:end])
	for row := bottom - lines + 1; row <= bottom; row++ {
		t.rowWrapped[row] = false
	}
}

func (t *Terminal) scrollDownRegion(top, bottom, lines int) {
	if lines <= 0 {
		lines = 1
	}
	if top < 0 {
		top = 0
	}
	if bottom >= t.rows {
		bottom = t.rows - 1
	}
	if bottom < top {
		return
	}
	height := bottom - top + 1
	if lines > height {
		lines = height
	}
	if t.imageSidecars != nil {
		t.imagesScrollDown(top, bottom, lines)
	}
	start := top * t.cols
	copyEnd := (bottom - lines + 1) * t.cols
	end := (bottom + 1) * t.cols
	copy(t.cells[start+lines*t.cols:end], t.cells[start:copyEnd])
	copy(t.rowWrapped[top+lines:bottom+1], t.rowWrapped[top:bottom-lines+1])
	blankEnd := (top + lines) * t.cols
	t.fillBlank(t.cells[start:blankEnd])
	for row := top; row < top+lines; row++ {
		t.rowWrapped[row] = false
	}
}

func (t *Terminal) appendScrollbackLine(line []Cell, wrapped bool) {
	if t.alternateScreen {
		t.displayOffset = 0
		return
	}
	if t.scrollbackCapacity == 0 {
		if t.imageSidecars != nil {
			t.dropPrimaryImageRows(1)
		}
		t.displayOffset = 0
		return
	}
	if len(t.scrollback) != t.scrollbackCapacity*t.cols {
		t.scrollback = make([]Cell, t.scrollbackCapacity*t.cols)
		t.scrollbackWrapped = make([]bool, t.scrollbackCapacity)
		t.scrollbackStart = 0
		t.scrollbackRows = 0
	}

	writeRow := (t.scrollbackStart + t.scrollbackRows) % t.scrollbackCapacity
	if t.scrollbackRows == t.scrollbackCapacity {
		if t.imageSidecars != nil {
			t.dropPrimaryImageRows(1)
		}
		writeRow = t.scrollbackStart
		t.scrollbackStart = (t.scrollbackStart + 1) % t.scrollbackCapacity
	} else {
		t.scrollbackRows++
	}
	copy(t.scrollback[writeRow*t.cols:(writeRow+1)*t.cols], line)
	t.scrollbackWrapped[writeRow] = wrapped

	// Pin the viewport to the same content while the user is scrolled back: a new
	// line shifts every visible row one step toward the live edge, so advance the
	// offset in lockstep to hold the view still (matching xterm). displayOffset ==
	// 0 (live tail) is left untouched so ordinary output still auto-follows. The
	// clamp below keeps it within the scrollback depth; at the ring's capacity the
	// oldest line the user was viewing is evicted, and the clamp lets the view
	// drift by exactly that one line, which is unavoidable.
	if t.displayOffset > 0 {
		t.displayOffset++
	}
	if t.displayOffset > t.ScrollbackLines() {
		t.displayOffset = t.ScrollbackLines()
	}
}
