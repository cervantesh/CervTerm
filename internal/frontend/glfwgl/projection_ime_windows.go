//go:build glfw && windows

package glfwgl

import (
	"errors"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	projectionIMEWindowHWND = glfwWindowHWND
	projectionIMEComponents = func(app *App, hwnd uintptr, report func(error)) (*immDecoder, *windowsWndProcHost) {
		decoder := newDormantIMMDecoder(app, hwnd, windowsIMMContextAPI{})
		return decoder, &windowsWndProcHost{backend: nativeWndProcBackend{}, decoder: decoder, hwnd: hwnd, report: report}
	}
)

func (a *App) activateProjectionIME(window *glfw.Window, before *compositionBeforeUnbind) projectionIMEActivation {
	if a == nil || !a.cfg.IME.Enabled {
		activation := projectionIMEActivation{State: imeActivationDisabled}
		if a != nil {
			a.recordIMEActivation(activation)
		}
		return activation
	}
	hwnd, err := projectionIMEWindowHWND(window)
	if err != nil {
		activation := projectionIMEActivation{State: imeActivationFallback, Err: err}
		a.recordIMEActivation(activation)
		return activation
	}
	reported := false
	decoder, host := projectionIMEComponents(a, hwnd, func(error) {
		if !reported {
			reported = true
			a.Notify("native IME input failed; composition was cancelled")
		}
	})
	if err := before.attachWndProcHost(host); err != nil {
		activation := projectionIMEActivation{State: imeActivationFallback, Err: err}
		a.recordIMEActivation(activation)
		return activation
	}
	if err := attachIMECandidateCleanup(a, before); err != nil {
		activation := projectionIMEActivation{State: imeActivationFallback, Err: err}
		a.recordIMEActivation(activation)
		return activation
	}
	if err := host.install(); err != nil {
		activation := projectionIMEActivation{State: imeActivationFallback, Err: err}
		a.recordIMEActivation(activation)
		return activation
	}
	if err := a.setCandidateGeometryCallbacks(decoder.publishCandidate, decoder.clearCandidate); err != nil {
		cleanupErr := errors.Join(host.deactivate(), host.restore(), host.release())
		activation := projectionIMEActivation{State: imeActivationFallback, Err: errors.Join(err, cleanupErr)}
		a.recordIMEActivation(activation)
		return activation
	}
	activation := projectionIMEActivation{State: imeActivationActive}
	a.recordIMEActivation(activation)
	return activation
}
