//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"log"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	errWindowProjectionExists  = errors.New("window projection already exists")
	errWindowProjectionMissing = errors.New("window projection not found")
	errWindowLoopInactive      = errors.New("window controller loop is not active")
)

// processServices is the process-owned half of the frontend. During the
// single-window compatibility stage App retains access aliases, but creation,
// routing and shutdown are coordinated through this one owner.
type processServices struct {
	mux           *termmux.Mux
	scriptRuntime *script.Runtime
	runtimeScopes *config.RuntimeScopes
}

type nativeWindowHost interface {
	MakeContextCurrent()
	ShouldClose() bool
	Destroy()
}

type nativeEventPump interface {
	PollEvents()
	WaitEventsTimeout(time.Duration)
}

type glfwEventPump struct{}

func (glfwEventPump) PollEvents() { glfw.PollEvents() }
func (glfwEventPump) WaitEventsTimeout(timeout time.Duration) {
	glfw.WaitEventsTimeout(timeout.Seconds())
}

type windowProjection struct {
	id       termmux.WindowID
	host     nativeWindowHost
	handle   func([]termmux.Event) bool
	teardown func() error
	dirty    bool
	closed   bool
}

// windowController is called only from the runtime.LockOSThread owner. It is
// deliberately independent of GLFW concrete window types so lifecycle and
// routing order are testable without a native display.
type windowController struct {
	services processServices
	pump     nativeEventPump
	windows  map[termmux.WindowID]*windowProjection
	pending  map[termmux.WindowID][]termmux.Event
	order    []termmux.WindowID
	active   termmux.WindowID
	current  termmux.WindowID
	inLoop   bool
}

func newWindowController(services processServices, pump nativeEventPump) *windowController {
	return &windowController{services: services, pump: pump, windows: make(map[termmux.WindowID]*windowProjection), pending: make(map[termmux.WindowID][]termmux.Event)}
}

func (c *windowController) setServices(services processServices) { c.services = services }

func (c *windowController) setTeardown(id termmux.WindowID, teardown func() error) error {
	projection, ok := c.windows[id]
	if !ok || projection.closed {
		return errWindowProjectionMissing
	}
	projection.teardown = teardown
	return nil
}

func (c *windowController) drainMux(limit int) []termmux.Event {
	if c.services.mux == nil {
		return nil
	}
	return c.services.mux.Drain(limit)
}

func (c *windowController) attach(id termmux.WindowID, host nativeWindowHost, handle func([]termmux.Event) bool) error {
	if id == 0 || host == nil || handle == nil {
		return errWindowProjectionMissing
	}
	if _, exists := c.windows[id]; exists {
		return errWindowProjectionExists
	}
	c.windows[id] = &windowProjection{id: id, host: host, handle: handle, dirty: true}
	c.order = append(c.order, id)
	if c.active == 0 {
		c.active = id
	}
	return nil
}

func (c *windowController) startLoop() error {
	if c.inLoop {
		return fmt.Errorf("window controller loop already active")
	}
	c.inLoop = true
	return nil
}

func (c *windowController) stopLoop() { c.inLoop = false }

func (c *windowController) requireLoop() error {
	if !c.inLoop {
		return errWindowLoopInactive
	}
	return nil
}

func (c *windowController) activate(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	projection, ok := c.windows[id]
	if !ok || projection.closed {
		return errWindowProjectionMissing
	}
	projection.host.MakeContextCurrent()
	c.current = id
	return nil
}

func (c *windowController) focus(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	projection, ok := c.windows[id]
	if !ok || projection.closed {
		return errWindowProjectionMissing
	}
	c.active = id
	return nil
}

func (c *windowController) shouldClose(id termmux.WindowID) bool {
	projection, ok := c.windows[id]
	return !ok || projection.closed || projection.host.ShouldClose()
}

func (c *windowController) pollEvents() error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	c.pump.PollEvents()
	return nil
}

func (c *windowController) waitEvents(timeout time.Duration) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	c.pump.WaitEventsTimeout(timeout)
	return nil
}

func (c *windowController) withCurrent(id termmux.WindowID, frame func()) error {
	if err := c.activate(id); err != nil {
		return err
	}
	frame()
	return nil
}

func (c *windowController) markDamage(id termmux.WindowID) {
	if projection := c.windows[id]; projection != nil && !projection.closed {
		projection.dirty = true
	}
}

func (c *windowController) clearDamage(id termmux.WindowID) {
	if projection := c.windows[id]; projection != nil {
		projection.dirty = false
	}
}

func (c *windowController) dispatch(events []termmux.Event) bool {
	if !c.inLoop {
		return false
	}
	batches := make(map[termmux.WindowID][]termmux.Event)
	for id, pending := range c.pending {
		if _, ok := c.windows[id]; ok {
			batches[id] = append(batches[id], pending...)
			delete(c.pending, id)
		}
	}
	for _, event := range events {
		target := event.Window
		if target == 0 {
			target = c.active
		}
		if _, ok := c.windows[target]; ok {
			batches[target] = append(batches[target], event)
		} else if target != 0 {
			c.queuePending(target, event)
		}
	}
	consumed := false
	for _, id := range c.order {
		projection := c.windows[id]
		batch := batches[id]
		if projection == nil || projection.closed || len(batch) == 0 {
			continue
		}
		if projection.handle(batch) {
			projection.dirty, consumed = true, true
		}
	}
	return consumed
}

const maxPendingWindowEvents = 256

func (c *windowController) queuePending(id termmux.WindowID, event termmux.Event) {
	pending := c.pending[id]
	if len(pending) == maxPendingWindowEvents {
		pending = pending[1:]
	}
	c.pending[id] = append(pending, event)
}

func (c *windowController) closeProjection(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	projection, ok := c.windows[id]
	if !ok || projection.closed {
		return nil
	}
	projection.host.MakeContextCurrent()
	var teardownErr error
	if projection.teardown != nil {
		teardownErr = projection.teardown()
	}
	projection.host.Destroy()
	projection.closed = true
	delete(c.windows, id)
	for i, candidate := range c.order {
		if candidate == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	if c.current == id {
		c.current = 0
	}
	if c.active == id {
		c.active = 0
		if len(c.order) > 0 {
			c.active = c.order[0]
		}
	}
	return teardownErr
}

const initialWindowID termmux.WindowID = 1

func (a *App) attachInitialWindowController(window *glfw.Window) error {
	a.controller = newWindowController(processServices{scriptRuntime: a.scriptRT, runtimeScopes: &a.runtimeScopes}, glfwEventPump{})
	if err := a.controller.attach(initialWindowID, window, a.applyMuxEvents); err != nil {
		return err
	}
	return a.controller.startLoop()
}

func (a *App) closeInitialWindowController() {
	if a.controller == nil {
		return
	}
	if err := a.controller.closeProjection(initialWindowID); err != nil {
		logControllerError(err)
	}
	a.controller.stopLoop()
}

func logControllerError(err error) {
	if err != nil {
		log.Printf("window controller: %v", err)
	}
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
		_ = a.controller.focus(initialWindowID)
	}
}

func (a *App) syncProcessServices() {
	if a.controller != nil {
		a.controller.setServices(processServices{mux: a.mux, scriptRuntime: a.scriptRT, runtimeScopes: &a.runtimeScopes})
	}
}

func (a *App) installScriptRuntime(runtime *script.Runtime) {
	a.scriptRT = runtime
	a.syncProcessServices()
}

func (a *App) drainMuxEvents(limit int) []termmux.Event {
	if a.controller != nil {
		return a.controller.drainMux(limit)
	}
	if a.mux == nil {
		return nil
	}
	return a.mux.Drain(limit)
}

func (c *windowController) shutdownServices() error {
	if c.services.mux == nil {
		return nil
	}
	err := c.services.mux.Shutdown()
	c.services.mux = nil
	return err
}

func (a *App) shutdownProcessServices() {
	if a.controller != nil && a.controller.services.mux != nil {
		_ = a.controller.shutdownServices()
		return
	}
	if a.mux != nil {
		_ = a.mux.Shutdown()
	}
}
