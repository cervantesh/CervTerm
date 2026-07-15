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
	totalRows := scrollbackRows + t.rows
	startRow := totalRows - t.rows - t.displayOffset
	for row := 0; row < t.rows; row++ {
		globalRow := startRow + row
		if globalRow < scrollbackRows {
			sourceRow := (t.scrollbackStart + globalRow) % maxScrollbackRows
			copy(dst[row*t.cols:(row+1)*t.cols], t.scrollback[sourceRow*t.cols:(sourceRow+1)*t.cols])
			continue
		}
		currentRow := globalRow - scrollbackRows
		copy(dst[row*t.cols:(row+1)*t.cols], t.cells[currentRow*t.cols:(currentRow+1)*t.cols])
	}
}

// LineWrapped reports whether a row in the current viewport wraps into the
// next row. Row indices are 0-based.
func (t *Terminal) LineWrapped(row int) (bool, bool) {
	if row < 0 || row >= t.rows {
		return false, false
	}

	globalRow := t.scrollbackRows - t.displayOffset + row
	if globalRow < t.scrollbackRows {
		sourceRow := (t.scrollbackStart + globalRow) % maxScrollbackRows
		if len(t.scrollbackWrapped) != maxScrollbackRows {
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
		end := start + t.cols
		last := end - 1
		for last >= start && isBlankCell(t.cells[last]) {
			last--
		}
		for i := start; i <= last; i++ {
			writeCellText(&b, t.cells[i])
		}
		if r != t.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (t *Terminal) blank() Cell { return Cell{Rune: ' ', Attr: Attr{FG: DefaultFG, BG: DefaultBG}} }

func isBlankRow(row []Cell) bool {
	blankAttr := Attr{FG: DefaultFG, BG: DefaultBG}
	for _, cell := range row {
		if (cell.Rune != 0 && cell.Rune != ' ') || cell.HasCombining() || cell.Attr != blankAttr || cell.WideContinuation {
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
		sourceRow := (t.scrollbackStart + row) % maxScrollbackRows
		start := sourceRow * t.cols
		rows = append(rows, cloneCellRow(t.scrollback[start:start+t.cols]))
		if len(t.scrollbackWrapped) == maxScrollbackRows {
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

// scrollbackPhysicalRows returns only the frozen history rows. The wrapped flag
// of the LAST row is forced false so the scrollback↔screen seam is cut: reflow
// then treats history and live screen as independent groups and never moves a
// wrapped logical line across the seam. Moving it either way is wrong under
// ConPTY — pulling the prefix into the viewport reintroduces the loss bug, and
// pushing the live tail into history duplicates it when ConPTY repaints the
// viewport — so we cut instead. Cost: a line that happened to wrap across the
// seam at resize time is split into two logical lines (not rejoined).
func (t *Terminal) scrollbackPhysicalRows() ([][]Cell, []bool) {
	rows := make([][]Cell, 0, t.scrollbackRows)
	wrapped := make([]bool, 0, t.scrollbackRows)
	for row := 0; row < t.scrollbackRows; row++ {
		sourceRow := (t.scrollbackStart + row) % maxScrollbackRows
		start := sourceRow * t.cols
		rows = append(rows, cloneCellRow(t.scrollback[start:start+t.cols]))
		if len(t.scrollbackWrapped) == maxScrollbackRows {
			wrapped = append(wrapped, t.scrollbackWrapped[sourceRow])
		} else {
			wrapped = append(wrapped, false)
		}
	}
	if n := len(wrapped); n > 0 {
		wrapped[n-1] = false // cut the seam
	}
	return rows, wrapped
}

// screenPhysicalRows returns only the live screen rows (t.cells) with their wrap
// flags — the shell-owned region that ConPTY repaints on resize.
func (t *Terminal) screenPhysicalRows() ([][]Cell, []bool) {
	rows := make([][]Cell, 0, t.rows)
	wrapped := make([]bool, 0, t.rows)
	for row := 0; row < t.rows; row++ {
		start := row * t.cols
		rows = append(rows, cloneCellRow(t.cells[start:start+t.cols]))
		wrapped = append(wrapped, row < len(t.rowWrapped) && t.rowWrapped[row])
	}
	return rows, wrapped
}

// reflowGroup rewraps one group (history or live screen) to a new width.
func reflowGroup(rows [][]Cell, wrapped []bool, cols int) ([][]Cell, []bool) {
	return reflowLogicalRows(logicalRowsFromPhysical(rows, wrapped), cols)
}

func concatRows(a, b [][]Cell) [][]Cell {
	out := make([][]Cell, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

func concatBools(a, b []bool) []bool {
	out := make([]bool, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

// rebuildScreen rebuilds the scrollback ring and the live grid from the two
// reflowed groups. Precondition: len(live) <= rows. The grid is TOP-anchored
// (live[0] -> row 0; any extra rows stay blank) so ConPTY's post-resize viewport
// repaint lands on the same rows we already show — no visual jump, no duplicated
// lines. Unlike the old rebuildFromPhysicalRows it does not decide the cursor or
// how much content spills to scrollback; resizePrimary owns that.
func (t *Terminal) rebuildScreen(cols, rows int, sb [][]Cell, sbW []bool, live [][]Cell, liveW []bool) {
	t.cols, t.rows = cols, rows
	t.cells = make([]Cell, cols*rows)
	t.rowWrapped = make([]bool, rows)
	t.fillBlank(t.cells)
	t.scrollback = nil
	t.scrollbackWrapped = nil
	t.scrollbackStart = 0
	t.scrollbackRows = 0
	t.displayOffset = 0

	blank := t.blank()
	for i := range sb {
		t.appendScrollbackLine(paddedCellRow(sb[i], cols, blank), i < len(sbW) && sbW[i])
	}
	for i := 0; i < len(live) && i < rows; i++ {
		copy(t.cells[i*cols:(i+1)*cols], paddedCellRow(live[i], cols, blank))
		t.rowWrapped[i] = i < len(liveW) && liveW[i]
	}
}

func (t *Terminal) snapshotScreen() *screenState {
	return &screenState{
		cols:              t.cols,
		rows:              t.rows,
		cells:             cloneCellRow(t.cells),
		rowWrapped:        cloneBoolRow(t.rowWrapped),
		scrollback:        cloneCellRow(t.scrollback),
		scrollbackWrapped: cloneBoolRow(t.scrollbackWrapped),
		scrollbackStart:   t.scrollbackStart,
		scrollbackRows:    t.scrollbackRows,
		displayOffset:     t.displayOffset,
		cursorRow:         t.cursorRow,
		cursorCol:         t.cursorCol,
		wrapNext:          t.wrapNext,
		savedCursorRow:    t.savedCursorRow,
		savedCursorCol:    t.savedCursorCol,
		savedWrapNext:     t.savedWrapNext,
		hasSavedCursor:    t.hasSavedCursor,
		scrollTop:         t.scrollTop,
		scrollBottom:      t.scrollBottom,
		charsets:          t.charsets,
		activeCharset:     t.activeCharset,
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
	if t.scrollBottom <= t.scrollTop {
		t.resetScrollRegion()
	}
}

func (t *Terminal) fillBlank(cells []Cell) {
	blank := t.blank()
	for i := range cells {
		cells[i] = blank
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
	return cell.WideContinuation || cell.Rune == 0 || cell.Rune == ' '
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

func logicalRowsFromPhysical(rows [][]Cell, wrappedRows []bool) [][]Cell {
	logicalRows := make([][]Cell, 0, len(rows))
	var current []Cell
	for i, row := range rows {
		wrapped := i < len(wrappedRows) && wrappedRows[i]
		if wrapped {
			// A wrapped row continues onto the next one, so its trailing spaces are
			// interior alignment of the logical line (e.g. columns in `dir`/`ls`
			// output), not padding. Keep the row in full — trimming here collapsed
			// those runs every time content was rewrapped through a narrow width.
			current = append(current, row...)
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
		accum += len(trimmedCellRow(physicalRows[i]))
		if i >= len(wrappedRows) || !wrappedRows[i] {
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
		segLen := len(trimmedCellRow(physicalRows[i]))
		wrapped := i < len(wrappedRows) && wrappedRows[i]
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

// cursorForAnchor maps a logical (line, char) anchor to a physical row/column in
// a (reflowed) layout — the inverse of physicalAnchor for the cursor. The column
// is clamped to the row width; ConPTY repositions the cursor after a resize, but
// core stays sensible standalone (Unix ptys, tests).
func cursorForAnchor(rows [][]Cell, wrapped []bool, line, char, cols int) (row, col int) {
	row = physicalForAnchor(rows, wrapped, line, char)
	accum := 0
	for i := 0; i < row && i < len(rows); i++ {
		accum += len(trimmedCellRow(rows[i]))
		if i >= len(wrapped) || !wrapped[i] {
			accum = 0
		}
	}
	col = char - accum
	if col < 0 {
		col = 0
	} else if col > cols-1 {
		col = cols - 1
	}
	return row, col
}

// anchoredOffsetSeparated returns the display offset that keeps the logical
// (line, char) anchor at the viewport top after a separated reflow. The
// scrollback count is the real len(sb): on a grow the live group can be shorter
// than rows, which the old len(physical)-rows formula underestimated.
func anchoredOffsetSeparated(sb [][]Cell, sbW []bool, live [][]Cell, liveW []bool, line, char int) int {
	newTop := physicalForAnchor(concatRows(sb, live), concatBools(sbW, liveW), line, char)
	newScrollback := len(sb)
	return max(0, min(newScrollback-newTop, newScrollback))
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
	if top == 0 && bottom == t.rows-1 {
		for line := 0; line < lines; line++ {
			rowStart := line * t.cols
			t.appendScrollbackLine(t.cells[rowStart:rowStart+t.cols], len(t.rowWrapped) > line && t.rowWrapped[line])
		}
	}
	start := top * t.cols
	copyStart := (top + lines) * t.cols
	end := (bottom + 1) * t.cols
	copy(t.cells[start:end-lines*t.cols], t.cells[copyStart:end])
	copy(t.rowWrapped[top:bottom-lines+1], t.rowWrapped[top+lines:bottom+1])
	blank := t.blank()
	blankStart := (bottom - lines + 1) * t.cols
	for i := blankStart; i < end; i++ {
		t.cells[i] = blank
	}
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
	start := top * t.cols
	copyEnd := (bottom - lines + 1) * t.cols
	end := (bottom + 1) * t.cols
	copy(t.cells[start+lines*t.cols:end], t.cells[start:copyEnd])
	copy(t.rowWrapped[top+lines:bottom+1], t.rowWrapped[top:bottom-lines+1])
	blank := t.blank()
	blankEnd := (top + lines) * t.cols
	for i := start; i < blankEnd; i++ {
		t.cells[i] = blank
	}
	for row := top; row < top+lines; row++ {
		t.rowWrapped[row] = false
	}
}

func (t *Terminal) appendScrollbackLine(line []Cell, wrapped bool) {
	if len(t.scrollback) != maxScrollbackRows*t.cols {
		t.scrollback = make([]Cell, maxScrollbackRows*t.cols)
		t.scrollbackWrapped = make([]bool, maxScrollbackRows)
		t.scrollbackStart = 0
		t.scrollbackRows = 0
	}

	writeRow := (t.scrollbackStart + t.scrollbackRows) % maxScrollbackRows
	if t.scrollbackRows == maxScrollbackRows {
		writeRow = t.scrollbackStart
		t.scrollbackStart = (t.scrollbackStart + 1) % maxScrollbackRows
	} else {
		t.scrollbackRows++
	}
	copy(t.scrollback[writeRow*t.cols:(writeRow+1)*t.cols], line)
	t.scrollbackWrapped[writeRow] = wrapped

	if t.displayOffset > t.ScrollbackLines() {
		t.displayOffset = t.ScrollbackLines()
	}
}
