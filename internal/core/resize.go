package core

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

	// Preserve whether the user is viewing history while the shared map remaps it.
	anchored := oldOffset > 0

	reflowed, reflowedW := combined, combinedW
	if cols != oldCols {
		reflowed, reflowedW = reflowLogicalRows(logicalRowsFromPhysical(combined, combinedW), cols)
	}

	// Split reflowed into history/live at the boundary through the shared cell map.
	initialMapping := newReflowMap(combined, combinedW, reflowed, reflowedW)
	b, boundaryCol, boundaryOK := initialMapping.mapCell(sbCount, 0)
	if !boundaryOK {
		b, boundaryCol = 0, 0
	}
	sb := append([][]Cell(nil), reflowed[:b]...)
	sbW := append([]bool(nil), reflowedW[:b]...)
	live := append([][]Cell(nil), reflowed[b:]...)
	liveW := append([]bool(nil), reflowedW[b:]...)
	straddle := false
	if b < len(reflowed) && boundaryCol > 0 {
		// A line straddles the boundary: its head remains history and tail remains live.
		straddle = true
		head, tail := splitRowAt(reflowed[b], boundaryCol)
		headFull := make([]Cell, cols)
		copy(headFull, head)
		sb = append(sb, headFull)
		sbW = append(sbW, true)
		live[0] = tail
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

	finalRows, finalWrapped := concatRows(sb, live), concatBools(sbW, liveW)
	mapping := newReflowMap(combined, combinedW, finalRows, finalWrapped)
	evicted := max(0, len(sb)-t.scrollbackCapacity)
	if t.imageSidecars != nil {
		t.imagesReflowPrimary(mapping, evicted, cols)
	}
	if mappedRow, mappedCol, ok := mapping.mapCell(sbCount+t.cursorRow, t.cursorCol); ok {
		curRow = mappedRow - len(sb)
		curCol = mappedCol
	}
	mappedTop, topMapped := 0, false
	if anchored {
		mappedTop, _, topMapped = mapping.mapCell(sbCount-oldOffset, 0)
	}

	t.rebuildScreen(cols, rows, sb, sbW, live, liveW)

	t.cursorRow = max(0, min(rows-1, curRow))
	t.cursorCol = max(0, min(cols-1, curCol))
	t.wrapNext = false
	t.resetScrollRegion()
	t.resizeTabStops(oldCols, cols)

	if anchored && topMapped {
		t.displayOffset = max(0, min(len(sb)-mappedTop, t.ScrollbackLines()))
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
	if t.imageSidecars != nil {
		t.imagesCropAlternate(cols, rows)
	}
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
	t.scrollbackStart, t.displayOffset = 0, 0
	t.scrollbackRows = min(len(sb), t.scrollbackCapacity)
	t.scrollback = nil
	t.scrollbackWrapped = nil
	blank := t.blank()
	if t.scrollbackCapacity > 0 {
		t.scrollback = make([]Cell, t.scrollbackCapacity*cols)
		t.scrollbackWrapped = make([]bool, t.scrollbackCapacity)
		first := len(sb) - t.scrollbackRows
		for row := 0; row < t.scrollbackRows; row++ {
			copy(t.scrollback[row*cols:(row+1)*cols], paddedCellRow(sb[first+row], cols, blank))
			t.scrollbackWrapped[row] = first+row < len(sbW) && sbW[first+row]
		}
	}
	for i := 0; i < len(live) && i < rows; i++ {
		copy(t.cells[i*cols:(i+1)*cols], paddedCellRow(live[i], cols, blank))
		t.rowWrapped[i] = i < len(liveW) && liveW[i]
	}
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

// splitRowAt divides a physical row into head (row[:k]) and tail (row[k:]) as
// independent copies. Used when a logical line straddles the scrollback/live
// boundary: the straddling row is split at the exact char so history cells never
// enter the viewport (which ConPTY would overwrite → loss) and live cells never
// freeze into history (→ duplication).
func splitRowAt(row []Cell, k int) (head, tail []Cell) {
	if k < 0 {
		k = 0
	}
	if k > len(row) {
		k = len(row)
	}
	return append([]Cell(nil), row[:k]...), append([]Cell(nil), row[k:]...)
}
