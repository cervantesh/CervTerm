//go:build glfw

package glfwgl

import (
	"fmt"
	"time"
)

func (a *App) reloadSourceActive() bool {
	return a.configPath != ""
}

func (a *App) pollReloadWatch(now time.Time) bool {
	return a.configWatch.poll(now)
}

func (a *App) markReloadPending() {
	a.reloadPending = true
}

func (a *App) reloadIsPending() bool {
	return a.reloadPending
}

func (a *App) consumeReloadPending() {
	a.reloadPending = false
}

func (a *App) drainReloadResults() {
	a.applyConfigReloadWorkerResults()
}

func (a *App) reloadWorkerCount() int {
	return a.configReloadAsync.workers
}

func (a *App) startReloadWorker() {
	a.startConfigReloadWorker()
}

func (a *App) reportMissingReloadSource() {
	a.reportConfigReloadFailure(fmt.Errorf("no config source is active"), time.Now())
}
