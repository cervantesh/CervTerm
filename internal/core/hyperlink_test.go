package core

import (
	"fmt"
	"testing"
	"unsafe"
)

func TestHyperlinkCellIdentityWideEraseAndSize(t *testing.T) {
	if got := unsafe.Sizeof(Cell{}); got != 32 {
		t.Fatalf("Cell size=%d", got)
	}
	term := NewTerminal(6, 2)
	if !term.OpenHyperlink("https://example.test/a", "one") {
		t.Fatal("open")
	}
	term.PutRune('A')
	term.PutRune('好')
	cells := make([]Cell, 12)
	term.CopyView(cells)
	id := cells[0].HyperlinkID
	if id == 0 || cells[1].HyperlinkID != id || cells[2].HyperlinkID != id {
		t.Fatalf("ids=%v/%v/%v", id, cells[1].HyperlinkID, cells[2].HyperlinkID)
	}
	term.CloseHyperlink()
	term.ClearLine(0)
	term.CopyView(cells)
	for _, cell := range cells[:6] {
		if cell.HyperlinkID != 0 {
			t.Fatal("erase retained hyperlink")
		}
	}
}

func TestHyperlinkTableBoundedEvictionAndExplicitReuse(t *testing.T) {
	term := NewTerminal(2, 1)
	if !term.OpenHyperlink("https://example.test/reuse", "same") {
		t.Fatal("open")
	}
	first := term.hyperlinks.current
	if !term.OpenHyperlink("https://example.test/reuse", "same") || term.hyperlinks.current != first {
		t.Fatal("explicit id not reused")
	}
	for i := 0; i < MaxHyperlinkEntries; i++ {
		if !term.OpenHyperlink(fmt.Sprintf("https://example.test/%d", i), "") {
			t.Fatalf("open %d", i)
		}
	}
	if len(term.hyperlinks.entries) != MaxHyperlinkEntries {
		t.Fatalf("entries=%d", len(term.hyperlinks.entries))
	}
	if _, ok := term.HyperlinkURI(first); ok {
		t.Fatal("oldest identity not evicted")
	}
}

func TestHyperlinkAlternateScreenIsolationAndReset(t *testing.T) {
	term := NewTerminal(4, 2)
	term.OpenHyperlink("https://primary.test", "")
	primaryID := term.hyperlinks.current
	term.PutRune('P')
	term.SetAlternateScreenMode(true)
	if term.hyperlinks.current != 0 {
		t.Fatal("primary current leaked")
	}
	term.OpenHyperlink("https://alternate.test", "")
	term.PutRune('A')
	term.SetAlternateScreenMode(false)
	cells := make([]Cell, 8)
	term.CopyView(cells)
	if cells[0].HyperlinkID != primaryID {
		t.Fatalf("primary id=%d", cells[0].HyperlinkID)
	}
	if uri, ok := term.HyperlinkURI(primaryID); !ok || uri != "https://primary.test" {
		t.Fatalf("uri=%q,%v", uri, ok)
	}
	term.Reset()
	if len(term.hyperlinks.entries) != 0 || term.hyperlinks.current != 0 {
		t.Fatal("reset retained hyperlinks")
	}
}

func TestLinkedTrailingSpaceSurvivesReflow(t *testing.T) {
	term := NewTerminal(4, 2)
	term.OpenHyperlink("https://space.test", "")
	term.PutRune('X')
	term.PutRune(' ')
	id := term.hyperlinks.current
	term.Resize(2, 3)
	term.Resize(4, 2)
	cells := make([]Cell, 8)
	term.CopyView(cells)
	found := false
	for _, cell := range cells {
		if cell.Rune == ' ' && cell.HyperlinkID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("linked trailing space was trimmed during reflow")
	}
}

func TestHyperlinkTableNeverEvictsReferencedCells(t *testing.T) {
	term := NewTerminalWithHistory(MaxHyperlinkEntries, 1, 0)
	for i := 0; i < MaxHyperlinkEntries; i++ {
		if !term.OpenHyperlink(fmt.Sprintf("https://live.test/%d", i), "") {
			t.Fatalf("open %d", i)
		}
		term.PutRune('x')
	}
	if term.OpenHyperlink("https://overflow.test", "") {
		t.Fatal("open displaced referenced hyperlink")
	}
	cells := make([]Cell, MaxHyperlinkEntries)
	term.CopyView(cells)
	for index, cell := range cells {
		if _, ok := term.HyperlinkURI(cell.HyperlinkID); !ok {
			t.Fatalf("cell %d lost metadata", index)
		}
	}
}

func TestHyperlinkIdentitySurvivesScrollbackAndResize(t *testing.T) {
	term := NewTerminalWithHistory(4, 2, 4)
	term.OpenHyperlink("https://history.test", "")
	term.PutRune('H')
	id := term.hyperlinks.current
	term.CloseHyperlink()
	for i := 0; i < 4; i++ {
		term.CarriageReturn()
		term.NewLine()
		term.PutRune(rune('0' + i))
	}
	rows, _ := term.physicalRows()
	found := false
	for _, row := range rows {
		for _, cell := range row {
			if cell.HyperlinkID == id {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("scrollback lost hyperlink identity")
	}
	term.Resize(3, 3)
	rows, _ = term.physicalRows()
	found = false
	for _, row := range rows {
		for _, cell := range row {
			if cell.HyperlinkID == id {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("resize lost scrollback hyperlink identity")
	}
}
