//go:build glfw

package glfwgl

import (
	"time"

	termmux "cervterm/internal/mux"
)

const scriptLifecycleControllerPortBudget = 16

type scriptLifecycleRuntimePort interface {
	scriptLifecycleRuntimeAvailable() bool
	scriptLifecycleWantsOutput() bool
}

type scriptLifecycleEventPort interface {
	fireScriptOutput(termmux.PaneID, string) error
	fireScriptTitle(termmux.PaneID, string) error
	fireScriptCWD(termmux.PaneID, string) error
	fireScriptBell(termmux.PaneID) error
	fireScriptFocus(termmux.PaneID, bool) error
}

type scriptLifecycleDeferredEventPort interface {
	fireScriptResize(termmux.PaneID, int, int) error
	fireScriptScroll(termmux.PaneID, int) error
}

type scriptLifecycleFailurePort interface {
	reportScriptLifecycleError(error)
}

type scriptLifecyclePendingPort interface {
	clearPendingScriptLifecycle()
	takePendingScriptResize() (scriptResizeEvent, bool)
	takePendingScriptScroll() (scriptScrollEvent, bool)
}

type scriptLifecycleTimerPort interface {
	fireDueScriptTimers(time.Time)
}

type scriptLifecycleProjectionPort interface {
	syncScriptStatus()
	syncScriptOverlays()
}

type scriptResizeEvent struct {
	pane       termmux.PaneID
	cols, rows int
}

type scriptScrollEvent struct {
	pane   termmux.PaneID
	offset int
}

// scriptLifecycleController owns script callback ordering only. Runtime,
// pending maps, timers, status, overlays, and pane state remain authoritative
// behind ports; pending values cross one at a time as detached scalar records.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type scriptLifecycleController struct {
	runtime     scriptLifecycleRuntimePort
	events      scriptLifecycleEventPort
	deferred    scriptLifecycleDeferredEventPort
	failures    scriptLifecycleFailurePort
	pending     scriptLifecyclePendingPort
	timers      scriptLifecycleTimerPort
	projections scriptLifecycleProjectionPort
}

func newScriptLifecycleController(
	runtime scriptLifecycleRuntimePort,
	events scriptLifecycleEventPort,
	deferred scriptLifecycleDeferredEventPort,
	failures scriptLifecycleFailurePort,
	pending scriptLifecyclePendingPort,
	timers scriptLifecycleTimerPort,
	projections scriptLifecycleProjectionPort,
) *scriptLifecycleController {
	return &scriptLifecycleController{
		runtime: runtime, events: events, deferred: deferred, failures: failures,
		pending: pending, timers: timers, projections: projections,
	}
}

func (c *scriptLifecycleController) output(pane termmux.PaneID, data string) {
	if data == "" || !c.runtime.scriptLifecycleRuntimeAvailable() || !c.runtime.scriptLifecycleWantsOutput() {
		return
	}
	c.report(c.events.fireScriptOutput(pane, data))
}

func (c *scriptLifecycleController) title(pane termmux.PaneID, title string) {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	c.report(c.events.fireScriptTitle(pane, title))
}

func (c *scriptLifecycleController) cwd(pane termmux.PaneID, cwd string) {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	c.report(c.events.fireScriptCWD(pane, cwd))
}

func (c *scriptLifecycleController) bell(pane termmux.PaneID) {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	c.report(c.events.fireScriptBell(pane))
}

func (c *scriptLifecycleController) focus(pane termmux.PaneID, focused bool) {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	c.report(c.events.fireScriptFocus(pane, focused))
}

func (c *scriptLifecycleController) dispatchPending() {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		c.pending.clearPendingScriptLifecycle()
		return
	}
	for {
		event, ok := c.pending.takePendingScriptResize()
		if !ok {
			break
		}
		c.report(c.deferred.fireScriptResize(event.pane, event.cols, event.rows))
	}
	for {
		event, ok := c.pending.takePendingScriptScroll()
		if !ok {
			break
		}
		c.report(c.deferred.fireScriptScroll(event.pane, event.offset))
	}
}

func (c *scriptLifecycleController) fireDueTimers(now time.Time) {
	if c.runtime.scriptLifecycleRuntimeAvailable() {
		c.timers.fireDueScriptTimers(now)
	}
}

func (c *scriptLifecycleController) syncProjections() {
	if !c.runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	c.projections.syncScriptStatus()
	c.projections.syncScriptOverlays()
}

func (c *scriptLifecycleController) report(err error) {
	if err != nil {
		c.failures.reportScriptLifecycleError(err)
	}
}
