//go:build glfw

package glfwgl

import (
	"errors"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	termsel "cervterm/internal/selection"
)

// paneHost binds the existing script.Host surface to one immutable pane ID for
// the duration of a callback. Background output can therefore never redirect
// term:* operations to the currently focused sibling.
type paneHost struct {
	app  *App
	pane termmux.PaneID
}

func (h paneHost) RuntimeConfig() config.Config { return h.app.RuntimeConfig() }

func (h paneHost) ApplyRuntimeConfig(next config.Config) error {
	return h.app.ApplyRuntimeConfig(next)
}

func (h paneHost) RequestConfigReload() bool { return h.app.RequestConfigReload() }

func (a *App) hostForFocused() paneHost {
	id, _ := a.mux.FocusedPane()
	return paneHost{app: a, pane: id}
}

// App remains a script.Host compatibility adapter for call sites that are
// inherently focused-pane/window-level (keys, timers, focus events).
func (a *App) WriteInput(s string)              { a.hostForFocused().WriteInput(s) }
func (a *App) Selection() string                { return a.hostForFocused().Selection() }
func (a *App) Scroll(lines int) bool            { return a.hostForFocused().Scroll(lines) }
func (a *App) ScrollToBottom()                  { a.hostForFocused().ScrollToBottom() }
func (a *App) ScrollbackLen() int               { return a.hostForFocused().ScrollbackLen() }
func (a *App) Size() (int, int)                 { return a.hostForFocused().Size() }
func (a *App) Cursor() (int, int)               { return a.hostForFocused().Cursor() }
func (a *App) Title() string                    { return a.hostForFocused().Title() }
func (a *App) Cwd() string                      { return a.hostForFocused().Cwd() }
func (a *App) SetTitle(title string)            { a.hostForFocused().SetTitle(title) }
func (a *App) Line(row int) (string, bool)      { return a.hostForFocused().Line(row) }
func (a *App) LineWrapped(row int) (bool, bool) { return a.hostForFocused().LineWrapped(row) }
func (a *App) Search(query string) bool         { return a.hostForFocused().Search(query) }

func (a *App) Notify(msg string) {
	a.notice = msg
	a.noticeUntil = time.Now().Add(4 * time.Second)
	a.requestRedraw()
}

func (a *App) SetClipboard(text string) {
	if a.clipboardSetter != nil {
		a.clipboardSetter(text)
		return
	}
	if a.window != nil {
		a.window.SetClipboardString(text)
	}
}

func (a *App) Clipboard() string {
	if a.window == nil {
		return ""
	}
	return a.window.GetClipboardString()
}

func (a *App) FontSize() float64 {
	if id, ok := a.focusedFontPane(); ok {
		return a.paneFontSize(id)
	}
	return a.cfg.Font.Size
}

func (a *App) SetFontSize(pts float64) {
	pts = clampZoomFontSize(pts)
	if id, ok := a.focusedFontPane(); ok {
		state := a.ensurePaneUI(id)
		if state.font.fontSize == pts && !state.font.pending {
			return
		}
		a.setPaneFontSize(id, pts)
		return
	}
	// Before mux bootstrap, scripting configures the base used by the initial
	// pane and by reset; there is no pane-local state or PTY to update yet.
	a.cfg.Font.Size = pts
	a.zoom.base = pts
}

func (h paneHost) WriteInput(s string) {
	if h.pane == 0 {
		return
	}
	events, err := h.app.mux.Write(h.pane, []byte(s))
	if errors.Is(err, termmux.ErrPaneNotRunning) {
		if view, ok := h.app.mux.PaneView(h.pane); ok && view.State == termmux.PaneStateFailed {
			events, err = h.app.mux.FeedFallback(h.pane, []byte(s))
		}
	}
	if err != nil {
		h.app.Notify("input: " + err.Error())
	}
	h.app.pendingMuxEvents = append(h.app.pendingMuxEvents, events...)
	if len(events) > 0 {
		h.app.requestRedraw()
	}
}

func (h paneHost) Notify(message string)    { h.app.Notify(message) }
func (h paneHost) SetClipboard(text string) { h.app.SetClipboard(text) }
func (h paneHost) Clipboard() string        { return h.app.Clipboard() }
func (h paneHost) FontSize() float64 {
	if h.pane != 0 {
		return h.app.paneFontSize(h.pane)
	}
	return h.app.FontSize()
}

func (h paneHost) SetFontSize(points float64) {
	if h.pane != 0 {
		h.app.setPaneFontSize(h.pane, points)
		return
	}
	h.app.SetFontSize(points)
}

func (h paneHost) Selection() string {
	if h.pane == h.app.focusedPane {
		h.app.saveActivePaneUI()
	}
	state := h.app.ensurePaneUI(h.pane)
	if !state.selection.active {
		return ""
	}
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return ""
	}
	return termsel.TextWithWrapped(view.Snapshot.Cells, view.Snapshot.Wrapped, view.Snapshot.Cols, view.Snapshot.Rows, termsel.Range{Start: state.selection.start, End: state.selection.end})
}

func (h paneHost) Scroll(lines int) bool {
	moved, _ := h.app.mux.ScrollViewport(h.pane, lines)
	if moved {
		h.app.recordPaneScroll(h.pane)
		h.app.requestRedraw()
	}
	return moved
}

func (h paneHost) ScrollToBottom() {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return
	}
	if moved, _ := h.app.mux.ScrollViewport(h.pane, -view.ScrollbackLines); moved {
		h.app.recordPaneScroll(h.pane)
		h.app.requestRedraw()
	}
}

func (h paneHost) ScrollbackLen() int {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return 0
	}
	return view.ScrollbackLines
}

func (h paneHost) Size() (int, int) {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return 0, 0
	}
	return view.Snapshot.Cols, view.Snapshot.Rows
}

func (h paneHost) Cursor() (int, int) {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return 0, 0
	}
	return view.Snapshot.CursorRow, view.Snapshot.CursorCol
}

func (h paneHost) Title() string {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return ""
	}
	return view.Snapshot.Title
}

func (h paneHost) Cwd() string {
	view, ok := h.app.mux.PaneView(h.pane)
	if !ok {
		return ""
	}
	return view.Snapshot.Cwd
}

func (h paneHost) SetTitle(title string) {
	changed, _ := h.app.mux.SetTitle(h.pane, title)
	if changed {
		h.app.pendingMuxEvents = append(h.app.pendingMuxEvents, termmux.Event{Kind: termmux.PaneTitleChanged, Pane: h.pane, Text: title}, termmux.Event{Kind: termmux.PaneDirty, Pane: h.pane})
		h.app.requestRedraw()
	}
}

func (h paneHost) Line(row int) (string, bool) { return h.app.mux.Line(h.pane, row) }
func (h paneHost) LineWrapped(row int) (bool, bool) {
	return h.app.mux.LineWrapped(h.pane, row)
}

func (h paneHost) Search(query string) bool {
	if query == "" {
		return false
	}
	row, col, ok, _ := h.app.mux.SearchUpward(h.pane, query, false, 0)
	if !ok {
		return false
	}
	state := h.app.ensurePaneUI(h.pane)
	state.search.matchRow, state.search.matchCol = row, col
	state.search.matchLen = len([]rune(query))
	state.search.hasMatch = true
	if h.pane == h.app.focusedPane {
		h.app.loadPaneUI(h.pane)
	}
	h.app.requestRedraw()
	return true
}
