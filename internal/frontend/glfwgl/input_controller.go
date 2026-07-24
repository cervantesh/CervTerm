//go:build glfw

package glfwgl

import (
	terminput "cervterm/internal/input"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type keyRouteEvent struct {
	key    glfw.Key
	action glfw.Action
	mods   glfw.ModifierKey
}

type buttonRouteEvent struct {
	button glfw.MouseButton
	action glfw.Action
	mods   glfw.ModifierKey
}

type cursorRouteEvent struct {
	x float64
	y float64
}

type wheelRouteEvent struct {
	xoff float64
	yoff float64
}

// inputModalPort keeps modal capture first for every routed input family.
type inputModalPort interface {
	routeModalKey(keyRouteEvent) bool
	routeModalButton(buttonRouteEvent) bool
	routeModalCursor(cursorRouteEvent) bool
	routeModalWheel(wheelRouteEvent) bool
}

type inputCursorPositionPort interface {
	inputCursorPosition() cursorRouteEvent
}

type inputKeyLifecyclePort interface {
	clearKeyCharacterSuppression()
}

type inputKeyReservedPort interface {
	routeSearchKey(keyRouteEvent, bool) bool
	routeReloadKey(keyRouteEvent, bool) bool
}

type inputKeyBindingPort interface {
	routeScriptTableKey(keyRouteEvent, bool) bool
	routeScriptKey(keyRouteEvent, bool) bool
	routeBuiltinKey(keyRouteEvent, bool) bool
}

type inputKeyTerminalPort interface {
	routeSelectionCopyKey(keyRouteEvent, bool) bool
	routeTerminalKey(terminput.Event)
}

// inputButtonPriorityPort preserves the outer ordering around the grouped
// terminal-report-then-configured route.
type inputButtonPriorityPort interface {
	routeTabBarButton(buttonRouteEvent, cursorRouteEvent) bool
	routeReportedOrConfiguredButton(buttonRouteEvent, cursorRouteEvent) bool
	routeActiveDividerButton(buttonRouteEvent, cursorRouteEvent) bool
	routeBeginDividerButton(buttonRouteEvent, cursorRouteEvent) bool
}

type inputButtonContentPort interface {
	routeScrollbarButton(buttonRouteEvent, cursorRouteEvent) bool
	routeSelectionButton(buttonRouteEvent, cursorRouteEvent)
}

type inputCursorPriorityPort interface {
	routeTabBarCursor(cursorRouteEvent) bool
	routeCapturedCursor(cursorRouteEvent) bool
	routeConfiguredDrag(cursorRouteEvent) bool
	routeDividerDrag(cursorRouteEvent) bool
}

type inputCursorContentPort interface {
	routeScrollbarCursor(cursorRouteEvent) bool
	routeTerminalMouseMove(cursorRouteEvent) bool
	routeDividerCursor(cursorRouteEvent) bool
	routeSelectionCursor(cursorRouteEvent)
}

// inputWheelPort keeps terminal reporting and configured bindings grouped in
// one route so their existing internal precedence cannot be inverted. Zoom
// remains before scrollbar handling.
type inputWheelPort interface {
	routeTabBarWheel(cursorRouteEvent) bool
	routeReportedOrConfiguredWheel(wheelRouteEvent, cursorRouteEvent) bool
	routeZoomWheel(wheelRouteEvent) bool
	routeScrollbarWheel(wheelRouteEvent, cursorRouteEvent) bool
	routeTerminalWheel(wheelRouteEvent)
}

type inputFocusPort interface {
	recordInputFocus(bool)
	cleanupBlurInput()
	routeScriptFocus(bool)
	routeTerminalFocus(bool)
}

// inputControllerTemporaryPortBudget explicitly bounds the aggregate App adapter
// while this preparatory slice preserves all existing input concerns.
// TODO(L1-01; expires Slice 6.3d): replace the aggregate adapter at facade closure.
const inputControllerTemporaryPortBudget = 36

// inputController owns input precedence only. Concrete App state and native
// handles remain behind consumer-defined concern-specific ports.
// TODO(L1-06; expires Slice 6.1b): replace fixed route order with typed routes.
type inputController struct {
	modal          inputModalPort
	positions      inputCursorPositionPort
	keyLifecycle   inputKeyLifecyclePort
	keyReserved    inputKeyReservedPort
	keyBindings    inputKeyBindingPort
	keyTerminal    inputKeyTerminalPort
	buttonPriority inputButtonPriorityPort
	buttonContent  inputButtonContentPort
	cursorPriority inputCursorPriorityPort
	cursorContent  inputCursorContentPort
	wheel          inputWheelPort
	focus          inputFocusPort
}

func newInputController(
	modal inputModalPort,
	positions inputCursorPositionPort,
	keyLifecycle inputKeyLifecyclePort,
	keyReserved inputKeyReservedPort,
	keyBindings inputKeyBindingPort,
	keyTerminal inputKeyTerminalPort,
	buttonPriority inputButtonPriorityPort,
	buttonContent inputButtonContentPort,
	cursorPriority inputCursorPriorityPort,
	cursorContent inputCursorContentPort,
	wheel inputWheelPort,
	focus inputFocusPort,
) *inputController {
	return &inputController{
		modal:          modal,
		positions:      positions,
		keyLifecycle:   keyLifecycle,
		keyReserved:    keyReserved,
		keyBindings:    keyBindings,
		keyTerminal:    keyTerminal,
		buttonPriority: buttonPriority,
		buttonContent:  buttonContent,
		cursorPriority: cursorPriority,
		cursorContent:  cursorContent,
		wheel:          wheel,
		focus:          focus,
	}
}

func (c *inputController) handleKey(key glfw.Key, action glfw.Action, mods glfw.ModifierKey) {
	event := keyRouteEvent{key: key, action: action, mods: mods}
	if action == glfw.Press || action == glfw.Repeat {
		c.keyLifecycle.clearKeyCharacterSuppression()
	}
	if c.modal.routeModalKey(event) {
		return
	}
	if action != glfw.Press && action != glfw.Repeat {
		return
	}

	repeat := action == glfw.Repeat
	if c.keyReserved.routeSearchKey(event, repeat) {
		return
	}
	if c.keyReserved.routeReloadKey(event, repeat) {
		return
	}
	if c.keyBindings.routeScriptTableKey(event, repeat) {
		return
	}
	if c.keyBindings.routeScriptKey(event, repeat) {
		return
	}
	if c.keyBindings.routeBuiltinKey(event, repeat) {
		return
	}

	terminalEvent, ok := inputEventFromGLFW(key, mods)
	if !ok {
		return
	}
	if key == glfw.KeyC && mods&glfw.ModControl != 0 && c.keyTerminal.routeSelectionCopyKey(event, repeat) {
		return
	}
	c.keyTerminal.routeTerminalKey(terminalEvent)
}

func (c *inputController) handleButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
	event := buttonRouteEvent{button: button, action: action, mods: mods}
	if c.modal.routeModalButton(event) {
		return
	}
	position := c.positions.inputCursorPosition()
	if c.buttonPriority.routeTabBarButton(event, position) {
		return
	}
	if c.buttonPriority.routeReportedOrConfiguredButton(event, position) {
		return
	}
	if c.buttonPriority.routeActiveDividerButton(event, position) {
		return
	}
	if c.buttonPriority.routeBeginDividerButton(event, position) {
		return
	}
	if c.buttonContent.routeScrollbarButton(event, position) {
		return
	}
	c.buttonContent.routeSelectionButton(event, position)
}

func (c *inputController) handleCursor(x, y float64) {
	position := cursorRouteEvent{x: x, y: y}
	if c.modal.routeModalCursor(position) {
		return
	}
	if c.cursorPriority.routeTabBarCursor(position) {
		return
	}
	if c.cursorPriority.routeCapturedCursor(position) {
		return
	}
	if c.cursorPriority.routeConfiguredDrag(position) {
		return
	}
	if c.cursorPriority.routeDividerDrag(position) {
		return
	}
	if c.cursorContent.routeScrollbarCursor(position) {
		return
	}
	reported := c.cursorContent.routeTerminalMouseMove(position)
	if c.cursorContent.routeDividerCursor(position) {
		return
	}
	if reported {
		return
	}
	c.cursorContent.routeSelectionCursor(position)
}

func (c *inputController) handleWheel(xoff, yoff float64) {
	event := wheelRouteEvent{xoff: xoff, yoff: yoff}
	if c.modal.routeModalWheel(event) {
		return
	}
	position := c.positions.inputCursorPosition()
	if c.wheel.routeTabBarWheel(position) {
		return
	}
	if c.wheel.routeReportedOrConfiguredWheel(event, position) {
		return
	}
	if c.wheel.routeZoomWheel(event) {
		return
	}
	if c.wheel.routeScrollbarWheel(event, position) {
		return
	}
	c.wheel.routeTerminalWheel(event)
}

func (c *inputController) handleFocus(focused bool) {
	c.focus.recordInputFocus(focused)
	if !focused {
		c.focus.cleanupBlurInput()
	}
	c.focus.routeScriptFocus(focused)
	c.focus.routeTerminalFocus(focused)
}
