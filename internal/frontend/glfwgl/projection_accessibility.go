//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/accessibility"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var errProjectionAccessibilityInvalid = errors.New("projection accessibility lifecycle is invalid")

type accessibilityActivationState uint8

const (
	accessibilityActivationDisabled accessibilityActivationState = iota
	accessibilityActivationActive
	accessibilityActivationFallback
	accessibilityActivationUnsupported
)

const accessibilityAllSemanticIntents = accessibility.IntentDocument | accessibility.IntentTopology | accessibility.IntentText | accessibility.IntentCaret | accessibility.IntentSelection | accessibility.IntentFocus

func (a *App) requestAccessibilityRedraw() {
	a.requestRedraw()
	if a.accessibilityRuntime != nil {
		a.accessibilityRuntime.Invalidate(accessibilityAllSemanticIntents)
	}
}

func (a *App) installAccessibilityWindowCallbacks() {
	a.window.SetPosCallback(func(_ *glfw.Window, _, _ int) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
	a.window.SetIconifyCallback(func(_ *glfw.Window, _ bool) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
}

type projectionAccessibilityActivation struct {
	State accessibilityActivationState
	Err   error
}

type projectionAccessibilityLifecycle interface {
	Close() error
}

type projectionAccessibilityRuntime interface {
	projectionAccessibilityLifecycle
	Invalidate(accessibility.SemanticIntent)
	Refresh() error
	Announce(accessibility.AnnouncementKind) error
}

type projectionAccessibilityFactoryFunc func(app *App, window *glfw.Window, before *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error)

// projectionAccessibilityFactory remains nil until the default-off activation
// slice installs a production factory. Tests may inject the dormant lifecycle.
var projectionAccessibilityFactory projectionAccessibilityFactoryFunc

func prepareProjectionAccessibility(app *App, window *glfw.Window, before *compositionBeforeUnbind) error {
	if app == nil {
		return errProjectionAccessibilityInvalid
	}
	if !app.cfg.Accessibility.Enabled {
		app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationDisabled})
		return nil
	}
	if window == nil || before == nil {
		return errProjectionAccessibilityInvalid
	}
	if projectionAccessibilityFactory == nil {
		app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationUnsupported})
		return nil
	}
	lifecycle, err := projectionAccessibilityFactory(app, window, before)
	if err != nil {
		if lifecycle != nil {
			err = errors.Join(err, lifecycle.Close())
		}
		app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationFallback, Err: err})
		return nil
	}
	if lifecycle == nil {
		app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationFallback, Err: errProjectionAccessibilityInvalid})
		return nil
	}
	closeLifecycle := lifecycle.Close
	if runtime, ok := lifecycle.(projectionAccessibilityRuntime); ok {
		app.accessibilityRuntime = runtime
		closeLifecycle = func() error {
			app.accessibilityRuntime = nil
			return lifecycle.Close()
		}
	}
	if err := before.attachBeforeNative(closeLifecycle); err != nil {
		app.accessibilityRuntime = nil
		err = errors.Join(err, lifecycle.Close())
		app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationFallback, Err: err})
		return nil
	}
	app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationActive})
	return nil
}

func (app *App) recordAccessibilityActivation(activation projectionAccessibilityActivation) {
	if app == nil {
		return
	}
	app.accessibilityActivation = activation
	switch activation.State {
	case accessibilityActivationFallback:
		app.Notify("accessibility unavailable; continuing without native integration")
	case accessibilityActivationUnsupported:
		app.Notify("accessibility is unsupported on this platform; continuing without native integration")
	}
}

func (app *App) failAccessibilityRuntime(err error) {
	if app == nil || app.accessibilityRuntime == nil {
		return
	}
	runtime := app.accessibilityRuntime
	app.accessibilityRuntime = nil
	_ = runtime.Close()
	app.recordAccessibilityActivation(projectionAccessibilityActivation{State: accessibilityActivationFallback, Err: err})
}

func (app *App) refreshAccessibilityProjection() {
	if app == nil || app.accessibilityRuntime == nil {
		return
	}
	app.accessibilityRuntime.Invalidate(accessibilityAllSemanticIntents)
	if err := app.accessibilityRuntime.Refresh(); err != nil {
		app.failAccessibilityRuntime(err)
	}
}
