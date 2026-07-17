//go:build glfw

package glfwgl

import (
	"errors"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) handleClipboardKey(key glfw.Key, mods glfw.ModifierKey) bool {
	var command termaction.Action
	switch {
	case mods&glfw.ModControl != 0 && key == glfw.KeyV:
		command = termaction.PasteClipboard{}
	case mods&glfw.ModShift != 0 && key == glfw.KeyInsert:
		command = termaction.PasteClipboard{}
	case mods&glfw.ModControl != 0 && key == glfw.KeyInsert:
		command = termaction.CopySelection{}
	default:
		return false
	}
	return a.dispatchReservedAction(command, key, mods, false)
}

func (a *App) writeInputBytes(data []byte) {
	a.writePaneInputBytes(a.focusedPane, data)
}

func (a *App) writePaneInputBytes(pane termmux.PaneID, data []byte) {
	if pane == 0 {
		return
	}
	events, err := a.mux.Write(pane, data)
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := a.mux.PaneView(pane); ok && view.State == termmux.PaneStateFailed {
			events, err = a.mux.FeedFallback(pane, data)
		}
	}
	if len(events) > 0 {
		a.pendingMuxEvents = append(a.pendingMuxEvents, events...)
	}
	if err != nil {
		a.Notify("input: " + err.Error())
	}
	if len(events) > 0 {
		a.requestRedraw()
	}
}

func (a *App) writeInput(s string) { a.writeInputBytes([]byte(s)) }
