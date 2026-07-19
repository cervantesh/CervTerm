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
	Focus()
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

// projectionResource is one independently owned part of a native projection.
// Resources are recorded in acquisition order and closed in reverse order.
type projectionResource interface {
	Close() error
}

type projectionResourceFunc func() error

func (close projectionResourceFunc) Close() error { return close() }

// nativeProjectionBundle is provisional until windowController.createProjection
// publishes it. A factory may return a partial bundle with an error; the
// controller still rolls it back, preventing callbacks or native resources
// from escaping failed candidate creation.
type nativeProjectionBundle struct {
	host      nativeWindowHost
	app       *App
	handle    func([]termmux.Event) bool
	bind      func(termmux.WindowID) error
	resources []projectionResource
	closed    bool
}

func (b *nativeProjectionBundle) close() error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	var joined error
	for i := len(b.resources) - 1; i >= 0; i-- {
		if b.resources[i] != nil {
			joined = errors.Join(joined, b.resources[i].Close())
		}
	}
	if b.host != nil {
		b.host.Destroy()
	}
	return joined
}

type nativeProjectionFactory interface {
	Create(termmux.WindowID) (*nativeProjectionBundle, error)
}

type windowProjection struct {
	id       termmux.WindowID
	host     nativeWindowHost
	app      *App
	handle   func([]termmux.Event) bool
	bundle   *nativeProjectionBundle
	teardown func() error
	dirty    bool
	closed   bool
}

// windowController is called only from the runtime.LockOSThread owner. It is
// deliberately independent of GLFW concrete window types so lifecycle and
// routing order are testable without a native display.
type windowController struct {
	services         processServices
	pump             nativeEventPump
	factory          nativeProjectionFactory
	candidateFactory nativeProjectionCandidateFactory
	runtimeWindows   runtimeWindowLifecycle
	windows          map[termmux.WindowID]*windowProjection
	pending          map[termmux.WindowID][]termmux.Event
	order            []termmux.WindowID
	active           termmux.WindowID
	current          termmux.WindowID
	inLoop           bool
}

func newWindowController(services processServices, pump nativeEventPump) *windowController {
	return &windowController{services: services, pump: pump, windows: make(map[termmux.WindowID]*windowProjection), pending: make(map[termmux.WindowID][]termmux.Event)}
}

func (c *windowController) setServices(services processServices) { c.services = services }

func (c *windowController) setProjectionFactory(factory nativeProjectionFactory) { c.factory = factory }

func (c *windowController) setCandidateProjectionFactory(factory nativeProjectionCandidateFactory) {
	c.setCandidateFactory(factory)
}

func (c *windowController) adoptProjectionBundle(id termmux.WindowID, bundle *nativeProjectionBundle) error {
	projection, ok := c.windows[id]
	if !ok || projection.closed || bundle == nil || bundle.host != projection.host || bundle.app != projection.app {
		return errWindowProjectionMissing
	}
	if projection.bundle != nil {
		return errWindowProjectionExists
	}
	projection.bundle = bundle
	return nil
}

// createProjection transactionally acquires a complete independent native
// projection. Nothing is addressable through windows/order until every factory
// stage succeeds. Partial candidates are always rolled back.
func (c *windowController) createProjection(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	if id == 0 || c.factory == nil {
		return errWindowProjectionMissing
	}
	if _, exists := c.windows[id]; exists {
		return errWindowProjectionExists
	}
	bundle, err := c.factory.Create(id)
	if err != nil {
		if rollbackErr := bundle.close(); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	if bundle == nil || bundle.host == nil || bundle.handle == nil {
		rollbackErr := bundle.close()
		return errors.Join(errWindowProjectionMissing, rollbackErr)
	}
	if err := c.attachApp(id, bundle.host, bundle.app, bundle.handle); err != nil {
		return errors.Join(err, bundle.close())
	}
	c.windows[id].bundle = bundle
	return nil
}

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
	return c.attachApp(id, host, nil, handle)
}

func (c *windowController) attachApp(id termmux.WindowID, host nativeWindowHost, app *App, handle func([]termmux.Event) bool) error {
	if id == 0 || host == nil || handle == nil {
		return errWindowProjectionMissing
	}
	if _, exists := c.windows[id]; exists {
		return errWindowProjectionExists
	}
	c.windows[id] = &windowProjection{id: id, host: host, app: app, handle: handle, dirty: true}
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
	projection.host.Focus()
	c.active = id
	return nil
}

func (c *windowController) projectionIDs() []termmux.WindowID {
	ids := make([]termmux.WindowID, 0, len(c.order))
	for _, id := range c.order {
		if projection := c.windows[id]; projection != nil && !projection.closed {
			ids = append(ids, id)
		}
	}
	return ids
}

func (c *windowController) projectionApp(id termmux.WindowID) *App {
	if projection := c.windows[id]; projection != nil && !projection.closed {
		return projection.app
	}
	return nil
}

func (c *windowController) projectionCount() int { return len(c.windows) }

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
	if projection.bundle != nil {
		teardownErr = errors.Join(teardownErr, projection.bundle.close())
	} else {
		projection.host.Destroy()
	}
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
	if err := a.controller.attachApp(initialWindowID, window, a, a.applyMuxEvents); err != nil {
		return err
	}
	a.windowID = initialWindowID
	a.controller.setCandidateProjectionFactory(&glfwProjectionFactory{owner: a})
	return a.controller.startLoop()
}

func (a *App) closeInitialWindowController() {
	if a.controller == nil {
		return
	}
	var joined error
	for _, id := range a.controller.projectionIDs() {
		joined = errors.Join(joined, a.controller.closeProjection(id))
	}
	logControllerError(joined)
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
		if err := a.controller.recordRuntimeFocus(a.windowID); err != nil {
			logControllerError(err)
		}
	}
}

func (a *App) syncProcessServices() {
	if a.controller != nil {
		a.controller.setServices(processServices{mux: a.mux, scriptRuntime: a.scriptRT, runtimeScopes: &a.runtimeScopes})
		if a.mux != nil {
			a.controller.setRuntimeWindows(a.mux)
		}
		a.controller.setCandidateProjectionFactory(&glfwProjectionFactory{owner: a})
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
