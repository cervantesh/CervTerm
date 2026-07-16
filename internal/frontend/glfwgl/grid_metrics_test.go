package glfwgl

import "testing"

func TestGridMetricsCellAt(t *testing.T) {
	// 10x20 cells, 5px padding, an 8x4 grid.
	g := gridMetrics{cellW: 10, cellH: 20, paddingX: 5, paddingY: 5, cols: 8, rows: 4}

	cases := []struct {
		name             string
		x, y             float32
		wantRow, wantCol int
	}{
		{"top-left of grid maps to 0,0", 5, 5, 0, 0},
		{"inside cell (2,2)", 5 + 2*10 + 3, 5 + 2*20 + 7, 2, 2},
		{"pixels above/left of the grid clamp to 0,0", -100, -100, 0, 0},
		{"pixels past the grid clamp to the last cell", 10000, 10000, 3, 7},
		{"right edge column clamps", 5 + 100, 5, 0, 7},
		{"bottom edge row clamps", 5, 5 + 100, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, col := g.cellAt(tc.x, tc.y)
			if row != tc.wantRow || col != tc.wantCol {
				t.Fatalf("cellAt(%v,%v) = (%d,%d), want (%d,%d)", tc.x, tc.y, row, col, tc.wantRow, tc.wantCol)
			}
		})
	}
}

func TestGridMetricsRejectsReservedGutter(t *testing.T) {
	g := gridMetrics{cellW: 10, cellH: 20, paddingX: 5, paddingY: 5, contentRight: 85, cols: 8, rows: 4}
	if !g.containsCell(84.9, 10) {
		t.Fatal("last grid pixel should be inside content")
	}
	if g.containsCell(85, 10) || g.containsCell(100, 10) {
		t.Fatal("reserved gutter must not be a terminal cell")
	}
}
