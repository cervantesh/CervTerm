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
		if (cell.Rune != 0 && cell.Rune != ' ') || len(cell.Combining) != 0 || cell.Attr != blankAttr || cell.WideContinuation {
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

func (t *Terminal) rebuildFromPhysicalRows(cols, rows int, physicalRows [][]Cell, wrappedRows []bool, displayOffset int) {
	t.cols, t.rows = cols, rows
	t.cells = make([]Cell, cols*rows)
	t.rowWrapped = make([]bool, rows)
	t.fillBlank(t.cells)
	t.scrollback = nil
	t.scrollbackWrapped = nil
	t.scrollbackStart = 0
	t.scrollbackRows = 0

	visibleRows := min(rows, len(physicalRows))
	scrollbackRows := len(physicalRows) - visibleRows
	blank := t.blank()
	for row := 0; row < scrollbackRows; row++ {
		t.appendScrollbackLine(paddedCellRow(physicalRows[row], cols, blank), row < len(wrappedRows) && wrappedRows[row])
	}

	visibleStart := len(physicalRows) - visibleRows
	for row := 0; row < visibleRows; row++ {
		physicalIndex := visibleStart + row
		copy(t.cells[row*cols:(row+1)*cols], paddedCellRow(physicalRows[physicalIndex], cols, blank))
		t.rowWrapped[row] = physicalIndex < len(wrappedRows) && wrappedRows[physicalIndex]
	}

	t.displayOffset = min(displayOffset, t.ScrollbackLines())
	if visibleRows > 0 {
		t.cursorRow = visibleRows - 1
	} else {
		t.cursorRow = 0
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
	for i := range out {
		if len(out[i].Combining) > 0 {
			out[i].Combining = append([]rune(nil), out[i].Combining...)
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
	for _, r := range cell.Combining {
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
		trimmed := trimmedCellRow(row)
		if len(trimmed) > 0 {
			current = append(current, trimmed...)
		}
		wrapped := i < len(wrappedRows) && wrappedRows[i]
		if wrapped {
			continue
		}
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
