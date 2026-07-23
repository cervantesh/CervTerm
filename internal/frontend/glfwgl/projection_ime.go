//go:build glfw

package glfwgl

import "errors"

type imeActivationState uint8

const (
	imeActivationDisabled imeActivationState = iota
	imeActivationActive
	imeActivationFallback
	imeActivationUnsupported
)

type projectionIMEActivation struct {
	State imeActivationState
	Err   error
}

func (a *App) recordIMEActivation(activation projectionIMEActivation) {
	if a == nil {
		return
	}
	a.imeActivation = activation
	if activation.State == imeActivationFallback {
		a.Notify("native IME unavailable; using GLFW text input")
	} else if activation.State == imeActivationUnsupported {
		a.Notify("native IME is unsupported on this platform; using GLFW text input")
	}
}

func attachIMECandidateCleanup(app *App, before *compositionBeforeUnbind) error {
	if app == nil || before == nil || before.done {
		return errWndProcHostInvalid
	}
	deactivate := before.deactivate
	before.deactivate = func() error {
		return errors.Join(callCompositionCleanupStep(deactivate), app.candidateGeometry.detachCallbacks())
	}
	return nil
}
