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
	if err := a.writePaneInputBytesResult(pane, data); err != nil {
		a.Notify("input: " + err.Error())
	}
}

func (a *App) writePaneInputBytesResult(pane termmux.PaneID, data []byte) error {
	if pane == 0 || a.mux == nil {
		return termmux.ErrPaneNotFound
	}
	events, err := a.mux.Write(pane, data)
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := a.mux.PaneView(pane); ok && view.State == termmux.PaneStateFailed {
			events, err = a.mux.FeedFallback(pane, data)
		}
	}
	if len(events) > 0 {
		a.pendingMuxEvents = append(a.pendingMuxEvents, events...)
		a.requestRedraw()
	}
	return err
}

func (a *App) writeInput(s string) { a.writeInputBytes([]byte(s)) }
