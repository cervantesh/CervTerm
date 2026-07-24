//go:build glfw

package glfwgl

import (
	"strings"
	"time"

	termmux "cervterm/internal/mux"
)

var (
	_ scriptLifecycleRuntimePort       = (*App)(nil)
	_ scriptLifecycleEventPort         = (*App)(nil)
	_ scriptLifecycleDeferredEventPort = (*App)(nil)
	_ scriptLifecycleFailurePort       = (*App)(nil)
	_ scriptLifecyclePendingPort       = (*App)(nil)
	_ scriptLifecycleTimerPort         = (*App)(nil)
	_ scriptLifecycleProjectionPort    = (*App)(nil)
)

func (a *App) scriptLifecycleRuntimeAvailable() bool { return a.scriptRT != nil }

func (a *App) scriptLifecycleWantsOutput() bool { return a.scriptRT.WantsOutput() }

func (a *App) fireScriptOutput(pane termmux.PaneID, data string) error {
	return a.scriptRT.FireOutput(paneHost{app: a, pane: pane}, data)
}

func (a *App) fireScriptTitle(pane termmux.PaneID, title string) error {
	return a.scriptRT.FireTitle(paneHost{app: a, pane: pane}, title)
}

func (a *App) fireScriptCWD(pane termmux.PaneID, cwd string) error {
	return a.scriptRT.FireCwd(paneHost{app: a, pane: pane}, cwd)
}

func (a *App) fireScriptBell(pane termmux.PaneID) error {
	return a.scriptRT.FireBell(paneHost{app: a, pane: pane})
}

func (a *App) fireScriptFocus(pane termmux.PaneID, focused bool) error {
	return a.scriptRT.FireFocus(paneHost{app: a, pane: pane}, focused)
}

func (a *App) fireScriptResize(pane termmux.PaneID, cols, rows int) error {
	return a.scriptRT.FireResize(paneHost{app: a, pane: pane}, cols, rows)
}

func (a *App) fireScriptScroll(pane termmux.PaneID, offset int) error {
	return a.scriptRT.FireScroll(paneHost{app: a, pane: pane}, offset)
}

func (a *App) reportScriptLifecycleError(err error) {
	a.Notify("script error: " + err.Error())
}

func (a *App) clearPendingScriptLifecycle() {
	clear(a.pendingPaneResize)
	clear(a.pendingPaneScroll)
}

func (a *App) snapshotPendingScriptResizes() []scriptResizeEvent {
	if len(a.pendingPaneResize) == 0 {
		return nil
	}
	pending := a.pendingPaneResize
	a.pendingPaneResize = make(map[termmux.PaneID]termmux.PaneGeometry)
	events := make([]scriptResizeEvent, 0, len(pending))
	for pane, geometry := range pending {
		events = append(events, scriptResizeEvent{pane: pane, cols: geometry.Cols, rows: geometry.Rows})
	}
	return events
}

func (a *App) snapshotPendingScriptScrolls() []scriptScrollEvent {
	if len(a.pendingPaneScroll) == 0 {
		return nil
	}
	pending := a.pendingPaneScroll
	a.pendingPaneScroll = make(map[termmux.PaneID]int)
	events := make([]scriptScrollEvent, 0, len(pending))
	for pane, offset := range pending {
		events = append(events, scriptScrollEvent{pane: pane, offset: offset})
	}
	return events
}

func (a *App) fireDueScriptTimers(now time.Time) {
	a.scriptRT.FireDueTimers(now, a.hostForFocused())
}

func (a *App) syncScriptStatus() {
	if a.scriptRT == nil {
		return
	}
	seq := a.scriptRT.StatusSeq()
	if seq == a.status.seq {
		return
	}
	a.status.seq = seq
	a.status.line = strings.Join(a.scriptRT.StatusSegments(), " · ")
	a.requestRedraw()
}

func (a *App) syncScriptOverlays() {
	if a.scriptRT == nil {
		return
	}
	for _, msg := range a.scriptRT.DrainOverlayNotices() {
		a.Notify("script error: " + msg)
	}
	seq := a.scriptRT.OverlaySeq()
	if seq == a.overlays.seq {
		return
	}
	a.overlays.seq = seq
	a.overlays.scenes = a.scriptRT.Overlays()
	a.requestRedraw()
}
