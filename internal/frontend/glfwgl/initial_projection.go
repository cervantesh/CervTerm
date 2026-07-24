//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/ime"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) closeUnadoptedProjectionResources() {
	if a != nil && a.window != nil {
		a.window.MakeContextCurrent()
	}
	if a != nil {
		_ = a.cancelComposition(ime.CancelTeardown)
		a.composition.deactivateDelivery()
		a.charSuppression.clear()
	}
	a.closeDividerCursors()
	_ = a.closeTerminalImageCache()
	if a.atlas != nil {
		a.atlas.close()
		a.atlas = nil
	}
	a.closeBackgroundSurface()
	if a.r != nil {
		a.r.Destroy()
		a.r = nil
	}
	if a.blurProvider != nil {
		_ = a.blurProvider.Close()
		a.blurProvider = nil
	}
	_ = a.closeNotificationEffectSink()
}

func (a *App) adoptInitialProjection(window *glfw.Window) error {
	bundle := &nativeProjectionBundle{
		host: window, app: a, handle: a.applyMuxEvents,
		beforeUnbind: newCompositionBeforeUnbind(a),
		resources: []projectionResource{
			projectionResourceFunc(func() error {
				a.closeDividerCursors()
				if a.blurProvider == nil {
					return nil
				}
				err := a.blurProvider.Close()
				a.blurProvider = nil
				return err
			}),
			projectionResourceFunc(func() error {
				if a.r != nil {
					a.r.Destroy()
					a.r = nil
				}
				return nil
			}),
			projectionResourceFunc(func() error { a.closeBackgroundSurface(); return nil }),
			projectionResourceFunc(func() error {
				if a.atlas != nil {
					a.atlas.close()
					a.atlas = nil
				}
				return nil
			}),
		},
	}
	appendTerminalImageCacheResource(bundle, a)
	bundle.resources = append(bundle.resources, projectionResourceFunc(a.closeNotificationEffectSink))
	capabilities := &initialNativeCapabilityAdapter{app: a, window: window, bundle: bundle}
	capabilities.activateInitialIME()
	if accessibilityErr := capabilities.prepareInitialAccessibility(); accessibilityErr != nil {
		return errors.Join(accessibilityErr, capabilities.rollbackInitialCapabilities())
	}
	if err := capabilities.adoptInitialCapabilities(); err != nil {
		return errors.Join(err, capabilities.rollbackInitialCapabilities())
	}
	return nil
}
