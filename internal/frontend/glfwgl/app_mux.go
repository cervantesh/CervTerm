//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/accessibility"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
	termsel "cervterm/internal/selection"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type paneUIState struct {
	selection   selectionState
	search      searchController
	link        linkState
	mouseReport mouseReportState
	font        paneFontState
}

type muxSearchTerminal struct {
	mux  *termmux.Mux
	pane termmux.PaneID
}

func (t muxSearchTerminal) SearchUpward(query string, hasPrev bool, prevRow int) (row, col int, ok bool) {
	row, col, ok, err := t.mux.SearchUpward(t.pane, query, hasPrev, prevRow)
	return row, col, ok && err == nil
}

func (a *App) muxMetrics() termmux.CellMetrics {
	return termmux.CellMetrics{
		CellWidth:  max(1, int(a.cellW)),
		CellHeight: max(1, int(a.cellH)),
	}
}

func (a *App) saveActivePaneUI() {
	if a.focusedPane == 0 {
		return
	}
	state := a.paneUI[a.focusedPane]
	if state == nil {
		if _, ok := a.mux.PaneView(a.focusedPane); !ok {
			return
		}
		state = a.ensurePaneUI(a.focusedPane)
	}
	state.selection = a.selection
	state.search = a.search
	state.link = a.link
	state.mouseReport = a.mouseReport
}

func (a *App) ensurePaneUI(id termmux.PaneID) *paneUIState {
	state := a.paneUI[id]
	if state == nil {
		state = &paneUIState{font: a.initialPaneFontState()}
		state.search.viewRow = -1
		a.paneUI[id] = state
	}
	return state
}

func (a *App) loadPaneUI(id termmux.PaneID) {
	state := a.ensurePaneUI(id)
	a.selection = state.selection
	a.search = state.search
	a.link = state.link
	a.mouseReport = state.mouseReport
	a.activatePaneFont(id)
	a.lterm = muxSearchTerminal{mux: a.mux, pane: id}
	a.search.init(a.lterm, a.requestAccessibilityRedraw)
	a.search.bindActivationChange(func() { _ = a.cancelComposition(ime.CancelTargetChanged) })
}

func (a *App) syncFocusedProjection() bool {
	id, ok := a.mux.FocusedPane()
	if !ok {
		return false
	}
	view, ok := a.mux.PaneView(id)
	if !ok {
		return false
	}
	if a.focusedPane != id {
		a.saveActivePaneUI()
		a.setFocusedPane(id)
		a.loadPaneUI(id)
	}
	a.snap = view.Snapshot
	a.cols, a.rows = view.Snapshot.Cols, view.Snapshot.Rows
	return true
}

func (a *App) focusedView() (termmux.PaneID, termmux.PaneView, bool) {
	if !a.syncFocusedProjection() {
		return 0, termmux.PaneView{}, false
	}
	view, ok := a.mux.PaneView(a.focusedPane)
	return a.focusedPane, view, ok
}

func (a *App) focusPane(id termmux.PaneID) bool {
	if id == 0 || id == a.focusedPane {
		return id != 0
	}
	a.saveActivePaneUI()
	events, err := a.mux.FocusPane(id)
	if err != nil {
		return false
	}
	a.handleMuxEvents(events)
	return true
}

func (a *App) applyMuxEvents(events []termmux.Event) bool {
	consumed := false
	var accessibilityIntents accessibility.SemanticIntent
	var accessibilityAnnouncements []accessibility.AnnouncementKind
	for _, event := range events {
		a.cancelCompositionForMuxEvent(event)
		intent, announcement := accessibilityIntentForMuxEvent(event)
		accessibilityIntents |= intent
		if announcement != accessibility.AnnouncementNone {
			accessibilityAnnouncements = append(accessibilityAnnouncements, announcement)
		}
		switch event.Kind {
		case termmux.PaneStarted:
			a.ensurePaneUI(event.Pane)
		case termmux.PaneOutput:
			a.meter.AddBytes(event.BytesRead)
			if tab, ok := a.mux.TabForPane(event.Pane); ok && tab != a.mux.ActiveTab() {
				if a.tabActivity == nil {
					a.tabActivity = make(map[termmux.TabID]bool)
				}
				a.tabActivity[tab] = true
			}
			if len(event.Data) > 0 && a.scriptLifecycleRuntimeAvailable() && a.scriptLifecycleWantsOutput() {
				if err := a.fireScriptOutput(event.Pane, string(event.Data)); err != nil {
					a.reportScriptLifecycleError(err)
				}
			}
			consumed = true
		case termmux.PaneDirty:
			consumed = true
		case termmux.PaneTitleChanged:
			if event.Pane == a.focusedPane {
				a.updateWindowTitle(event.Text)
			}
			a.fireScriptEvent(func() error { return a.fireScriptTitle(event.Pane, event.Text) })
		case termmux.PaneCWDChanged:
			a.fireScriptEvent(func() error { return a.fireScriptCWD(event.Pane, event.Text) })
		case termmux.PaneBell:
			a.deliverBellEvent(event.Pane)
		case termmux.PaneNotificationRequested:
			a.applyNotificationEffect(event.Notification, event.Fresh)
		case termmux.PaneNotificationOverflow:
			a.reportNotificationOverflow()
		case termmux.PaneFocused:
			oldPane := a.focusedPane
			if oldPane != 0 && oldPane != event.Pane {
				a.sendPaneFocus(oldPane, false)
			}
			a.saveActivePaneUI()
			a.setFocusedPane(event.Pane)
			a.loadPaneUI(event.Pane)
			if oldPane != event.Pane {
				a.sendPaneFocus(event.Pane, true)
			}
			if view, ok := a.mux.PaneView(event.Pane); ok {
				a.snap = view.Snapshot
				a.cols, a.rows = view.Snapshot.Cols, view.Snapshot.Rows
				a.updateWindowTitle(view.Snapshot.Title)
			}
			consumed = true
		case termmux.PaneGeometryChanged:
			a.invalidateCandidateGeometry()
			if event.Pane == a.focusedPane {
				a.cols, a.rows = event.Geometry.Cols, event.Geometry.Rows
			}
			if a.pendingPaneResize == nil {
				a.pendingPaneResize = make(map[termmux.PaneID]termmux.PaneGeometry)
			}
			a.pendingPaneResize[event.Pane] = event.Geometry
			consumed = true
		case termmux.PaneWriteFailed, termmux.PaneResizeFailed, termmux.PaneCloseFailed:
			if event.Err != nil {
				a.Notify(fmt.Sprintf("pane %d: %v", event.Pane, event.Err))
			}
		case termmux.PaneExited:
			if event.Err != nil {
				a.Notify(fmt.Sprintf("pane %d exited: %v", event.Pane, event.Err))
			} else if event.Pane == a.focusedPane {
				a.Notify(fmt.Sprintf("pane %d exited; close it to collapse the split", event.Pane))
			}
			consumed = true
		case termmux.PaneClosed:
			if state := a.paneUI[event.Pane]; state != nil && state.link.handCursor != nil {
				state.link.handCursor.Destroy()
			}
			delete(a.paneUI, event.Pane)
			delete(a.pendingPaneScroll, event.Pane)
			delete(a.pendingPaneResize, event.Pane)
			delete(a.bellDelivered, event.Pane)
			if a.mouseCapturePane == event.Pane {
				a.mouseCapturePane = 0
			}
			consumed = true
		case termmux.TabSpawned, termmux.TabActivated, termmux.TabRenamed, termmux.TabMoved, termmux.TabClosed, termmux.TabRevisionChanged, termmux.PaneTransferred:
			if event.Kind == termmux.TabActivated || event.Kind == termmux.TabClosed {
				delete(a.tabActivity, event.Tab)
			}
			newHeight := a.effectiveTabBarHeight()
			if newHeight != a.tabBarHeight {
				a.tabBarHeight = newHeight
				if a.window != nil {
					a.resizeToWindow()
				}
			}
			consumed = true
		case termmux.TabEmpty, termmux.WindowTabsEmpty:
			if a.window != nil {
				a.window.SetShouldClose(true)
			}
		}
	}
	if consumed {
		a.syncFocusedProjection()
		_ = a.reconcileComposition(ime.CancelTargetChanged)
		a.requestRedraw()
	}
	if a.accessibilityRuntime != nil {
		a.accessibilityRuntime.Invalidate(accessibilityIntents)
		for _, announcement := range accessibilityAnnouncements {
			if err := a.accessibilityRuntime.Announce(announcement); err != nil {
				a.failAccessibilityRuntime(err)
				break
			}
		}
	}
	return consumed
}

func (a *App) updateWindowTitle(title string) {
	if a.window == nil {
		return
	}
	if a.cfg.Window.DynamicTitle && title != "" {
		a.window.SetTitle("CervTerm · " + title)
		return
	}
	a.window.SetTitle("CervTerm")
}

func (a *App) paneAtWindowPosition(x, y float64) (termmux.PaneID, termsel.Point, bool) {
	if a.window == nil {
		return 0, termsel.Point{}, false
	}
	windowW, windowH := a.window.GetSize()
	fbW, fbH := a.window.GetFramebufferSize()
	if windowW <= 0 || windowH <= 0 {
		return 0, termsel.Point{}, false
	}
	fx := float32(x) * float32(fbW) / float32(windowW)
	fy := float32(y) * float32(fbH) / float32(windowH)
	layout, err := a.mux.Layout()
	if err != nil {
		return 0, termsel.Point{}, false
	}
	for _, pane := range layout.Panes {
		r := pane.Pixels
		if fx < float32(r.X) || fx >= float32(r.Right()) || fy < float32(r.Y) || fy >= float32(r.Bottom()) {
			continue
		}
		point := a.pointForPaneFramebufferPosition(pane.Pane, pane, fx, fy)
		return pane.Pane, point, true
	}
	return 0, termsel.Point{}, false
}

func (a *App) pointForPaneWindowPosition(id termmux.PaneID, x, y float64) (termsel.Point, bool) {
	if a.window == nil {
		return termsel.Point{}, false
	}
	view, ok := a.mux.PaneView(id)
	if !ok {
		return termsel.Point{}, false
	}
	windowW, windowH := a.window.GetSize()
	fbW, fbH := a.window.GetFramebufferSize()
	if windowW <= 0 || windowH <= 0 {
		return termsel.Point{}, false
	}
	fx := float32(x) * float32(fbW) / float32(windowW)
	fy := float32(y) * float32(fbH) / float32(windowH)
	return a.pointForPaneFramebufferPosition(id, view.Geometry, fx, fy), true
}

func (a *App) paneGridMetrics(id termmux.PaneID, cols, rows int) gridMetrics {
	cellW, cellH := a.cellW, a.cellH
	if state := a.paneUI[id]; state != nil {
		cellW, cellH = state.font.cellW, state.font.cellH
	}
	return gridMetrics{cellW: cellW, cellH: cellH, cols: cols, rows: rows}
}

func (a *App) pointForPaneFramebufferPosition(id termmux.PaneID, geometry termmux.PaneGeometry, fx, fy float32) termsel.Point {
	localX := fx - float32(geometry.Pixels.X)
	localY := fy - float32(geometry.Pixels.Y)
	row, col := a.paneGridMetrics(id, geometry.Cols, geometry.Rows).cellAt(localX, localY)
	return termsel.Point{Row: row, Col: col}
}

func (a *App) updateHoverForPane(id termmux.PaneID, x, y float64) {
	for pane, state := range a.paneUI {
		if pane != id {
			state.link.hoverActive = false
		}
	}
	state := a.ensurePaneUI(id)
	point, pointOK := a.pointForPaneWindowPosition(id, x, y)
	link, linkOK := linkRegion{}, false
	if pointOK {
		link, linkOK = linkAt(state.link.links, point)
	}
	changed := linkOK != state.link.hoverActive || link != state.link.hover
	state.link.hover, state.link.hoverActive = link, linkOK
	if id == a.focusedPane {
		a.link = state.link
	}
	if a.window != nil {
		if linkOK {
			if state.link.handCursor == nil {
				state.link.handCursor = glfw.CreateStandardCursor(glfw.HandCursor)
			}
			a.window.SetCursor(state.link.handCursor)
		} else {
			a.window.SetCursor(nil)
		}
	}
	if changed {
		a.requestRedraw()
	}
}

func (a *App) sendPaneFocus(id termmux.PaneID, focused bool) {
	view, ok := a.mux.PaneView(id)
	if !ok || !view.FocusEvents || view.State != termmux.PaneStateRunning {
		return
	}
	sequence := []byte("\x1b[O")
	if focused {
		sequence = []byte("\x1b[I")
	}
	_, _ = a.mux.Write(id, sequence)
}

func (a *App) writePaneInput(id termmux.PaneID, data []byte) error {
	_, err := a.mux.Write(id, data)
	return err
}
