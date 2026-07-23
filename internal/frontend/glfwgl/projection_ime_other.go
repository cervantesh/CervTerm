//go:build glfw && !windows

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

func (a *App) activateProjectionIME(_ *glfw.Window, _ *compositionBeforeUnbind) projectionIMEActivation {
	activation := projectionIMEActivation{State: imeActivationDisabled}
	if a != nil && a.cfg.IME.Enabled {
		activation.State = imeActivationUnsupported
	}
	if a != nil {
		a.recordIMEActivation(activation)
	}
	return activation
}
