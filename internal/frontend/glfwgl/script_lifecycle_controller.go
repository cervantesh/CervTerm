//go:build glfw

package glfwgl

import (
	"time"

	termmux "cervterm/internal/mux"
)

const scriptLifecycleControllerPortBudget = 14

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

type scriptLifecycleFailurePort interface {
	reportScriptLifecycleError(error)
}

type scriptLifecyclePendingPort interface {
	clearPendingScriptLifecycle()
	dispatchPendingScriptResizes()
	dispatchPendingScriptScrolls()
}

type scriptLifecycleTimerPort interface {
	fireDueScriptTimers(time.Time)
}

type scriptLifecycleProjectionPort interface {
	syncScriptStatus()
	syncScriptOverlays()
}

// scriptLifecycleController owns callback phase ordering only. It is a
// zero-field value retained per App; every operation receives its ports
// ephemerally, so no App-to-controller-to-App dynamic backreference exists.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type scriptLifecycleController struct{}

func newScriptLifecycleController() scriptLifecycleController {
	return scriptLifecycleController{}
}

func (*scriptLifecycleController) output(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID, data string) {
	if data == "" || !runtime.scriptLifecycleRuntimeAvailable() || !runtime.scriptLifecycleWantsOutput() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptOutput(pane, data))
}

func (*scriptLifecycleController) outputBytes(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID, data []byte) {
	if len(data) == 0 || !runtime.scriptLifecycleRuntimeAvailable() || !runtime.scriptLifecycleWantsOutput() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptOutput(pane, string(data)))
}

func (*scriptLifecycleController) title(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID, title string) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptTitle(pane, title))
}

func (*scriptLifecycleController) cwd(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID, cwd string) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptCWD(pane, cwd))
}

func (*scriptLifecycleController) bell(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptBell(pane))
}

func (*scriptLifecycleController) focus(runtime scriptLifecycleRuntimePort, events scriptLifecycleEventPort, failures scriptLifecycleFailurePort, pane termmux.PaneID, focused bool) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	reportScriptLifecycleError(failures, events.fireScriptFocus(pane, focused))
}

func (*scriptLifecycleController) dispatchPending(runtime scriptLifecycleRuntimePort, pending scriptLifecyclePendingPort) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		pending.clearPendingScriptLifecycle()
		return
	}
	pending.dispatchPendingScriptResizes()
	pending.dispatchPendingScriptScrolls()
}

func (*scriptLifecycleController) fireDueTimers(runtime scriptLifecycleRuntimePort, timers scriptLifecycleTimerPort, now time.Time) {
	if runtime.scriptLifecycleRuntimeAvailable() {
		timers.fireDueScriptTimers(now)
	}
}

func (*scriptLifecycleController) syncStatus(runtime scriptLifecycleRuntimePort, projections scriptLifecycleProjectionPort) {
	if runtime.scriptLifecycleRuntimeAvailable() {
		projections.syncScriptStatus()
	}
}

func (*scriptLifecycleController) syncOverlays(runtime scriptLifecycleRuntimePort, projections scriptLifecycleProjectionPort) {
	if runtime.scriptLifecycleRuntimeAvailable() {
		projections.syncScriptOverlays()
	}
}

func (*scriptLifecycleController) syncProjections(runtime scriptLifecycleRuntimePort, projections scriptLifecycleProjectionPort) {
	if !runtime.scriptLifecycleRuntimeAvailable() {
		return
	}
	projections.syncScriptStatus()
	projections.syncScriptOverlays()
}

func reportScriptLifecycleError(failures scriptLifecycleFailurePort, err error) {
	if err != nil {
		failures.reportScriptLifecycleError(err)
	}
}
