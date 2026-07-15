package core

import "strings"

// RowText flattens one row of cells into its visible text using the single
// canonical rule for "the text of a row": skip WideContinuation and Rune==0
// padding cells, emit each cell's rune followed by its combining marks, and trim
// trailing blank cells (spaces and Rune==0 padding). It reuses the same
// writeCellText/isBlankCell core that PlainText/CopyView already apply, so there
// is exactly one policy for row text.
//
// This is the shared source of truth for copy/selection (selection.Text) and the
// scriptable term:line() accessor (App.Line): a row reads identically whether it
// is copied to the clipboard or queried from Lua. Before this seam, term:line()
// diverged (it turned Rune==0 into a written space and kept WideContinuation),
// so "A", {Rune:0}, "B" copied as "AB" but scripted as "A B". Both now agree.
func RowText(cells []Cell) string {
	last := len(cells) - 1
	for last >= 0 && isBlankCell(cells[last]) {
		last--
	}
	if last < 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i <= last; i++ {
		writeCellText(&b, cells[i])
	}
	return b.String()
}
