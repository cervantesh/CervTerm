//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

// paneHost binds the existing script.Host surface to one immutable pane ID for
// the duration of a callback. Background output can therefore never redirect
// term:* operations to the currently focused sibling.
type paneHost struct {
	app  *App
	pane termmux.PaneID
}

func (h paneHost) RuntimeConfig() config.Config { return h.app.scriptHostRuntimeConfig() }

func (h paneHost) ApplyRuntimeConfig(next config.Config) error {
	return h.app.applyScriptHostRuntimeConfig(next)
}

func (h paneHost) RequestConfigReload() bool { return h.app.requestScriptHostConfigReload() }

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

func (a *App) Notify(message string) { a.notifyScriptHost(message) }

func (a *App) SetClipboard(text string) { a.setScriptHostClipboard(text) }

func (a *App) Clipboard() string { return a.scriptHostClipboard() }

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

func (h paneHost) WriteInput(data string) { h.app.writeScriptHostInput(h.pane, data) }

func (h paneHost) Notify(message string)    { h.app.notifyScriptHost(message) }
func (h paneHost) SetClipboard(text string) { h.app.setScriptHostClipboard(text) }
func (h paneHost) Clipboard() string        { return h.app.scriptHostClipboard() }
func (h paneHost) FontSize() float64        { return h.app.scriptHostFontSize(h.pane) }

func (h paneHost) SetFontSize(points float64) {
	h.app.setScriptHostFontSize(h.pane, points)
}

func (h paneHost) Selection() string { return h.app.scriptHostSelection(h.pane) }

func (h paneHost) Scroll(lines int) bool { return h.app.scrollScriptHost(h.pane, lines) }

func (h paneHost) ScrollToBottom() { h.app.scrollScriptHostToBottom(h.pane) }

func (h paneHost) ScrollbackLen() int { return h.app.scriptHostScrollbackLen(h.pane) }

func (h paneHost) Size() (int, int) { return h.app.scriptHostSize(h.pane) }

func (h paneHost) Cursor() (int, int) { return h.app.scriptHostCursor(h.pane) }

func (h paneHost) Title() string { return h.app.scriptHostTitle(h.pane) }

func (h paneHost) Cwd() string { return h.app.scriptHostCWD(h.pane) }

func (h paneHost) SetTitle(title string) { h.app.setScriptHostTitle(h.pane, title) }

func (h paneHost) Line(row int) (string, bool) { return h.app.scriptHostLine(h.pane, row) }

func (h paneHost) LineWrapped(row int) (bool, bool) {
	return h.app.scriptHostLineWrapped(h.pane, row)
}

func (h paneHost) Search(query string) bool { return h.app.searchScriptHost(h.pane, query) }
