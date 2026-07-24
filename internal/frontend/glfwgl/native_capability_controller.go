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

// nativeCapabilityController owns capability activation ordering only. Initial
// and child adapters are operation-scoped and are never retained with their
// windows, projection bundles, WndProcs, prepared objects, or rollback resources.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type nativeCapabilityController struct{}

func newNativeCapabilityController() nativeCapabilityController {
	return nativeCapabilityController{}
}

func (nativeCapabilityController) activateInitial(initial nativeInitialCapabilityPort) error {
	initial.activateInitialIME()
	if err := initial.prepareInitialAccessibility(); err != nil {
		return errors.Join(err, initial.rollbackInitialCapabilities())
	}
	if err := initial.adoptInitialCapabilities(); err != nil {
		return errors.Join(err, initial.rollbackInitialCapabilities())
	}
	return nil
}

func (nativeCapabilityController) prepareChild(child nativeChildCapabilityPort) error {
	if err := child.activateChildCapabilities(); err != nil {
		return errors.Join(err, child.rollbackChildCapabilities())
	}
	return nil
}

func (nativeCapabilityController) bindChild(child nativeChildCapabilityPort, id termmux.WindowID) error {
	if err := child.bindChildCapabilities(id); err != nil {
		return errors.Join(err, child.rollbackChildCapabilities())
	}
	child.markChildCapabilitiesReady()
	return nil
}

func (c nativeCapabilityController) activateChild(child nativeChildCapabilityPort, id termmux.WindowID) error {
	if err := c.prepareChild(child); err != nil {
		return err
	}
	return c.bindChild(child, id)
}
