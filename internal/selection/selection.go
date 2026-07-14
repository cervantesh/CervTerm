package selection

import (
	"strings"

	"cervterm/internal/core"
	"cervterm/internal/render"
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

func Text(snap render.Snapshot, r Range) string {
	if snap.Cols <= 0 || snap.Rows <= 0 || len(snap.Cells) == 0 {
		return ""
	}
	r = clamp(snap, Normalize(r))

	var b strings.Builder
	for row := r.Start.Row; row <= r.End.Row; row++ {
		startCol := 0
		if row == r.Start.Row {
			startCol = r.Start.Col
		}
		endCol := snap.Cols - 1
		if row == r.End.Row {
			endCol = r.End.Col
		}
		if endCol < startCol {
			continue
		}

		line := lineText(snap, row, startCol, endCol)
		b.WriteString(line)
		if row != r.End.Row {
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

func clamp(snap render.Snapshot, r Range) Range {
	r.Start = clampPoint(snap, r.Start)
	r.End = clampPoint(snap, r.End)
	return Normalize(r)
}

func clampPoint(snap render.Snapshot, p Point) Point {
	if p.Row < 0 {
		p.Row = 0
	}
	if p.Col < 0 {
		p.Col = 0
	}
	if p.Row >= snap.Rows {
		p.Row = snap.Rows - 1
	}
	if p.Col >= snap.Cols {
		p.Col = snap.Cols - 1
	}
	return p
}

func lineText(snap render.Snapshot, row, startCol, endCol int) string {
	last := endCol
	for last >= startCol {
		cell := snap.Cells[row*snap.Cols+last]
		if !isBlankCell(cell) {
			break
		}
		last--
	}
	if last < startCol {
		return ""
	}

	var b strings.Builder
	for col := startCol; col <= last; col++ {
		writeCellText(&b, snap.Cells[row*snap.Cols+col])
	}
	return b.String()
}

func isBlankCell(cell core.Cell) bool {
	return cell.WideContinuation || cell.Rune == 0 || cell.Rune == ' '
}

func writeCellText(b *strings.Builder, cell core.Cell) {
	if cell.WideContinuation || cell.Rune == 0 {
		return
	}
	b.WriteRune(cell.Rune)
	for _, r := range cell.Combining() {
		b.WriteRune(r)
	}
}
