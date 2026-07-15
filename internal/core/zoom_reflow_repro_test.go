package core

import (
	"strings"
	"testing"
)

// fullText returns every physical row (scrollback + screen) joined, so a test
// can assert content survived a resize even if it scrolled into history.
func fullText(t *Terminal) string {
	rows, _ := t.physicalRows()
	var b strings.Builder
	for _, row := range rows {
		last := len(row) - 1
		for last >= 0 && isBlankCell(row[last]) {
			last--
		}
		for i := 0; i <= last; i++ {
			writeCellText(&b, row[i])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func feedLine(t *Terminal, s string) {
	for _, r := range s {
		t.PutRune(r)
	}
	t.CarriageReturn()
	t.NewLine()
}

// TestZoomCycleDoesNotLoseHistory reproduces the reported bug: zooming in (font
// grows -> fewer cols/rows) then back out drops earlier lines entirely. A zoom
// step is a Resize; a burst is a sequence of Resizes. Content must survive.
func TestZoomCycleDoesNotLoseHistory(t *testing.T) {
	term := NewTerminal(120, 37)
	for i := 1; i <= 8; i++ {
		feedLine(term, "LINEA-"+string(rune('0'+i))+"-marcador")
	}
	before := fullText(term)
	for i := 1; i <= 8; i++ {
		marker := "LINEA-" + string(rune('0'+i)) + "-marcador"
		if !strings.Contains(before, marker) {
			t.Fatalf("setup: %s missing before resize", marker)
		}
	}

	// Zoom in: shrink toward a huge font (few cols/rows), stepping like the burst.
	sizes := [][2]int{{100, 30}, {80, 24}, {60, 18}, {40, 12}, {30, 9}, {23, 7}}
	for _, s := range sizes {
		term.Resize(s[0], s[1])
	}
	// Zoom back out to the original geometry, stepping back up.
	for i := len(sizes) - 2; i >= 0; i-- {
		term.Resize(sizes[i][0], sizes[i][1])
	}
	term.Resize(120, 37)

	after := fullText(term)
	for i := 1; i <= 8; i++ {
		marker := "LINEA-" + string(rune('0'+i)) + "-marcador"
		if !strings.Contains(after, marker) {
			t.Fatalf("content lost after zoom cycle: %s missing\n--- after ---\n%s", marker, after)
		}
	}
}

// TestSingleZoomInThenOutKeepsHistory isolates one shrink + one grow.
func TestSingleZoomInThenOutKeepsHistory(t *testing.T) {
	term := NewTerminal(120, 37)
	for i := 1; i <= 8; i++ {
		feedLine(term, "LINEA-"+string(rune('0'+i))+"-marcador")
	}
	term.Resize(23, 7) // zoom way in
	term.Resize(120, 37)

	after := fullText(term)
	for i := 1; i <= 8; i++ {
		marker := "LINEA-" + string(rune('0'+i)) + "-marcador"
		if !strings.Contains(after, marker) {
			t.Fatalf("content lost after single zoom in/out: %s missing\n--- after ---\n%s", marker, after)
		}
	}
}
