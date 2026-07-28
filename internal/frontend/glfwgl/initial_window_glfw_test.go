//go:build glfw

package glfwgl

import (
	"testing"
	"time"

	termmux "cervterm/internal/mux"
)

func TestInitialDoublePresentationUsesNormalFrameAccounting(t *testing.T) {
	const id termmux.WindowID = initialWindowID
	firstPresentedAt := time.Unix(123, 456)
	visiblePresentedAt := firstPresentedAt.Add(time.Millisecond)
	projection := &App{needsRedraw: true}
	controller := &windowController{windows: map[termmux.WindowID]*windowProjection{
		id: {id: id, app: projection, dirty: true},
	}}
	owner := &App{controller: controller}

	owner.acknowledgePresentedFrame(id, projection, firstPresentedAt)
	controller.windows[id].dirty = true
	projection.needsRedraw = true
	owner.acknowledgePresentedFrame(id, projection, visiblePresentedAt)

	if controller.windows[id].dirty {
		t.Fatal("startup damage remained dirty after presentation")
	}
	if projection.needsRedraw {
		t.Fatal("startup redraw demand remained set after presentation")
	}
	if !projection.presentation.last.Equal(visiblePresentedAt) {
		t.Fatalf("presentation time=%v want=%v", projection.presentation.last, visiblePresentedAt)
	}
	if frames := projection.meter.Frames(); frames != 2 {
		t.Fatalf("frames=%d want=2", frames)
	}
}
