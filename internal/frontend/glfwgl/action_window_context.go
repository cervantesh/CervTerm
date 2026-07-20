//go:build glfw

package glfwgl

import termaction "cervterm/internal/action"

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
