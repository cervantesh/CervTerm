//go:build glfw

package glfwgl

import (
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Native capability adapters are ephemeral views over the authoritative App,
// window, and projection bundle. They never own or copy lifecycle resources.
type initialNativeCapabilityAdapter struct {
	app    *App
	window *glfw.Window
	bundle *nativeProjectionBundle
}

type childNativeCapabilityAdapter struct {
	app    *App
	host   *windowController
	window *glfw.Window
	bundle *nativeProjectionBundle
}

var (
	_ nativeInitialCapabilityPort = (*initialNativeCapabilityAdapter)(nil)
	_ nativeChildCapabilityPort   = (*childNativeCapabilityAdapter)(nil)
)

func (a *initialNativeCapabilityAdapter) activateInitialIME() {
	a.app.activateProjectionIME(a.window, a.bundle.beforeUnbind)
}

func (a *initialNativeCapabilityAdapter) prepareInitialAccessibility() error {
	return prepareProjectionAccessibility(a.app, a.window, a.bundle.beforeUnbind)
}

func (a *initialNativeCapabilityAdapter) adoptInitialCapabilities() error {
	if a.app == nil || a.app.host == nil {
		return errWindowProjectionMissing
	}
	return a.app.host.adoptProjectionBundle(termmux.WindowID(initialWindowID), a.bundle)
}

func (a *initialNativeCapabilityAdapter) rollbackInitialCapabilities() error {
	return a.bundle.beforeUnbind.close()
}

func (a *childNativeCapabilityAdapter) activateChildCapabilities() error {
	a.app.activateProjectionIME(a.window, a.bundle.beforeUnbind)
	return nil
}

func (a *childNativeCapabilityAdapter) bindChildCapabilities(id termmux.WindowID) error {
	if id == 0 || a.app == nil || a.host == nil || a.window == nil {
		return errWindowProjectionMissing
	}
	a.app.windowID = id
	window, identity, err := a.host.acquireWindowCapability(id, a.app, a.window)
	if err != nil {
		a.app.windowID = 0
		return err
	}
	a.app.mux = window
	a.app.windowIdentity = identity
	if accessibilityErr := prepareProjectionAccessibility(a.app, a.window, a.bundle.beforeUnbind); accessibilityErr != nil {
		delete(a.host.boundOrigins, identity)
		a.app.mux = nil
		a.app.windowIdentity = termmux.WindowIdentity{}
		a.app.windowID = 0
		return accessibilityErr
	}
	return nil
}

func (a *childNativeCapabilityAdapter) markChildCapabilitiesReady() {
	a.app.catchUpBellEvents()
	a.app.installCallbacks()
	a.app.needsRedraw = true
}

func (a *childNativeCapabilityAdapter) rollbackChildCapabilities() error {
	if a.app != nil && a.host != nil {
		delete(a.host.boundOrigins, a.app.windowIdentity)
	}
	a.app.mux = nil
	a.app.windowIdentity = termmux.WindowIdentity{}
	a.app.windowID = 0
	return nil
}
