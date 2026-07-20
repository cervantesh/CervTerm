package core

// CopyViewRows copies a bounded row range from the current viewport without
// materializing the rest of the viewport or any unrelated history.
func (t *Terminal) CopyViewRows(dst []Cell, startRow, rows int) bool {
	if startRow < 0 || rows < 0 || startRow+rows > t.rows || len(dst) < rows*t.cols {
		return false
	}
	startGlobal := t.ViewportTopGlobalRow() + startRow
	for row := 0; row < rows; row++ {
		globalRow := startGlobal + row
		target := dst[row*t.cols : (row+1)*t.cols]
		if globalRow < t.scrollbackRows {
			sourceRow := (t.scrollbackStart + globalRow) % t.scrollbackCapacity
			copy(target, t.scrollback[sourceRow*t.cols:(sourceRow+1)*t.cols])
		} else {
			currentRow := globalRow - t.scrollbackRows
			copy(target, t.cells[currentRow*t.cols:(currentRow+1)*t.cols])
		}
	}
	return true
}
