package selection

import (
	"strings"

	"cervterm/internal/core"
)

type Point struct {
	Row int
	Col int
}

type Range struct {
	Start Point
	End   Point
}

func Normalize(r Range) Range {
	if before(r.End, r.Start) {
		r.Start, r.End = r.End, r.Start
	}
	return r
}

func Contains(r Range, p Point) bool {
	r = Normalize(r)
	return !before(p, r.Start) && !before(r.End, p)
}

func Text(cells []core.Cell, cols, rows int, r Range) string {
	return TextWithWrapped(cells, nil, cols, rows, r)
}

func TextWithWrapped(cells []core.Cell, wrapped []bool, cols, rows int, r Range) string {
	if cols <= 0 || rows <= 0 || len(cells) == 0 {
		return ""
	}
	r = clamp(cols, rows, Normalize(r))

	var b strings.Builder
	for row := r.Start.Row; row <= r.End.Row; row++ {
		startCol := 0
		if row == r.Start.Row {
			startCol = r.Start.Col
		}
		endCol := cols - 1
		if row == r.End.Row {
			endCol = r.End.Col
		}
		if endCol < startCol {
			continue
		}

		line := lineText(cells, cols, row, startCol, endCol)
		b.WriteString(line)
		if row != r.End.Row && (row >= len(wrapped) || !wrapped[row]) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func before(a, b Point) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

func clamp(cols, rows int, r Range) Range {
	r.Start = clampPoint(cols, rows, r.Start)
	r.End = clampPoint(cols, rows, r.End)
	return Normalize(r)
}

func clampPoint(cols, rows int, p Point) Point {
	if p.Row < 0 {
		p.Row = 0
	}
	if p.Col < 0 {
		p.Col = 0
	}
	if p.Row >= rows {
		p.Row = rows - 1
	}
	if p.Col >= cols {
		p.Col = cols - 1
	}
	return p
}

// lineText returns the text of one selected span [startCol, endCol] of a row.
// It defers to core.RowText — the single canonical row-text rule shared with the
// term:line() accessor — applied to the span slice, so the trailing-blank trim
// and cell-skipping policy match copy/paste and Lua exactly.
func lineText(cells []core.Cell, cols, row, startCol, endCol int) string {
	base := row * cols
	return core.RowText(cells[base+startCol : base+endCol+1])
}
