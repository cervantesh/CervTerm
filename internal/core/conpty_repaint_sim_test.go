package core

import (
	"strings"
	"testing"
)

// simulateConPTYViewportRepaint mimics what ConPTY does after a resize on
// Windows: it homes the cursor and repaints the whole viewport from the shell's
// (often nearly empty) buffer. Here that is "home + erase to end of screen +
// draw a fresh prompt" — it touches only the live screen, never scrollback.
func simulateConPTYViewportRepaint(t *Terminal) {
	t.SetCursor(0, 0)
	t.ClearToEndOfScreen()
	for _, r := range ">" {
		t.PutRune(r)
	}
}

// TestConPTYRepaintAfterGrowKeepsHistory is the hermetic capture of the reported
// bug. The pure Resize path is correct (see TestZoomCycleDoesNotLoseHistory), so
// the loss only appears once ConPTY repaints the viewport after a grow. With the
// old shared reflow, a grow pulled scrollback up into the viewport, and this
// repaint then erased it. With the separated reflow, history stays in scrollback
// and the viewport repaint cannot touch it.
//
// This test FAILS against the old rebuildFromPhysicalRows and PASSES with the
// separated resizePrimary.
func TestConPTYRepaintAfterGrowKeepsHistory(t *testing.T) {
	term := NewTerminal(40, 12)
	for i := 1; i <= 10; i++ {
		feedLine(term, "LINEA-"+twoDigit(i)+"-marcador")
	}

	term.Resize(40, 2)  // zoom in: most lines spill into scrollback
	term.Resize(40, 12) // zoom out: separated reflow must keep them in scrollback

	// The shell/ConPTY now repaints the enlarged viewport.
	simulateConPTYViewportRepaint(term)

	after := fullText(term)
	// The lines that scrolled into history (the earliest ones) must survive the
	// viewport repaint. LINEA-01..08 are comfortably in scrollback after a shrink
	// to 2 rows of a 10-line screen.
	for i := 1; i <= 8; i++ {
		marker := "LINEA-" + twoDigit(i) + "-marcador"
		if !strings.Contains(after, marker) {
			t.Fatalf("history line %s destroyed by ConPTY viewport repaint after grow\n--- full text ---\n%s", marker, after)
		}
	}
	if term.ScrollbackLines() == 0 {
		t.Fatalf("expected surviving scrollback after grow+repaint, got 0 lines")
	}
}

// TestGrowDoesNotPullScrollbackIntoViewport pins the core invariant: growing the
// grid must not move history into the live viewport (where ConPTY would clobber
// it). After a shrink+grow the earliest lines stay in scrollback, not on screen.
func TestGrowDoesNotPullScrollbackIntoViewport(t *testing.T) {
	term := NewTerminal(40, 12)
	for i := 1; i <= 10; i++ {
		feedLine(term, "LINEA-"+twoDigit(i)+"-marcador")
	}
	term.Resize(40, 2)
	sbAfterShrink := term.ScrollbackLines()
	term.Resize(40, 12)

	if got := term.ScrollbackLines(); got < sbAfterShrink {
		t.Fatalf("grow pulled history out of scrollback: %d -> %d", sbAfterShrink, got)
	}
	// The earliest line must NOT be on the live screen; it lives in history.
	if strings.Contains(term.PlainText(), "LINEA-01-marcador") {
		t.Fatalf("grow pulled LINEA-01 into the viewport:\n%s", term.PlainText())
	}
	// ...but it is still reachable in full history.
	if !strings.Contains(fullText(term), "LINEA-01-marcador") {
		t.Fatalf("LINEA-01 lost from history entirely")
	}
}

// TestShrinkCutsSeamAtNewBoundary covers the pair-review finding: when a shrink
// pushes live rows into scrollback, the last pushed row must have its wrapped
// flag cleared (seam cut at the NEW boundary), or selection/copy would wrongly
// join that history line with the first live line until the next resize.
func TestShrinkCutsSeamAtNewBoundary(t *testing.T) {
	term := NewTerminal(4, 4)
	for _, r := range "abcdefghijkl" { // wraps to abcd|efgh|ijkl (rows 0,1 wrapped)
		term.PutRune(r)
	}
	term.Resize(4, 1) // pushes abcd + efgh into scrollback; efgh becomes the seam

	if term.ScrollbackLines() != 2 {
		t.Fatalf("expected 2 scrollback rows after shrink, got %d", term.ScrollbackLines())
	}
	last := (term.scrollbackStart + term.scrollbackRows - 1) % maxScrollbackRows
	if term.scrollbackWrapped[last] {
		t.Fatalf("seam not cut: last scrollback row still wrapped=true after shrink")
	}
}

// TestZoomRepaintColsAndRowsKeepsHistory is the combined case (both dimensions
// change, like a real font zoom) with a ConPTY viewport repaint afterwards.
func TestZoomRepaintColsAndRowsKeepsHistory(t *testing.T) {
	term := NewTerminal(40, 12)
	for i := 1; i <= 10; i++ {
		feedLine(term, "LINEA-"+twoDigit(i)+"-marcador")
	}
	term.Resize(16, 3)  // zoom in: fewer cols AND rows
	term.Resize(40, 12) // zoom out: both grow back
	simulateConPTYViewportRepaint(term)

	after := fullText(term)
	for i := 1; i <= 8; i++ {
		marker := "LINEA-" + twoDigit(i) + "-marcador"
		if !strings.Contains(after, marker) {
			t.Fatalf("history line %s lost across cols+rows zoom + repaint\n--- full ---\n%s", marker, after)
		}
	}
}

func twoDigit(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
