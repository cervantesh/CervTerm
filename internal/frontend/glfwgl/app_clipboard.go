//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/input"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) handleClipboardKey(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&glfw.ModControl != 0 && key == glfw.KeyV {
		text := a.window.GetClipboardString()
		a.writeInputBytes(input.EncodePaste(text, a.bracketedPasteMode()))
		return true
	}
	if mods&glfw.ModShift != 0 && key == glfw.KeyInsert {
		text := a.window.GetClipboardString()
		a.writeInputBytes(input.EncodePaste(text, a.bracketedPasteMode()))
		return true
	}
	if mods&glfw.ModControl != 0 && key == glfw.KeyInsert {
		_ = a.copySelectionToClipboard()
		return true
	}
	return false
}

func (a *App) copySelectionToClipboard() bool {
	text := a.Selection()
	if text == "" {
		return false
	}
	a.SetClipboard(text)
	return true
}

func (a *App) writeInputBytes(data []byte) {
	if a.focusedPane == 0 {
		return
	}
	events, err := a.mux.Write(a.focusedPane, data)
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := a.mux.PaneView(a.focusedPane); ok && view.State == termmux.PaneStateFailed {
			events, err = a.mux.FeedFallback(a.focusedPane, data)
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
