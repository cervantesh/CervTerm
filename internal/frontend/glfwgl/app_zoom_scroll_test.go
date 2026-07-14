//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/core"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestZoomCoalescesAndCompounds(t *testing.T) {
	a := &App{}
	a.cfg.Font.Size = 14

	// A burst of wheel-up ticks before any rebuild must compound, not collapse
	// into one step, and must not touch the live font size (rebuild is deferred).
	for i := 0; i < 5; i++ {
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	}
	if a.cfg.Font.Size != 14 {
		t.Fatalf("live font size changed before applyPendingZoom: %v", a.cfg.Font.Size)
	}
	if !a.zoom.pendingSet || a.zoom.pending != 14+5*zoomFontStep {
		t.Fatalf("expected pending %v, got %v (set=%v)", 14+5*zoomFontStep, a.zoom.pending, a.zoom.pendingSet)
	}

	// Zooming past the max clamps.
	for i := 0; i < 200; i++ {
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	}
	if a.zoom.pending != zoomFontMax {
		t.Fatalf("expected clamp to %v, got %v", zoomFontMax, a.zoom.pending)
	}

	// zoomTarget falls back to the live size once nothing is pending.
	a.zoom.pendingSet = false
	if got := a.zoomTarget(); got != a.cfg.Font.Size {
		t.Fatalf("zoomTarget should fall back to live size %v, got %v", a.cfg.Font.Size, got)
	}
}

func TestHandleScrollKeyShiftOnly(t *testing.T) {
	a := &App{term: core.NewTerminal(20, 10)}
	for i := 0; i < 100; i++ {
		a.term.NewLine()
	}
	if a.term.ScrollbackLines() == 0 {
		t.Fatalf("expected scrollback to accumulate")
	}
	if a.term.DisplayOffset() != 0 {
		t.Fatalf("expected viewport at bottom, offset=%d", a.term.DisplayOffset())
	}

	// Plain PageUp is not a scroll chord: it must fall through (return false) so
	// the shell receives it, and it must not move the viewport.
	if a.handleScrollKey(glfw.KeyPageUp, 0) {
		t.Fatalf("plain PageUp should not be consumed by handleScrollKey")
	}
	if a.term.DisplayOffset() != 0 {
		t.Fatalf("plain PageUp must not scroll, offset=%d", a.term.DisplayOffset())
	}

	// Shift+PageUp scrolls up into history.
	if !a.handleScrollKey(glfw.KeyPageUp, glfw.ModShift) {
		t.Fatalf("Shift+PageUp should be consumed")
	}
	afterPageUp := a.term.DisplayOffset()
	if afterPageUp <= 0 {
		t.Fatalf("Shift+PageUp should scroll up, offset=%d", afterPageUp)
	}

	// Shift+Home jumps to the oldest row.
	a.handleScrollKey(glfw.KeyHome, glfw.ModShift)
	atTop := a.term.DisplayOffset()
	if atTop < afterPageUp {
		t.Fatalf("Shift+Home should scroll to the top, offset=%d (page up was %d)", atTop, afterPageUp)
	}

	// Shift+PageDown moves back toward the present.
	a.handleScrollKey(glfw.KeyPageDown, glfw.ModShift)
	if a.term.DisplayOffset() >= atTop {
		t.Fatalf("Shift+PageDown should scroll down, offset=%d (top was %d)", a.term.DisplayOffset(), atTop)
	}

	// Shift+End returns to the live prompt.
	a.handleScrollKey(glfw.KeyEnd, glfw.ModShift)
	if a.term.DisplayOffset() != 0 {
		t.Fatalf("Shift+End should return to the bottom, offset=%d", a.term.DisplayOffset())
	}

	// Adding Ctrl means it is no longer the scroll chord (leaves it for the app).
	if a.handleScrollKey(glfw.KeyPageUp, glfw.ModShift|glfw.ModControl) {
		t.Fatalf("Ctrl+Shift+PageUp should not be consumed by handleScrollKey")
	}
}
