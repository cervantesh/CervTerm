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
	before := a.modal.Revision()
	switch key {
	case glfw.KeyEscape:
		a.applyModalIntents(a.closeModal())
	case glfw.KeyEnter, glfw.KeyKPEnter:
		intents := a.modal.Accept()
		if eventAction == glfw.Press && a.modal.Mode() == modal.ModeQuickSelect && len(intents) != 0 {
			a.quickSelect.userActivation = true
		}
		a.applyModalIntents(intents)
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
	if a.modal.Mode() == modal.ModeQuickSelect {
		return a.handleQuickSelectChar(char)
	}
	before := a.modal.Revision()
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
	before := a.modal.Revision()
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
	if a.modal.Revision() != before {
		a.requestRedraw()
	}
}

func (a *App) applyModalIntents(intents []modal.Intent) {
	for _, intent := range intents {
		switch intent.Kind {
		case modal.IntentAccept:
			if a.modal.Mode() != modal.ModeCommandPalette && a.modal.Mode() != modal.ModeQuickSelect && a.modal.Mode() != modal.ModeLaunchMenu && a.modal.Mode() != modal.ModeTabSwitcher && a.modal.Mode() != modal.ModeWorkspaceSwitcher && a.modal.Mode() != modal.ModeTabCloseConfirmation {
				continue
			}
			var err error
			switch a.modal.Mode() {
			case modal.ModeQuickSelect:
				err = a.acceptQuickSelect(intent.Entry, termmux.PaneID(intent.Pane))
			case modal.ModeLaunchMenu:
				err = a.acceptLaunchMenu(intent.Entry, termmux.PaneID(intent.Pane))
			case modal.ModeTabSwitcher:
				err = a.acceptCommandPalette(intent.Entry, termmux.PaneID(intent.Pane))
			case modal.ModeWorkspaceSwitcher:
				err = a.acceptWorkspaceSwitcher(intent.Entry)
			case modal.ModeTabCloseConfirmation:
				err = a.acceptTabClose(intent.Entry)
			default:
				err = a.acceptCommandPalette(intent.Entry, termmux.PaneID(intent.Pane))
			}
			if err != nil {
				a.modal.SetError(err.Error())
				a.requestRedraw()
				continue
			}
			a.applyModalIntents(a.closeModal())
		case modal.IntentRestoreFocus:
			if intent.Pane != 0 && a.mux != nil {
				_ = a.focusPane(termmux.PaneID(intent.Pane))
			}
		}
	}
}
