//go:build glfw

package glfwgl

import (
	"errors"
	"log"

	termmux "cervterm/internal/mux"
	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const initialWindowID termmux.WindowID = 1

func logControllerError(err error) {
	if err != nil {
		log.Printf("window controller: %v", err)
	}
}

func (a *App) attachInitialWindowController(window *glfw.Window) error {
	a.host = newWindowController(processServices{scriptRuntime: a.scriptRT, runtimeScopes: &a.runtimeScopes}, glfwEventPump{})
	a.controller = newProjectionMessageRouter(a.host)
	a.host.primary = a
	if err := a.host.startLoop(); err != nil {
		a.host = nil
		a.controller = nil
		return err
	}
	if err := a.host.attachApp(initialWindowID, window, a, a.applyMuxEvents); err != nil {
		a.host.stopLoop()
		a.host = nil
		a.controller = nil
		return err
	}
	a.windowID = initialWindowID
	a.host.setCandidateProjectionFactory(&glfwProjectionFactory{owner: a})
	return nil
}

func (c *windowController) closeProjectionLoop() error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	ids := c.projectionIDs()
	var joined error
	for index := len(ids) - 1; index >= 0; index-- {
		joined = errors.Join(joined, c.closeProjection(ids[index]))
	}
	c.stopLoop()
	return joined
}

func (a *App) closeInitialWindowController() {
	if a.host == nil {
		return
	}
	logControllerError(a.host.closeProjectionLoop())
}

func (a *App) handleMuxEvents(events []termmux.Event) bool { return a.dispatchMuxEvents(events) }

func (a *App) dispatchMuxEvents(events []termmux.Event) bool {
	if a.controller == nil {
		return a.applyMuxEvents(events)
	}
	return a.controller.dispatch(events)
}

func (a *App) recordNativeFocus(focused bool) {
	if focused && a.controller != nil {
		if err := a.controller.recordRuntimeFocus(a.windowID); err != nil {
			logControllerError(err)
		}
	}
}

func (a *App) syncProcessServices() {
	if a.host == nil {
		return
	}
	a.host.setSharedServices(a.scriptRT, &a.runtimeScopes)
	a.host.setCandidateProjectionFactory(&glfwProjectionFactory{owner: a})
	if a.cfg.LayoutPersistence.Enabled {
		a.host.persistLayout = a.persistCurrentLayout
	} else {
		a.host.persistLayout = nil
	}
}

func (a *App) installScriptRuntime(runtime *script.Runtime) {
	a.scriptRT = runtime
	a.syncProcessServices()
}

func (a *App) drainMuxEvents(limit int) []termmux.Event {
	if a.host == nil {
		return nil
	}
	return a.host.drainMux(limit)
}
