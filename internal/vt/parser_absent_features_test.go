package vt

import (
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestParserIndexNextLineReverseIndex(t *testing.T) {
	term := core.NewTerminal(4, 3)
	var p Parser
	p.Advance(term, []byte("aaa\r\nbbb\r\nccc\x1bM"))
	if term.CursorRow() != 1 {
		t.Fatalf("RI moved cursor to row %d", term.CursorRow())
	}
	p.Advance(term, []byte("\x1bD"))
	if term.CursorRow() != 2 {
		t.Fatalf("IND moved cursor to row %d", term.CursorRow())
	}
	p.Advance(term, []byte("\x1bE"))
	if term.CursorCol() != 0 {
		t.Fatalf("NEL cursor col = %d", term.CursorCol())
	}
}

func TestParserDECSpecialGraphics(t *testing.T) {
	term := core.NewTerminal(8, 2)
	var p Parser
	p.Advance(term, []byte("\x1b(0lqk\x1b(Bq"))
	if got := strings.TrimSpace(term.PlainText()); got != "┌─┐q" {
		t.Fatalf("plain text = %q", got)
	}
}

func TestParserOriginInsertTabsAndCursorStyle(t *testing.T) {
	term := core.NewTerminal(10, 5)
	var p Parser
	p.Advance(term, []byte("\x1b[2;4r\x1b[?6h\x1b[1;1H"))
	if term.CursorRow() != 1 || term.CursorCol() != 0 {
		t.Fatalf("DECOM cursor = %d,%d", term.CursorRow(), term.CursorCol())
	}
	var got strings.Builder
	p.Reply = func(b []byte) { got.Write(b) }
	p.Advance(term, []byte("\x1b[6n"))
	if got.String() != "\x1b[1;1R" {
		t.Fatalf("origin CPR = %q", got.String())
	}

	term = core.NewTerminal(8, 2)
	p = Parser{}
	p.Advance(term, []byte("abcd\x1b[1G\x1b[4hX"))
	if got := strings.TrimSpace(term.PlainText()); got != "Xabcd" {
		t.Fatalf("IRM text = %q", got)
	}

	term = core.NewTerminal(12, 1)
	p.Advance(term, []byte("a\x1bH\x1b[3g\x1b[1G\x1b[2I\x1b[2 q"))
	if term.CursorCol() != 11 {
		t.Fatalf("CHT after clearing all stops = %d", term.CursorCol())
	}
	if term.CursorStyle() != 2 {
		t.Fatalf("cursor style = %d", term.CursorStyle())
	}
}

func TestParserSGRDimBlink(t *testing.T) {
	term := core.NewTerminal(8, 1)
	var p Parser
	p.Advance(term, []byte("\x1b[1;2;5mX\x1b[22mY\x1b[25mZ"))
	cells := term.Cells()
	if !cells[0].Attr.Bold || !cells[0].Attr.Dim || !cells[0].Attr.Blink {
		t.Fatalf("first attr = %+v", cells[0].Attr)
	}
	if cells[1].Attr.Bold || cells[1].Attr.Dim || !cells[1].Attr.Blink {
		t.Fatalf("second attr = %+v", cells[1].Attr)
	}
	if cells[2].Attr.Blink {
		t.Fatalf("third attr = %+v", cells[2].Attr)
	}
}

func TestParserOSC52AndOSC7(t *testing.T) {
	term := core.NewTerminal(8, 1)
	var got string
	p := Parser{SetClipboard: func(s string) { got = s }}
	p.Advance(term, []byte("\x1b]52;c;aGVsbG8=\x07"))
	if got != "hello" {
		t.Fatalf("clipboard = %q", got)
	}
	p.Advance(term, []byte("\x1b]52;c;?\x07"))
	if got != "hello" {
		t.Fatalf("query changed clipboard to %q", got)
	}
	p.Advance(term, []byte("\x1b]52;c;not-base64\x07"))
	if got != "hello" {
		t.Fatalf("invalid changed clipboard to %q", got)
	}
	p.Advance(term, []byte("\x1b]7;file://host/tmp\x07"))
	if term.WorkingDirectoryURL() != "file://host/tmp" {
		t.Fatalf("cwd = %q", term.WorkingDirectoryURL())
	}
}
