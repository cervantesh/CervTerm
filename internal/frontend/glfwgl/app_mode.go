//go:build glfw

package glfwgl

func (a *App) bracketedPasteMode() bool {
	_, view, ok := a.focusedView()
	return ok && view.BracketedPaste
}
