package vt

import (
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestOSC8OpenCloseBELSTAndChunking(t *testing.T) {
	term := core.NewTerminal(10, 2)
	var p Parser
	p.Advance(term, []byte("\x1b]8;id=doc;https://example.test/"))
	p.Advance(term, []byte("a\x1b\\A\x1b]8;;\aB"))
	cells := copyCells(term)
	if cells[0].HyperlinkID == 0 || cells[1].HyperlinkID != 0 {
		t.Fatalf("ids=%d/%d", cells[0].HyperlinkID, cells[1].HyperlinkID)
	}
	uri, ok := term.HyperlinkURI(cells[0].HyperlinkID)
	if !ok || uri != "https://example.test/a" {
		t.Fatalf("uri=%q,%v", uri, ok)
	}
}

func TestOSC8MalformedOpenIsAtomic(t *testing.T) {
	term := core.NewTerminal(10, 2)
	var p Parser
	p.Advance(term, []byte("\x1b]8;;https://good.test\x1b\\A"))
	id := copyCells(term)[0].HyperlinkID
	cases := []string{
		"\x1b]8;id=x\x1b\\", "\x1b]8;id=x:id=y;https://bad.test\x1b\\", "\x1b]8;bad;https://bad.test\x1b\\", "\x1b]8;;noscheme\x1b\\",
	}
	for _, seq := range cases {
		p.Advance(term, []byte(seq))
	}
	p.Advance(term, []byte("B"))
	if copyCells(term)[1].HyperlinkID != id {
		t.Fatal("malformed open changed active hyperlink")
	}
}

func TestOSC8ProtocolLimits(t *testing.T) {
	term := core.NewTerminal(4, 1)
	var p Parser
	p.Advance(term, []byte("\x1b]8;;https://good.test\x1b\\"))
	id := termCellID(term)
	tooLong := strings.Repeat("x", core.MaxHyperlinkURIBytes+1)
	p.Advance(term, []byte("\x1b]8;;https://example.test/"+tooLong+"\x1b\\Z"))
	if copyCells(term)[0].HyperlinkID != id {
		t.Fatal("oversized URI changed active hyperlink")
	}
}

func termCellID(term *core.Terminal) core.HyperlinkID {
	term.PutRune('X')
	id := copyCells(term)[0].HyperlinkID
	term.SetCursor(0, 0)
	return id
}
