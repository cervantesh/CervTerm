//go:build glfw

package glfwgl

import (
	"errors"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	termsel "cervterm/internal/selection"
)

var (
	_ scriptHostConfigPort       = (*App)(nil)
	_ scriptHostInputPort        = (*App)(nil)
	_ scriptHostNotificationPort = (*App)(nil)
	_ scriptHostFontPort         = (*App)(nil)
	_ scriptHostSelectionPort    = (*App)(nil)
	_ scriptHostViewPort         = (*App)(nil)
	_ scriptHostMutationPort     = (*App)(nil)
)

func (a *App) scriptHostRuntimeConfig() config.Config { return a.cfg.Clone() }

func (a *App) applyScriptHostRuntimeConfig(next config.Config) error {
	return a.applyLiveConfig(next)
}

func (a *App) requestScriptHostConfigReload() bool { return a.requestConfigReload() }

func (a *App) writeScriptHostInput(pane termmux.PaneID, data string) {
	if pane == 0 {
		return
	}
	events, err := a.mux.Write(pane, []byte(data))
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := a.mux.PaneView(pane); ok && view.State == termmux.PaneStateFailed {
			events, err = a.mux.FeedFallback(pane, []byte(data))
		}
	}
	if err != nil {
		a.Notify("input: " + err.Error())
	}
	a.pendingMuxEvents = append(a.pendingMuxEvents, events...)
	if len(events) > 0 {
		a.requestRedraw()
	}
}

func (a *App) notifyScriptHost(message string) {
	a.notice = message
	a.noticeUntil = time.Now().Add(4 * time.Second)
	a.requestRedraw()
}

func (a *App) setScriptHostClipboard(text string) {
	if a.clipboardSetter != nil {
		a.clipboardSetter(text)
		return
	}
	if a.window != nil {
		a.window.SetClipboardString(text)
	}
}

func (a *App) scriptHostClipboard() string {
	if a.window == nil {
		return ""
	}
	return a.window.GetClipboardString()
}

func (a *App) scriptHostFontSize(pane termmux.PaneID) float64 {
	if pane != 0 {
		return a.paneFontSize(pane)
	}
	// Preserve the pre-controller paneHost literal adapter: an unspecified pane
	// follows the focused pane once mux bootstrap has completed.
	if focused, ok := a.focusedFontPane(); ok {
		return a.paneFontSize(focused)
	}
	return a.cfg.Font.Size
}

func (a *App) setScriptHostFontSize(pane termmux.PaneID, points float64) {
	points = clampZoomFontSize(points)
	if pane != 0 {
		a.setPaneFontSize(pane, points)
		return
	}
	// Preserve the pre-controller paneHost literal adapter without retargeting
	// production hosts, whose initialized controller retains its stable pane ID.
	if focused, ok := a.focusedFontPane(); ok {
		state := a.ensurePaneUI(focused)
		if state.font.fontSize == points && !state.font.pending {
			return
		}
		a.setPaneFontSize(focused, points)
		return
	}
	// Before mux bootstrap, scripting configures the base used by the initial
	// pane and by reset; there is no pane-local state or PTY to update yet.
	a.cfg.Font.Size = points
	a.zoom.base = points
}

func (a *App) scriptHostSelection(pane termmux.PaneID) string {
	if pane == a.focusedPane {
		a.saveActivePaneUI()
	}
	state := a.ensurePaneUI(pane)
	if !state.selection.active {
		return ""
	}
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return ""
	}
	return termsel.TextWithWrapped(view.Snapshot.Cells, view.Snapshot.Wrapped, view.Snapshot.Cols, view.Snapshot.Rows, termsel.Range{Start: state.selection.start, End: state.selection.end})
}

func (a *App) scrollScriptHost(pane termmux.PaneID, lines int) bool {
	moved, _ := a.mux.ScrollViewport(pane, lines)
	if moved {
		a.recordPaneScroll(pane)
		a.requestAccessibilityRedraw()
	}
	return moved
}

func (a *App) scrollScriptHostToBottom(pane termmux.PaneID) {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return
	}
	if moved, _ := a.mux.ScrollViewport(pane, -view.ScrollbackLines); moved {
		a.recordPaneScroll(pane)
		a.requestAccessibilityRedraw()
	}
}

func (a *App) scriptHostScrollbackLen(pane termmux.PaneID) int {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return 0
	}
	return view.ScrollbackLines
}

func (a *App) scriptHostSize(pane termmux.PaneID) (int, int) {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return 0, 0
	}
	return view.Snapshot.Cols, view.Snapshot.Rows
}

func (a *App) scriptHostCursor(pane termmux.PaneID) (int, int) {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return 0, 0
	}
	return view.Snapshot.CursorRow, view.Snapshot.CursorCol
}

func (a *App) scriptHostTitle(pane termmux.PaneID) string {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return ""
	}
	return view.Snapshot.Title
}

func (a *App) scriptHostCWD(pane termmux.PaneID) string {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return ""
	}
	return view.Snapshot.Cwd
}

func (a *App) scriptHostLine(pane termmux.PaneID, row int) (string, bool) {
	return a.mux.Line(pane, row)
}

func (a *App) setScriptHostTitle(pane termmux.PaneID, title string) {
	changed, _ := a.mux.SetTitle(pane, title)
	if changed {
		a.pendingMuxEvents = append(a.pendingMuxEvents, termmux.Event{Kind: termmux.PaneTitleChanged, Pane: pane, Text: title}, termmux.Event{Kind: termmux.PaneDirty, Pane: pane})
		a.requestRedraw()
	}
}

func (a *App) scriptHostLineWrapped(pane termmux.PaneID, row int) (bool, bool) {
	return a.mux.LineWrapped(pane, row)
}

func (a *App) searchScriptHost(pane termmux.PaneID, query string) bool {
	if query == "" {
		return false
	}
	row, col, ok, _ := a.mux.SearchUpward(pane, query, false, 0)
	if !ok {
		return false
	}
	state := a.ensurePaneUI(pane)
	state.search.matchRow, state.search.matchCol = row, col
	state.search.matchLen = len([]rune(query))
	state.search.hasMatch = true
	if pane == a.focusedPane {
		a.loadPaneUI(pane)
	}
	a.requestRedraw()
	return true
}
