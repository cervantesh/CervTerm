//go:build glfw

package glfwgl

import (
	"errors"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var errProjectionAccessibilityInvalid = errors.New("projection accessibility lifecycle is invalid")

type projectionAccessibilityLifecycle interface {
	Close() error
}

type projectionAccessibilityFactoryFunc func(app *App, window *glfw.Window, before *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error)

// projectionAccessibilityFactory remains nil until the default-off activation
// slice installs a production factory. Tests may inject the dormant lifecycle.
var projectionAccessibilityFactory projectionAccessibilityFactoryFunc

func prepareProjectionAccessibility(app *App, window *glfw.Window, before *compositionBeforeUnbind) error {
	if projectionAccessibilityFactory == nil {
		return nil
	}
	if app == nil || window == nil || before == nil {
		return errProjectionAccessibilityInvalid
	}
	lifecycle, err := projectionAccessibilityFactory(app, window, before)
	if err != nil {
		if lifecycle != nil {
			return errors.Join(err, lifecycle.Close())
		}
		return err
	}
	if lifecycle == nil {
		return errProjectionAccessibilityInvalid
	}
	if err := before.attachBeforeNative(lifecycle.Close); err != nil {
		return errors.Join(err, lifecycle.Close())
	}
	return nil
}
