//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

// scriptHostControllerPortBudget pins the complete temporary paneHost surface.
// Ports are passed ephemerally by paneHost; the controller retains only its
// stable pane target.
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

// scriptHostController binds routing policy to one stable pane ID. Mutable
// owners and effects cross only as operation-scoped narrow ports.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapters.
type scriptHostController struct {
	pane        termmux.PaneID
	initialized bool
}

func newScriptHostController(pane termmux.PaneID) scriptHostController {
	return scriptHostController{pane: pane, initialized: true}
}

func (c scriptHostController) runtimeConfig(port scriptHostConfigPort) config.Config {
	return port.scriptHostRuntimeConfig().Clone()
}

func (c scriptHostController) applyRuntimeConfig(port scriptHostConfigPort, next config.Config) error {
	return port.applyScriptHostRuntimeConfig(next.Clone())
}

func (c scriptHostController) requestConfigReload(port scriptHostConfigPort) bool {
	return port.requestScriptHostConfigReload()
}

func (c scriptHostController) writeInput(port scriptHostInputPort, data string) {
	if c.pane == 0 {
		return
	}
	port.writeScriptHostInput(c.pane, data)
}

func (scriptHostController) notify(port scriptHostNotificationPort, message string) {
	port.notifyScriptHost(message)
}

func (scriptHostController) setClipboard(port scriptHostNotificationPort, text string) {
	port.setScriptHostClipboard(text)
}

func (scriptHostController) clipboard(port scriptHostNotificationPort) string {
	return port.scriptHostClipboard()
}

func (c scriptHostController) fontSize(port scriptHostFontPort) float64 {
	return port.scriptHostFontSize(c.pane)
}

func (c scriptHostController) setFontSize(port scriptHostFontPort, points float64) {
	port.setScriptHostFontSize(c.pane, points)
}

func (c scriptHostController) selectionText(port scriptHostSelectionPort) string {
	return port.scriptHostSelection(c.pane)
}

func (c scriptHostController) scroll(port scriptHostSelectionPort, lines int) bool {
	return port.scrollScriptHost(c.pane, lines)
}

func (c scriptHostController) scrollToBottom(port scriptHostSelectionPort) {
	port.scrollScriptHostToBottom(c.pane)
}

func (c scriptHostController) scrollbackLen(port scriptHostSelectionPort) int {
	return port.scriptHostScrollbackLen(c.pane)
}

func (c scriptHostController) size(port scriptHostViewPort) (int, int) {
	return port.scriptHostSize(c.pane)
}

func (c scriptHostController) cursor(port scriptHostViewPort) (int, int) {
	return port.scriptHostCursor(c.pane)
}

func (c scriptHostController) title(port scriptHostViewPort) string {
	return port.scriptHostTitle(c.pane)
}

func (c scriptHostController) cwd(port scriptHostViewPort) string {
	return port.scriptHostCWD(c.pane)
}

func (c scriptHostController) setTitle(port scriptHostMutationPort, title string) {
	port.setScriptHostTitle(c.pane, title)
}

func (c scriptHostController) line(port scriptHostViewPort, row int) (string, bool) {
	return port.scriptHostLine(c.pane, row)
}

func (c scriptHostController) lineWrapped(port scriptHostMutationPort, row int) (bool, bool) {
	return port.scriptHostLineWrapped(c.pane, row)
}

func (c scriptHostController) search(port scriptHostMutationPort, query string) bool {
	if query == "" {
		return false
	}
	return port.searchScriptHost(c.pane, query)
}
