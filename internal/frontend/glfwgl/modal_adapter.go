//go:build glfw

package glfwgl

import (
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// modalCoordinator is deliberately unreachable from public actions, configuration,
// and bindings in Slice 7.1. Tests in this package may seed App.modal directly.
func (a *App) handleModalKey(key glfw.Key, eventAction glfw.Action, _ glfw.ModifierKey) bool {
	if !a.modal.Active() {
		return false
	}
	if eventAction != glfw.Press && eventAction != glfw.Repeat {
		return true
	}
	before := a.modal.Snapshot().Revision
	switch key {
	case glfw.KeyEscape:
		a.applyModalIntents(a.modal.Close())
	case glfw.KeyEnter, glfw.KeyKPEnter:
		// Acceptance is intentionally only a pure retained intent in this slice.
		_ = a.modal.Accept()
	case glfw.KeyBackspace:
		a.modal.Backspace()
	case glfw.KeyUp:
		a.modal.Move(-1)
	case glfw.KeyDown:
		a.modal.Move(1)
	case glfw.KeyPageUp:
		a.modal.Page(-1, a.modalVisibleRows())
	case glfw.KeyPageDown:
		a.modal.Page(1, a.modalVisibleRows())
	case glfw.KeyHome:
		a.modal.Move(-modal.MaxEntries)
	case glfw.KeyEnd:
		a.modal.Move(modal.MaxEntries)
	}
	a.redrawModalMutation(before)
	return true
}

func (a *App) handleModalChar(char rune) bool {
	if !a.modal.Active() {
		return false
	}
	before := a.modal.Snapshot().Revision
	a.modal.AppendRune(char)
	a.redrawModalMutation(before)
	return true
}

func (a *App) handleModalMouseButton(_ glfw.MouseButton, _ glfw.Action, _ glfw.ModifierKey) bool {
	return a.modal.Active()
}

func (a *App) handleModalCursorPos(_, _ float64) bool { return a.modal.Active() }

func (a *App) handleModalScroll(_, yoff float64) bool {
	if !a.modal.Active() {
		return false
	}
	before := a.modal.Snapshot().Revision
	delta := -1
	if yoff < 0 {
		delta = 1
	}
	if yoff != 0 {
		a.modal.Scroll(delta, a.modalVisibleRows())
	}
	a.redrawModalMutation(before)
	return true
}

func (a *App) modalVisibleRows() int {
	rows := a.rows - 2
	if rows < 1 {
		return 1
	}
	return rows
}

func (a *App) redrawModalMutation(before uint64) {
	if a.modal.Snapshot().Revision != before {
		a.requestRedraw()
	}
}

func (a *App) applyModalIntents(intents []modal.Intent) {
	for _, intent := range intents {
		if intent.Kind == modal.IntentRestoreFocus && intent.Pane != 0 && a.mux != nil {
			_ = a.focusPane(termmux.PaneID(intent.Pane))
		}
	}
}
