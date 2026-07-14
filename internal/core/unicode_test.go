package core

import "testing"

func TestTerminalWideRuneOccupiesTwoCells(t *testing.T) {
	term := NewTerminal(6, 2)
	term.PutRune('A')
	term.PutRune('好')
	term.PutRune('B')

	cells := term.Cells()
	if cells[0].Rune != 'A' || cells[1].Rune != '好' || !cells[2].WideContinuation || cells[3].Rune != 'B' {
		t.Fatalf("unexpected wide-cell layout: %#v", cells[:4])
	}
	if term.CursorCol() != 4 {
		t.Fatalf("wide rune should advance cursor by 2 cells, got col %d", term.CursorCol())
	}
	if got := term.PlainText(); got != "A好B\n" {
		t.Fatalf("plain text should skip wide continuation, got %q", got)
	}
}

func TestTerminalWideRuneWrapsAtRightEdge(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abc" {
		term.PutRune(r)
	}
	term.PutRune('好')
	if got := term.PlainText(); got != "abc\n好" {
		t.Fatalf("wide rune wrap mismatch: %q", got)
	}
}

func TestTerminalCombiningRuneAttachesToPreviousCell(t *testing.T) {
	term := NewTerminal(6, 1)
	term.PutRune('e')
	term.PutRune('\u0301')
	term.PutRune('x')

	cells := term.Cells()
	if len(cells[0].Combining()) != 1 || cells[0].Combining()[0] != '\u0301' {
		t.Fatalf("combining mark not attached: %#v", cells[0])
	}
	if term.CursorCol() != 2 {
		t.Fatalf("combining mark should not advance cursor, got col %d", term.CursorCol())
	}
	if got := term.PlainText(); got != "e\u0301x" {
		t.Fatalf("plain text should preserve combining mark, got %q", got)
	}
}
