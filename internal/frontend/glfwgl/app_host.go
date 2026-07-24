//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

// paneHost binds the existing script.Host surface to one immutable pane ID for
// the duration of a callback. Background output can therefore never redirect
// term:* operations to the currently focused sibling. The App is only the
// compatibility adapter; the controller retains no owner or port.
type paneHost struct {
	app        *App
	pane       termmux.PaneID
	controller scriptHostController
}

func newPaneHost(app *App, pane termmux.PaneID) paneHost {
	return paneHost{
		app:        app,
		pane:       pane,
		controller: newScriptHostController(pane),
	}
}

func (h paneHost) scriptHostRoute() scriptHostController {
	if h.controller.initialized {
		return h.controller
	}
	return newScriptHostController(h.pane)
}

func (h paneHost) RuntimeConfig() config.Config {
	return h.scriptHostRoute().runtimeConfig(h.app)
}

func (h paneHost) ApplyRuntimeConfig(next config.Config) error {
	return h.scriptHostRoute().applyRuntimeConfig(h.app, next)
}

func (h paneHost) RequestConfigReload() bool {
	return h.scriptHostRoute().requestConfigReload(h.app)
}

func (a *App) focusedScriptPane() termmux.PaneID {
	if a.mux == nil {
		return 0
	}
	id, _ := a.mux.FocusedPane()
	return id
}

func (a *App) hostForFocused() paneHost {
	return newPaneHost(a, a.focusedScriptPane())
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
func (a *App) Notify(message string)            { a.hostForFocused().Notify(message) }
func (a *App) SetClipboard(text string)         { a.hostForFocused().SetClipboard(text) }
func (a *App) Clipboard() string                { return a.hostForFocused().Clipboard() }
func (a *App) FontSize() float64 {
	pane, ok := a.focusedFontPane()
	if !ok {
		pane = 0
	}
	return newPaneHost(a, pane).FontSize()
}

func (a *App) SetFontSize(points float64) {
	points = clampZoomFontSize(points)
	pane, ok := a.focusedFontPane()
	if !ok {
		pane = 0
	} else {
		state := a.ensurePaneUI(pane)
		if state.font.fontSize == points && !state.font.pending {
			return
		}
	}
	newPaneHost(a, pane).SetFontSize(points)
}

func (h paneHost) WriteInput(data string) { h.scriptHostRoute().writeInput(h.app, data) }

func (h paneHost) Notify(message string) {
	h.scriptHostRoute().notify(h.app, message)
}

func (h paneHost) SetClipboard(text string) {
	h.scriptHostRoute().setClipboard(h.app, text)
}

func (h paneHost) Clipboard() string {
	return h.scriptHostRoute().clipboard(h.app)
}

func (h paneHost) FontSize() float64 {
	return h.scriptHostRoute().fontSize(h.app)
}

func (h paneHost) SetFontSize(points float64) {
	h.scriptHostRoute().setFontSize(h.app, points)
}

func (h paneHost) Selection() string {
	return h.scriptHostRoute().selectionText(h.app)
}

func (h paneHost) Scroll(lines int) bool {
	return h.scriptHostRoute().scroll(h.app, lines)
}

func (h paneHost) ScrollToBottom() {
	h.scriptHostRoute().scrollToBottom(h.app)
}

func (h paneHost) ScrollbackLen() int {
	return h.scriptHostRoute().scrollbackLen(h.app)
}

func (h paneHost) Size() (int, int) {
	return h.scriptHostRoute().size(h.app)
}

func (h paneHost) Cursor() (int, int) {
	return h.scriptHostRoute().cursor(h.app)
}

func (h paneHost) Title() string {
	return h.scriptHostRoute().title(h.app)
}

func (h paneHost) Cwd() string {
	return h.scriptHostRoute().cwd(h.app)
}

func (h paneHost) SetTitle(title string) {
	h.scriptHostRoute().setTitle(h.app, title)
}

func (h paneHost) Line(row int) (string, bool) {
	return h.scriptHostRoute().line(h.app, row)
}

func (h paneHost) LineWrapped(row int) (bool, bool) {
	return h.scriptHostRoute().lineWrapped(h.app, row)
}

func (h paneHost) Search(query string) bool {
	return h.scriptHostRoute().search(h.app, query)
}
