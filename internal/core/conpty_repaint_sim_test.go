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

// TestWrappedLineSurvivesShrinkWidenInOrder pins content integrity across a
// shrink+widen. The combined reflow preserves wrap flags (no permanent seam cut),
// so the line's characters stay in order with none lost or duplicated. A line
// that lands exactly on the scrollback/live boundary may still render across two
// rows (dual ownership under ConPTY — it fully heals once new output scrolls its
// live part into history), but its content must remain contiguous and single.
func TestWrappedLineSurvivesShrinkWidenInOrder(t *testing.T) {
	term := NewTerminal(12, 4)
	for _, r := range "abcdefghijkl" {
		term.PutRune(r)
	}
	term.Resize(4, 1)  // narrow: wraps; top rows spill to scrollback
	term.Resize(12, 4) // widen back

	joined := strings.ReplaceAll(strings.TrimRight(fullText(term), "\n"), "\n", "")
	if joined != "abcdefghijkl" {
		t.Fatalf("content not preserved contiguously after shrink+widen: %q", joined)
	}
}

// TestNoAccumulationAcrossZoomCycles pins the core win over the old seam-cut:
// repeated narrow/wide cycles must not shred a line into ever more fragments or
// duplicate it. The old path committed a permanent cut per cycle; this one does
// not, so the fragment/line count stays bounded.
func TestNoAccumulationAcrossZoomCycles(t *testing.T) {
	term := NewTerminal(30, 6)
	for _, r := range "MARKER-0123456789-0123456789-END" {
		term.PutRune(r)
	}
	for i := 0; i < 8; i++ {
		term.Resize(6, 2)
		term.Resize(30, 6)
	}
	full := fullText(term)
	joined := strings.ReplaceAll(strings.TrimRight(full, "\n"), "\n", "")
	if joined != "MARKER-0123456789-0123456789-END" {
		t.Fatalf("line accumulated fragments / lost content over cycles: %q", joined)
	}
	if strings.Count(full, "MARKER") != 1 {
		t.Fatalf("line duplicated over cycles: MARKER appears %d times\n%s", strings.Count(full, "MARKER"), full)
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

// TestStraddleLineNoLossNoDuplication is Fable's key correctness case: a long
// line whose head lands in scrollback and tail in the live viewport must not be
// duplicated when ConPTY repaints the viewport, and its head must survive in
// history. The char-split at the boundary is what guarantees this.
func TestStraddleLineNoLossNoDuplication(t *testing.T) {
	term := NewTerminal(40, 6)
	line := "HEADMARK-1234567890-1234567890-TAILMARK"
	for _, r := range line {
		term.PutRune(r)
	}
	term.Resize(12, 2)  // narrow AND short: the line wraps past the viewport, so its
	term.Resize(40, 6)  // head spills to scrollback and its tail stays live (straddle)
	simulateConPTYViewportRepaint(term) // shell repaints the viewport (clears the tail)

	full := fullText(term)
	if n := strings.Count(full, "HEADMARK"); n != 1 {
		t.Fatalf("head duplicated or lost: HEADMARK appears %d times\n%s", n, full)
	}
	// The surviving head must have no fake spaces spliced after its content (the
	// char-split head is zero-padded, so re-reads drop the ring padding).
	joined := strings.ReplaceAll(strings.TrimRight(full, "\n"), "\n", "")
	if strings.Contains(joined, "1234  ") || strings.Contains(joined, "HEADMARK ") {
		t.Fatalf("space pollution near boundary:\n%q", joined)
	}
}

func twoDigit(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
