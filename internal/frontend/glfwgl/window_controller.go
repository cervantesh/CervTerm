//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
	"cervterm/internal/ownerthread"
	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	errWindowProjectionExists  = errors.New("window projection already exists")
	errWindowProjectionMissing = errors.New("window projection not found")
	errWindowLoopInactive      = errors.New("window controller loop is not active")
	errWindowLoopThread        = errors.New("window controller loop native thread mismatch")
	errWindowLoopEpoch         = errors.New("window controller loop epoch is stale")
)

// processServices is the process-owned half of the frontend. App instances never
// receive either interface; every global operation crosses an explicit
// windowController method.
type processServices struct {
	commands           processCommandCapability
	windowCapabilities windowCapabilityFactory
	scriptRuntime      *script.Runtime
	runtimeScopes      *config.RuntimeScopes
}

type windowLoopLease struct{ epoch atomic.Uint64 }

type nativeContextCurrent func(nativeWindowHost) bool

func glfwContextCurrent(host nativeWindowHost) bool {
	window, ok := host.(*glfw.Window)
	return ok && window != nil && glfw.GetCurrentContext() == window
}

type nativeWindowHost interface {
	MakeContextCurrent()
	Focus()
	Show()
	Hide()
	ShouldClose() bool
	Destroy()
	GetPos() (int, int)
	GetSize() (int, int)
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
	host         nativeWindowHost
	app          *App
	handle       func([]termmux.Event) bool
	bind         func(termmux.WindowID) error
	unbind       func() error
	beforeUnbind *compositionBeforeUnbind
	resources    []projectionResource
	closed       bool
}

type nativeProjectionFactory interface {
	Create(termmux.WindowID) (*nativeProjectionBundle, error)
}

type windowProjection struct {
	id       termmux.WindowID
	identity termmux.WindowIdentity
	host     nativeWindowHost
	app      *App
	handle   func([]termmux.Event) bool
	bundle   *nativeProjectionBundle
	teardown func() error
	dirty    bool
	visible  bool
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
	restoreWindows   restoreWindowLifecycle
	persistLayout    func() error
	primary          *App
	windows          map[termmux.WindowID]*windowProjection
	pending          map[termmux.WindowID][]termmux.Event
	boundOrigins     map[termmux.WindowIdentity]*App
	order            []termmux.WindowID
	active           termmux.WindowID
	current          termmux.WindowID
	inLoop           bool
	threadSource     ownerthread.Source
	loopThread       ownerthread.Attestation
	loopThreadLocked bool
	loopEpoch        uint64
	activeLoopEpoch  uint64
	loopLease        *windowLoopLease
	contextCurrent   nativeContextCurrent
	restorePending   *restoreProjectionCandidate
}

func newWindowController(services processServices, pump nativeEventPump) *windowController {
	return &windowController{
		services: services, pump: pump, runtimeWindows: services.commands, restoreWindows: services.commands,
		threadSource: ownerthread.Native{}, loopLease: &windowLoopLease{}, contextCurrent: glfwContextCurrent,
		windows: make(map[termmux.WindowID]*windowProjection), pending: make(map[termmux.WindowID][]termmux.Event),
	}
}

func (c *windowController) setSharedServices(runtime *script.Runtime, scopes *config.RuntimeScopes) {
	c.services.scriptRuntime = runtime
	c.services.runtimeScopes = scopes
}

func (c *windowController) installProcessServices(commands processCommandCapability, windows windowCapabilityFactory) error {
	if c == nil || commands == nil || windows == nil || c.services.commands != nil || c.services.windowCapabilities != nil {
		return errWindowProjectionExists
	}
	c.services.commands = commands
	c.services.windowCapabilities = windows
	c.runtimeWindows = commands
	c.restoreWindows = commands
	return nil
}

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
		if rollbackErr := closeProjectionBundleWithCurrent(bundle); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	if bundle == nil || bundle.host == nil || bundle.handle == nil {
		rollbackErr := closeProjectionBundleWithCurrent(bundle)
		return errors.Join(errWindowProjectionMissing, rollbackErr)
	}
	if err := c.attachApp(id, bundle.host, bundle.app, bundle.handle); err != nil {
		return errors.Join(err, closeProjectionBundleWithCurrent(bundle))
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
	if c.services.commands == nil {
		return nil
	}
	events, err := c.services.commands.Drain(limit)
	logControllerError(err)
	return events
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
	if app != nil && app.windowIdentity != (termmux.WindowIdentity{}) {
		if bound := c.boundOrigins[app.windowIdentity]; bound != nil && bound != app {
			return errWindowProjectionMissing
		}
		delete(c.boundOrigins, app.windowIdentity)
	}
	projection := &windowProjection{id: id, host: host, app: app, handle: handle, dirty: true, visible: true}
	if app != nil {
		projection.identity = app.windowIdentity
	}
	c.windows[id] = projection
	c.order = append(c.order, id)
	if c.active == 0 {
		c.active = id
	}
	return nil
}

func (c *windowController) startLoop() error {
	if c == nil || c.inLoop {
		return fmt.Errorf("window controller loop already active")
	}
	runtime.LockOSThread()
	attestation, ok := ownerthread.Capture(c.threadSource)
	if !ok {
		runtime.UnlockOSThread()
		return errWindowLoopThread
	}
	c.loopEpoch++
	if c.loopEpoch == 0 {
		runtime.UnlockOSThread()
		return errWindowLoopEpoch
	}
	c.loopThread = attestation
	c.activeLoopEpoch = c.loopEpoch
	c.loopLease.epoch.Store(c.loopEpoch)
	c.inLoop = true
	c.loopThreadLocked = true
	return nil
}

func (c *windowController) stopLoop() {
	if c == nil {
		return
	}
	c.inLoop = false
	c.activeLoopEpoch = 0
	c.loopLease.epoch.Store(0)
	if c.loopThreadLocked && c.loopThread.Current(c.threadSource) {
		c.loopThreadLocked = false
		runtime.UnlockOSThread()
	}
}

func (c *windowController) requireLoop() error {
	if c == nil || !c.inLoop {
		return errWindowLoopInactive
	}
	if c.loopEpoch == 0 || c.activeLoopEpoch != c.loopEpoch {
		return errWindowLoopEpoch
	}
	if !c.loopThread.Current(c.threadSource) {
		return errWindowLoopThread
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
	if c.requireLoop() != nil {
		return true
	}
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
	if err := c.requireLoop(); err != nil {
		return err
	}
	if err := c.activate(id); err != nil {
		return err
	}
	frame()
	return nil
}

func (c *windowController) markDamageFrom(origin termmux.WindowIdentity) error {
	if err := c.requireOrigin(origin); err != nil {
		return err
	}
	c.markDamage(origin.ID)
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
	if c.requireLoop() != nil {
		return false
	}
	if c.services.commands != nil {
		events = c.services.commands.ResolveEventAddresses(events)
	}
	if err := c.applyWorkspaceProjection(events); err != nil {
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
	if c.persistLayout != nil && layoutPersistenceEvent(events) {
		logControllerError(c.persistLayout())
	}
	return consumed
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
	if projection.bundle != nil {
		teardownErr = projection.bundle.unbindProjection()
	} else if projection.app != nil {
		teardownErr = projection.app.cancelComposition(ime.CancelTeardown)
		projection.app.composition.deactivateDelivery()
		projection.app.charSuppression.clear()
	}
	if projection.teardown != nil {
		teardownErr = errors.Join(teardownErr, projection.teardown())
	}
	if projection.bundle != nil {
		teardownErr = errors.Join(teardownErr, projection.bundle.close())
	} else {
		projection.host.Destroy()
	}
	projection.closed = true
	if projection.app != nil {
		delete(c.boundOrigins, projection.app.windowIdentity)
	}
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
