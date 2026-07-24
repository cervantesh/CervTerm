//go:build glfw

package glfwgl

import (
	"errors"

	termmux "cervterm/internal/mux"
)

const nativeCapabilityControllerPortBudget = 8

type nativeInitialCapabilityPort interface {
	activateInitialIME()
	prepareInitialAccessibility() error
	adoptInitialCapabilities() error
	rollbackInitialCapabilities() error
}

type nativeChildCapabilityPort interface {
	activateChildCapabilities() error
	bindChildCapabilities(termmux.WindowID) error
	markChildCapabilitiesReady()
	rollbackChildCapabilities() error
}

// nativeCapabilityController owns capability activation ordering only. It
// retains narrow ports and never owns a window, projection bundle, WndProc,
// prepared object, GPU/GL object, or rollback resource.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type nativeCapabilityController struct {
	initial nativeInitialCapabilityPort
	child   nativeChildCapabilityPort
}

func newNativeCapabilityController(initial nativeInitialCapabilityPort, child nativeChildCapabilityPort) *nativeCapabilityController {
	return &nativeCapabilityController{initial: initial, child: child}
}

func (c *nativeCapabilityController) activateInitial() error {
	c.initial.activateInitialIME()
	if err := c.initial.prepareInitialAccessibility(); err != nil {
		return errors.Join(err, c.initial.rollbackInitialCapabilities())
	}
	if err := c.initial.adoptInitialCapabilities(); err != nil {
		return errors.Join(err, c.initial.rollbackInitialCapabilities())
	}
	return nil
}

func (c *nativeCapabilityController) activateChild(id termmux.WindowID) error {
	if err := c.child.activateChildCapabilities(); err != nil {
		return errors.Join(err, c.child.rollbackChildCapabilities())
	}
	if err := c.child.bindChildCapabilities(id); err != nil {
		return errors.Join(err, c.child.rollbackChildCapabilities())
	}
	c.child.markChildCapabilitiesReady()
	return nil
}
