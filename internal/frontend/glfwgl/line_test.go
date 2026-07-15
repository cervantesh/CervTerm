//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/core"
)

// TestLineMatchesCanonicalRowText pins that term:line() (App.Line) returns
// exactly the canonical core.RowText of the row — the same text the clipboard
// and selection produce. This is the anti-divergence guard for the refactor:
// App.Line used to reimplement the row-text rule by hand and diverged from the
// canonical policy (it turned Rune==0 padding into a written space and kept
// WideContinuation), so a row could copy as "AB" but script as "A B". Asserting
// byte-for-byte equality with core.RowText over the whole viewport makes it
// impossible for the call site to drift from the shared rule again, whatever the
// row contains (gaps, wide glyphs, trailing blanks).
func TestLineMatchesCanonicalRowText(t *testing.T) {
	a := &App{term: core.NewTerminal(8, 3)}
	// Row 0: normal text with a cursor-positioned gap and trailing blanks.
	// Row 1: a wide (CJK) glyph, which lays down a base rune + WideContinuation.
	a.parser.Advance(a.term, []byte("Hi\x1b[5GX\r\n世ok"))

	cols, rows := 8, 3
	cells := make([]core.Cell, cols*rows)
	a.term.CopyView(cells)

	for row := 0; row < rows; row++ {
		got, ok := a.Line(row)
		if !ok {
			t.Fatalf("Line(%d) returned ok=false", row)
		}
		want := core.RowText(cells[row*cols : row*cols+cols])
		if got != want {
			t.Fatalf("Line(%d) = %q, core.RowText = %q; term:line() must equal the canonical row text", row, got, want)
		}
	}
}

// TestLineWideGlyph pins that a wide glyph's trailing WideContinuation cell is
// emitted once (as the base rune), never doubled or turned into padding.
func TestLineWideGlyph(t *testing.T) {
	a := &App{term: core.NewTerminal(6, 2)}
	a.parser.Advance(a.term, []byte("世X"))

	got, ok := a.Line(0)
	if !ok {
		t.Fatal("Line(0) returned ok=false")
	}
	if got != "世X" {
		t.Fatalf("Line(0) = %q, want %q (wide continuation must not duplicate or pad)", got, "世X")
	}
}
