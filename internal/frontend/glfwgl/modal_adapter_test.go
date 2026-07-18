//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/modal"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func openTestModal(t *testing.T, app *App) {
	t.Helper()
	if !app.modal.Open(modal.ModeCommandPalette, 7, 7, []modal.Entry{{ID: "copy", Label: "Copy"}, {ID: "paste", Label: "Paste"}}) {
		t.Fatal("open modal")
	}
}

func TestInactiveModalAdapterIsNoOp(t *testing.T) {
	a := &App{rows: 10}
	if a.handleModalKey(glfw.KeyA, glfw.Press, 0) || a.handleModalChar('a') || a.handleModalMouseButton(glfw.MouseButtonLeft, glfw.Press, 0) || a.handleModalCursorPos(1, 2) || a.handleModalScroll(0, 1) {
		t.Fatal("inactive modal consumed input")
	}
}

func TestActiveModalAdapterConsumesEveryInputPath(t *testing.T) {
	a := &App{rows: 10}
	openTestModal(t, a)
	if !a.handleModalChar('p') {
		t.Fatal("char leaked")
	}
	if got := string(a.modal.Snapshot().Query); got != "p" {
		t.Fatalf("query=%q", got)
	}
	if !a.handleModalKey(glfw.KeyDown, glfw.Press, 0) {
		t.Fatal("key leaked")
	}
	if !a.handleModalMouseButton(glfw.MouseButtonLeft, glfw.Press, 0) {
		t.Fatal("press leaked")
	}
	if !a.handleModalMouseButton(glfw.MouseButtonLeft, glfw.Release, 0) {
		t.Fatal("release leaked")
	}
	if !a.handleModalCursorPos(2, 3) {
		t.Fatal("drag leaked")
	}
	if !a.handleModalScroll(0, -1) {
		t.Fatal("wheel leaked")
	}
	if !a.handleModalKey(glfw.KeyEscape, glfw.Press, 0) || a.modal.Active() {
		t.Fatal("escape did not consume and close")
	}
}

func TestModalAdapterRequestsRedrawOnlyOnMutation(t *testing.T) {
	a := &App{rows: 10}
	openTestModal(t, a)
	a.needsRedraw = false
	if !a.handleModalKey(glfw.KeyUp, glfw.Press, 0) {
		t.Fatal("key not consumed")
	}
	if a.needsRedraw {
		t.Fatal("clamped navigation requested redraw")
	}
	if !a.handleModalChar('x') || !a.needsRedraw {
		t.Fatal("query mutation did not request redraw")
	}
}
