//go:build glfw

package glfwgl

import (
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestZoomCoalescesAndCompoundsPerPane(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	a.cfg.Font.Size = 14
	a.zoom.base = 14
	state := a.ensurePaneUI(a.focusedPane)
	state.font.fontSize = 14

	for i := 0; i < 5; i++ {
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	}
	if state.font.fontSize != 14 {
		t.Fatalf("live font size changed before applyPendingZoom: %v", state.font.fontSize)
	}
	if !state.font.pending || state.font.pendingTarget != 14+5*zoomFontStep {
		t.Fatalf("expected pending %v, got %v (set=%v)", 14+5*zoomFontStep, state.font.pendingTarget, state.font.pending)
	}

	for i := 0; i < 200; i++ {
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	}
	if state.font.pendingTarget != zoomFontMax {
		t.Fatalf("expected clamp to %v, got %v", zoomFontMax, state.font.pendingTarget)
	}

	state.font.pending = false
	if got := a.zoomTarget(); got != state.font.fontSize {
		t.Fatalf("zoomTarget should fall back to live size %v, got %v", state.font.fontSize, got)
	}
}

func TestZoomArmsPaneDebounceWithoutApplyingLiveSize(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	a.cfg.Font.Size = 14
	state := a.ensurePaneUI(a.focusedPane)
	state.font.fontSize = 14

	a.applyFontSize(15)
	if state.font.fontSize != 14 {
		t.Fatalf("applyFontSize must not change the live size, got %v", state.font.fontSize)
	}
	if !state.font.pending || state.font.pendingTarget != 15 {
		t.Fatalf("pending not recorded: set=%v pending=%v", state.font.pending, state.font.pendingTarget)
	}
	if !a.needsRedraw {
		t.Fatalf("a zoom step should request a redraw")
	}
	if !state.font.deadline.After(time.Now()) {
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
