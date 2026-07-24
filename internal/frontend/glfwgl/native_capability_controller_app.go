//go:build glfw

package glfwgl

import (
	"fmt"

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
	return a.app.controller.adoptProjectionBundle(termmux.WindowID(initialWindowID), a.bundle)
}

func (a *initialNativeCapabilityAdapter) rollbackInitialCapabilities() error {
	return a.bundle.beforeUnbind.close()
}

func (a *childNativeCapabilityAdapter) activateChildCapabilities() error {
	a.app.activateProjectionIME(a.window, a.bundle.beforeUnbind)
	return nil
}

func (a *childNativeCapabilityAdapter) bindChildCapabilities(id termmux.WindowID) error {
	if id == 0 {
		return fmt.Errorf("bind projection: %w", errWindowProjectionMissing)
	}
	a.app.windowID = id
	if accessibilityErr := prepareProjectionAccessibility(a.app, a.window, a.bundle.beforeUnbind); accessibilityErr != nil {
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
	a.app.windowID = 0
	return a.bundle.beforeUnbind.close()
}
