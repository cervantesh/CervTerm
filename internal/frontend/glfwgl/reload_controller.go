//go:build glfw

package glfwgl

import "time"

const (
	reloadControllerWorkerCap  = 2
	reloadControllerPortBudget = 9
)

type reloadSourcePort interface {
	reloadSourceActive() bool
}

type reloadWatchPort interface {
	pollReloadWatch(time.Time) bool
}

type reloadPendingPort interface {
	markReloadPending()
	reloadIsPending() bool
	consumeReloadPending()
}

type reloadWorkerPort interface {
	drainReloadResults()
	reloadWorkerCount() int
	startReloadWorker()
}

type reloadFailurePort interface {
	reportMissingReloadSource(time.Time)
}

// reloadController owns only reload request and dispatch ordering. Pending state,
// prepared generations, runtime/config state, GPU resources, and worker results
// remain behind App-owned ports until later movement and wiring commits.
type reloadController struct {
	source  reloadSourcePort
	watch   reloadWatchPort
	pending reloadPendingPort
	workers reloadWorkerPort
	failure reloadFailurePort
}

func newReloadController(source reloadSourcePort, watch reloadWatchPort, pending reloadPendingPort, workers reloadWorkerPort, failure reloadFailurePort) *reloadController {
	return &reloadController{source: source, watch: watch, pending: pending, workers: workers, failure: failure}
}

func (c *reloadController) requestReload() bool {
	if !c.source.reloadSourceActive() {
		return false
	}
	c.pending.markReloadPending()
	return true
}

func (c *reloadController) pollReload(now time.Time) {
	if c.watch.pollReloadWatch(now) {
		c.pending.markReloadPending()
	}
}

func (c *reloadController) applyReload(now time.Time) {
	c.workers.drainReloadResults()
	if !c.pending.reloadIsPending() {
		return
	}
	if c.workers.reloadWorkerCount() >= reloadControllerWorkerCap {
		return
	}
	c.pending.consumeReloadPending()
	if !c.source.reloadSourceActive() {
		c.failure.reportMissingReloadSource(now)
		return
	}
	c.workers.startReloadWorker()
}
