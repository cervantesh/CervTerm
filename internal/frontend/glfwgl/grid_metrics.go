package glfwgl

// gridMetrics is the pure pixel<->cell geometry of the terminal grid: cell size,
// padding, and grid extent. It is extracted from App so the mapping is a plain
// value with no window, GL, or lock dependency — the mouse and selection paths
// derive cells from it, and it is testable in isolation (this file carries no
// build tag on purpose, so the geometry tests run without the glfw toolchain).
type gridMetrics struct {
	cellW, cellH       float32
	paddingX, paddingY float32
	contentRight       float32
	cols, rows         int
}

// cellAt maps a window pixel to the grid cell under it, clamped to the visible
// grid. Callers pass framebuffer-space pixels; the result is a (row, col) inside
// [0,rows) x [0,cols).
func (g gridMetrics) cellAt(x, y float32) (row, col int) {
	col = int((x - g.paddingX) / g.cellW)
	row = int((y - g.paddingY) / g.cellH)
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
	if right <= g.paddingX {
		right = g.paddingX + float32(g.cols)*g.cellW
	}
	return x >= g.paddingX && x < right && y >= g.paddingY && y < g.paddingY+float32(g.rows)*g.cellH
}
