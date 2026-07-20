//go:build glfw

package glfwgl

import (
	"errors"

	termmux "cervterm/internal/mux"
)

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
