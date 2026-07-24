//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

// scriptHostControllerPortBudget pins the complete temporary paneHost surface.
// Config values cross the seam only as detached method values; the controller
// retains no config, App, mux, runtime, or native owner.
const scriptHostControllerPortBudget = 21

type scriptHostConfigPort interface {
	scriptHostRuntimeConfig() config.Config
	applyScriptHostRuntimeConfig(config.Config) error
	requestScriptHostConfigReload() bool
}

type scriptHostInputPort interface {
	writeScriptHostInput(termmux.PaneID, string)
}

type scriptHostNotificationPort interface {
	notifyScriptHost(string)
	setScriptHostClipboard(string)
	scriptHostClipboard() string
}

type scriptHostFontPort interface {
	scriptHostFontSize(termmux.PaneID) float64
	setScriptHostFontSize(termmux.PaneID, float64)
}

type scriptHostSelectionPort interface {
	scriptHostSelection(termmux.PaneID) string
	scrollScriptHost(termmux.PaneID, int) bool
	scrollScriptHostToBottom(termmux.PaneID)
	scriptHostScrollbackLen(termmux.PaneID) int
}

type scriptHostViewPort interface {
	scriptHostSize(termmux.PaneID) (int, int)
	scriptHostCursor(termmux.PaneID) (int, int)
	scriptHostTitle(termmux.PaneID) string
	scriptHostCWD(termmux.PaneID) string
	scriptHostLine(termmux.PaneID, int) (string, bool)
}

type scriptHostMutationPort interface {
	setScriptHostTitle(termmux.PaneID, string)
	scriptHostLineWrapped(termmux.PaneID, int) (bool, bool)
	searchScriptHost(termmux.PaneID, string) bool
}

// scriptHostController binds the script host surface to one stable pane ID.
// It owns only routing and detached-config boundaries; all mutable state and
// effects remain behind concern-specific ports until the later move/wire work.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type scriptHostController struct {
	pane          termmux.PaneID
	config        scriptHostConfigPort
	input         scriptHostInputPort
	notifications scriptHostNotificationPort
	fonts         scriptHostFontPort
	selection     scriptHostSelectionPort
	view          scriptHostViewPort
	mutations     scriptHostMutationPort
}

func newScriptHostController(
	pane termmux.PaneID,
	config scriptHostConfigPort,
	input scriptHostInputPort,
	notifications scriptHostNotificationPort,
	fonts scriptHostFontPort,
	selection scriptHostSelectionPort,
	view scriptHostViewPort,
	mutations scriptHostMutationPort,
) *scriptHostController {
	return &scriptHostController{
		pane: pane, config: config, input: input, notifications: notifications,
		fonts: fonts, selection: selection, view: view, mutations: mutations,
	}
}

func (c *scriptHostController) RuntimeConfig() config.Config {
	return c.config.scriptHostRuntimeConfig().Clone()
}

func (c *scriptHostController) ApplyRuntimeConfig(next config.Config) error {
	return c.config.applyScriptHostRuntimeConfig(next.Clone())
}

func (c *scriptHostController) RequestConfigReload() bool {
	return c.config.requestScriptHostConfigReload()
}

func (c *scriptHostController) WriteInput(data string) {
	if c.pane == 0 {
		return
	}
	c.input.writeScriptHostInput(c.pane, data)
}

func (c *scriptHostController) Notify(message string) {
	c.notifications.notifyScriptHost(message)
}

func (c *scriptHostController) SetClipboard(text string) {
	c.notifications.setScriptHostClipboard(text)
}

func (c *scriptHostController) Clipboard() string {
	return c.notifications.scriptHostClipboard()
}

func (c *scriptHostController) FontSize() float64 {
	return c.fonts.scriptHostFontSize(c.pane)
}

func (c *scriptHostController) SetFontSize(points float64) {
	c.fonts.setScriptHostFontSize(c.pane, points)
}

func (c *scriptHostController) Selection() string {
	return c.selection.scriptHostSelection(c.pane)
}

func (c *scriptHostController) Scroll(lines int) bool {
	return c.selection.scrollScriptHost(c.pane, lines)
}

func (c *scriptHostController) ScrollToBottom() {
	c.selection.scrollScriptHostToBottom(c.pane)
}

func (c *scriptHostController) ScrollbackLen() int {
	return c.selection.scriptHostScrollbackLen(c.pane)
}

func (c *scriptHostController) Size() (int, int) {
	return c.view.scriptHostSize(c.pane)
}

func (c *scriptHostController) Cursor() (int, int) {
	return c.view.scriptHostCursor(c.pane)
}

func (c *scriptHostController) Title() string {
	return c.view.scriptHostTitle(c.pane)
}

func (c *scriptHostController) Cwd() string {
	return c.view.scriptHostCWD(c.pane)
}

func (c *scriptHostController) SetTitle(title string) {
	c.mutations.setScriptHostTitle(c.pane, title)
}

func (c *scriptHostController) Line(row int) (string, bool) {
	return c.view.scriptHostLine(c.pane, row)
}

func (c *scriptHostController) LineWrapped(row int) (bool, bool) {
	return c.mutations.scriptHostLineWrapped(c.pane, row)
}

func (c *scriptHostController) Search(query string) bool {
	if query == "" {
		return false
	}
	return c.mutations.searchScriptHost(c.pane, query)
}
