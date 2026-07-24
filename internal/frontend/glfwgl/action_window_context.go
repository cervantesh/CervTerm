//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

func (a *App) windowActionRef() termaction.Ref {
	if a.windowID == 0 {
		return termaction.Ref{}
	}
	return termaction.Ref{Kind: termaction.RefWindow, ID: uint64(a.windowID)}
}

func (a *App) refreshFocusedActionContext(context termaction.Context) termaction.Context {
	projection := a
	if a.controller != nil {
		if active := a.controller.activeProjectionApp(); active != nil {
			projection = active
		}
	}
	context.Focused = projection.focusedActionRef()
	context.FocusedWindow = projection.windowActionRef()
	return context
}

func (a *App) activeActionRoute() actionExecutionRoute {
	if a.controller != nil {
		if active := a.controller.activeProjectionApp(); active != nil {
			return active.ensureActionController()
		}
	}
	return a.ensureActionController()
}

func (a *App) actionPaneExists(pane termmux.PaneID) bool {
	_, exists := a.mux.PaneView(pane)
	return exists
}
