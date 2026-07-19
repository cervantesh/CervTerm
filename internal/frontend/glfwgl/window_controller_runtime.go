//go:build glfw

package glfwgl

import (
	"errors"

	termmux "cervterm/internal/mux"
)

// nativeProjectionCandidateFactory prepares all native/per-window resources
// before a mux runtime window is created. Bind installs WindowID-addressed
// callbacks only after the mux side has succeeded.
type nativeProjectionCandidateFactory interface {
	Prepare() (*nativeProjectionBundle, termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics, string, error)
}

// runtimeWindowLifecycle is the failure seam around the mux transaction. The
// production adapter delegates directly to the process-owned Mux.
type runtimeWindowLifecycle interface {
	CreateWindow(termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics, string) (termmux.WindowView, []termmux.Event, error)
	ActivateWindow(termmux.WindowID) ([]termmux.Event, error)
	CloseWindow(termmux.WindowID) (termmux.CloseWindowResult, []termmux.Event, error)
	RollbackWindow(termmux.WindowID) error
}

func (c *windowController) setCandidateFactory(factory nativeProjectionCandidateFactory) {
	c.candidateFactory = factory
}

func (c *windowController) setRuntimeWindows(runtimeWindows runtimeWindowLifecycle) {
	c.runtimeWindows = runtimeWindows
}

func (c *windowController) createRuntimeProjection() (termmux.WindowID, error) {
	if err := c.requireLoop(); err != nil {
		return 0, err
	}
	if c.candidateFactory == nil || c.runtimeWindows == nil {
		return 0, errWindowProjectionMissing
	}
	bundle, spec, content, metrics, title, err := c.candidateFactory.Prepare()
	if err != nil {
		return 0, errors.Join(err, bundle.close())
	}
	if bundle == nil || bundle.host == nil || bundle.handle == nil {
		return 0, errors.Join(errWindowProjectionMissing, bundle.close())
	}
	view, events, err := c.runtimeWindows.CreateWindow(spec, content, metrics, title)
	if err != nil {
		return 0, errors.Join(err, bundle.close())
	}
	rollback := func(cause error) error {
		return errors.Join(cause, c.runtimeWindows.RollbackWindow(view.ID), bundle.close())
	}
	if bundle.bind != nil {
		if err := bundle.bind(view.ID); err != nil {
			return 0, rollback(err)
		}
	}
	if err := c.attachApp(view.ID, bundle.host, bundle.app, bundle.handle); err != nil {
		return 0, rollback(err)
	}
	c.windows[view.ID].bundle = bundle
	if err := c.focus(view.ID); err != nil {
		return 0, errors.Join(err, c.closeProjection(view.ID), c.runtimeWindows.RollbackWindow(view.ID))
	}
	c.dispatch(events)
	return view.ID, nil
}

func (c *windowController) closeRuntimeProjection(id termmux.WindowID) (termmux.CloseWindowResult, error) {
	if err := c.requireLoop(); err != nil {
		return termmux.CloseWindowResult{}, err
	}
	if c.runtimeWindows == nil {
		return termmux.CloseWindowResult{}, errWindowProjectionMissing
	}
	result, events, runtimeErr := c.runtimeWindows.CloseWindow(id)
	c.dispatch(events)
	return result, errors.Join(runtimeErr, c.closeProjection(id))
}

func (c *windowController) activateRuntimeProjection(id termmux.WindowID) error {
	projection, ok := c.windows[id]
	if !ok || projection.closed || c.runtimeWindows == nil {
		return errWindowProjectionMissing
	}
	events, err := c.runtimeWindows.ActivateWindow(id)
	if err != nil {
		return err
	}
	if err := c.focus(id); err != nil {
		return err
	}
	c.dispatch(events)
	return nil
}

// recordRuntimeFocus mirrors a native focus callback into the exact mux window.
// It deliberately does not call host.Focus: the native callback is already the
// result of that transition, and focusing again would re-enter the callback.
func (c *windowController) recordRuntimeFocus(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	projection, ok := c.windows[id]
	if !ok || projection.closed || c.runtimeWindows == nil {
		return errWindowProjectionMissing
	}
	events, err := c.runtimeWindows.ActivateWindow(id)
	if err != nil {
		return err
	}
	c.active = id
	c.dispatch(events)
	return nil
}

func (c *windowController) activeProjectionApp() *App {
	return c.projectionApp(c.active)
}

func (c *windowController) syncSharedProjectionState(owner *App) error {
	for _, id := range c.projectionIDs() {
		child := c.projectionApp(id)
		if child == nil || child == owner || child.scriptGeneration == owner.scriptGeneration {
			continue
		}
		if err := c.withCurrent(id, func() {
			child.scriptRT = owner.scriptRT
			child.scriptGeneration = owner.scriptGeneration
			child.cfg = owner.cfg.Clone()
			child.desiredCfg = owner.desiredCfg.Clone()
			child.composedCfg = owner.composedCfg.Clone()
			child.composedProvenance = append(child.composedProvenance[:0], owner.composedProvenance...)
			child.initZoomHotkeys()
			child.initActionBindings()
			child.closeBackgroundSurface()
			if err := child.prepareInitialBackgroundSurface(); err != nil {
				child.Notify("config reload: " + err.Error())
			}
			child.applyWindowAppearance()
			child.requestRedraw()
		}); err != nil {
			return err
		}
	}
	return nil
}
