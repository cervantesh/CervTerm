package glfwgl

// gridMetrics is the pure pixel<->cell geometry of a terminal grid.
type gridMetrics struct {
	cellW, cellH     float32
	originX, originY float32
	contentRight     float32
	cols, rows       int
}

// cellAt maps a window pixel to the grid cell under it, clamped to the visible
// grid. Callers pass framebuffer-space pixels; the result is a (row, col) inside
// [0,rows) x [0,cols).
func (g gridMetrics) cellAt(x, y float32) (row, col int) {
	col = int((x - g.originX) / g.cellW)
	row = int((y - g.originY) / g.cellH)
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if row >= g.rows {
		row = g.rows - 1
	}
	if col >= g.cols {
		col = g.cols - 1
	}
	return row, col
}

func (g gridMetrics) containsCell(x, y float32) bool {
	right := g.contentRight
	if right <= g.originX {
		right = g.originX + float32(g.cols)*g.cellW
	}
	return x >= g.originX && x < right && y >= g.originY && y < g.originY+float32(g.rows)*g.cellH
}
