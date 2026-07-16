//go:build glfw

package glfwgl

import (
	"testing"
	"time"

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

func TestZoomArmsDebounceWithoutApplyingLiveSize(t *testing.T) {
	a := &App{}
	a.cfg.Font.Size = 14

	// applyFontSize only records the target + a future PTY-resize deadline and
	// asks for a redraw; the visual rebuild happens on the loop thread (needs GL)
	// so the live font size must not change here.
	a.applyFontSize(15)
	if a.cfg.Font.Size != 14 {
		t.Fatalf("applyFontSize must not change the live size, got %v", a.cfg.Font.Size)
	}
	if !a.zoom.pendingSet || a.zoom.pending != 15 {
		t.Fatalf("pending not recorded: set=%v pending=%v", a.zoom.pendingSet, a.zoom.pending)
	}
	if !a.needsRedraw {
		t.Fatalf("a zoom step should request a redraw")
	}
	if !a.zoom.deadline.After(time.Now()) {
		t.Fatalf("a zoom step should push the PTY-resize deadline into the future")
	}
}

func TestHandleScrollKeyShiftOnly(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	for i := 0; i < 100; i++ {
		feedTestPane(t, a, []byte("\n"))
	}
	_, view, _ := a.focusedView()
	if view.ScrollbackLines == 0 {
		t.Fatalf("expected scrollback to accumulate")
	}
	if view.DisplayOffset != 0 {
		t.Fatalf("expected viewport at bottom, offset=%d", view.DisplayOffset)
	}

	if a.handleScrollKey(glfw.KeyPageUp, 0) {
		t.Fatalf("plain PageUp should not be consumed by handleScrollKey")
	}
	_, view, _ = a.focusedView()
	if view.DisplayOffset != 0 {
		t.Fatalf("plain PageUp must not scroll, offset=%d", view.DisplayOffset)
	}

	if !a.handleScrollKey(glfw.KeyPageUp, glfw.ModShift) {
		t.Fatalf("Shift+PageUp should be consumed")
	}
	_, view, _ = a.focusedView()
	afterPageUp := view.DisplayOffset
	if afterPageUp <= 0 {
		t.Fatalf("Shift+PageUp should scroll up, offset=%d", afterPageUp)
	}

	a.handleScrollKey(glfw.KeyHome, glfw.ModShift)
	_, view, _ = a.focusedView()
	atTop := view.DisplayOffset
	if atTop < afterPageUp {
		t.Fatalf("Shift+Home should scroll to the top, offset=%d (page up was %d)", atTop, afterPageUp)
	}

	a.handleScrollKey(glfw.KeyPageDown, glfw.ModShift)
	_, view, _ = a.focusedView()
	if view.DisplayOffset >= atTop {
		t.Fatalf("Shift+PageDown should scroll down, offset=%d (top was %d)", view.DisplayOffset, atTop)
	}

	a.handleScrollKey(glfw.KeyEnd, glfw.ModShift)
	_, view, _ = a.focusedView()
	if view.DisplayOffset != 0 {
		t.Fatalf("Shift+End should return to the bottom, offset=%d", view.DisplayOffset)
	}

	if a.handleScrollKey(glfw.KeyPageUp, glfw.ModShift|glfw.ModControl) {
		t.Fatalf("Ctrl+Shift+PageUp should not be consumed by handleScrollKey")
	}
}
