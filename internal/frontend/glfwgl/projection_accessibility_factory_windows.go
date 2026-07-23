//go:build glfw && windows

package glfwgl

import (
	"fmt"

	"cervterm/internal/accessibility"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func init() {
	projectionAccessibilityFactory = prepareWindowsProjectionAccessibility
}

func prepareWindowsProjectionAccessibility(app *App, window *glfw.Window, before *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
	if app == nil || window == nil || before == nil || app.cfg.Accessibility.Scope != "visible" {
		return nil, errProjectionAccessibilityInvalid
	}
	hwnd, err := glfwWindowHWND(window)
	if err != nil {
		return nil, fmt.Errorf("accessibility native window unavailable")
	}
	capture := func(generation uint64) (projectionAccessibilitySnapshot, error) {
		nativeVisible := window.GetAttrib(glfw.Visible) == glfw.True && window.GetAttrib(glfw.Iconified) != glfw.True
		document, ok, captureErr := app.captureAccessibilityDocumentVisibility(generation, nativeVisible)
		if captureErr != nil {
			return projectionAccessibilitySnapshot{}, captureErr
		}
		if !ok {
			return projectionAccessibilitySnapshot{}, errProjectionAccessibilityInvalid
		}
		x, y := window.GetPos()
		width, height := window.GetFramebufferSize()
		if width <= 0 || height <= 0 {
			return projectionAccessibilitySnapshot{}, errProjectionAccessibilityInvalid
		}
		return projectionAccessibilitySnapshot{
			Document: document, ScreenX: float64(x), ScreenY: float64(y),
			Bounds: accessibility.Rect{X: float64(x), Y: float64(y), Width: float64(width), Height: float64(height)},
		}, nil
	}
	initial, err := capture(1)
	if err != nil {
		return nil, err
	}
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: initial.Document, ScreenX: initial.ScreenX, ScreenY: initial.ScreenY, Bounds: initial.Bounds,
		HWND: hwnd, API: windowsUIANativeAPI{}, Report: func(error) { app.Notify("accessibility unavailable; continuing without native integration") },
	}, before)
	if lifecycle != nil {
		lifecycle.capture = capture
		lifecycle.generation = 1
		lifecycle.semanticEventSink = lifecycle.native.RaiseSemanticEvent
	}
	return lifecycle, err
}
