package core

import (
	"strings"
	"unicode/utf8"
)

const MaxSemanticTextBytes = 1 << 20

func (t *Terminal) SemanticRangeText(target SemanticRange) (string, bool) {
	if target.Kind == SemanticNone || target.Kind > SemanticOutput || target.Start.GlobalRow < 0 ||
		target.End.GlobalRow < target.Start.GlobalRow || target.End.GlobalRow >= t.scrollbackRows+t.rows {
		return "", false
	}
	var out strings.Builder
	for global := target.Start.GlobalRow; global <= target.End.GlobalRow; global++ {
		row := t.semanticPhysicalRow(global)
		start, end := 0, len(row)
		if global == target.Start.GlobalRow {
			start = target.Start.Col
		}
		if global == target.End.GlobalRow {
			end = target.End.Col
		}
		if start < 0 || end < start || end > len(row) {
			return "", false
		}
		for _, cell := range row[start:end] {
			if SemanticCellKind(cell) != target.Kind || cell.WideContinuation || cell.Rune == 0 {
				continue
			}
			needed := utf8.RuneLen(cell.Rune)
			for _, mark := range cell.Combining() {
				needed += utf8.RuneLen(mark)
			}
			if needed < 0 || out.Len()+needed > MaxSemanticTextBytes {
				return "", false
			}
			out.WriteRune(cell.Rune)
			for _, mark := range cell.Combining() {
				out.WriteRune(mark)
			}
		}
		if global < target.End.GlobalRow && !t.semanticPhysicalRowWrapped(global) {
			if out.Len()+1 > MaxSemanticTextBytes {
				return "", false
			}
			out.WriteByte('\n')
		}
	}
	return out.String(), true
}
