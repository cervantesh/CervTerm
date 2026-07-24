//go:build glfw

package glfwgl

import (
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) installCallbacks() {
	a.window.SetContentScaleCallback(func(_ *glfw.Window, scaleX, scaleY float32) {
		a.invalidateCandidateGeometry()
		a.rebuildForContentScale(scaleX, scaleY)
		a.requestAccessibilityRedraw()
	})
	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, _, _ int) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
	a.window.SetSizeCallback(func(_ *glfw.Window, _, _ int) {
		a.invalidateCandidateGeometry()
		a.requestAccessibilityRedraw()
	})
	a.installAccessibilityWindowCallbacks()
	a.window.SetCharCallback(func(_ *glfw.Window, char rune) {
		a.routeGLFWChar(char)
	})
	a.window.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		a.handleKeyEvent(key, action, mods)
	})

	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		a.ensureInputController().handleButton(button, action, mods)
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		a.ensureInputController().handleCursor(x, y)
	})
	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		a.ensureInputController().handleWheel(xoff, yoff)
	})
	a.window.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		a.ensureInputController().handleFocus(focused)
	})
}

func (a *App) routeModalButton(event buttonRouteEvent) bool {
	return a.handleModalMouseButton(event.button, event.action, event.mods)
}

func (a *App) inputCursorPosition() cursorRouteEvent {
	x, y := a.window.GetCursorPos()
	return cursorRouteEvent{x: x, y: y}
}

func (a *App) routeTabBarButton(event buttonRouteEvent, position cursorRouteEvent) bool {
	return a.handleTabBarButton(event.button, event.action, position.x, position.y)
}

func (a *App) routeReportedOrConfiguredButton(event buttonRouteEvent, position cursorRouteEvent) bool {
	return a.handleConfiguredMouseButton(event.button, event.action, event.mods, position.x, position.y)
}

func (a *App) routeActiveDividerButton(event buttonRouteEvent, position cursorRouteEvent) bool {
	if !a.divider.active {
		return false
	}
	if event.button == glfw.MouseButtonLeft && event.action == glfw.Release {
		a.finishDividerDrag()
		a.updateDividerCursor(position.x, position.y)
	}
	return true
}

func (a *App) routeBeginDividerButton(event buttonRouteEvent, position cursorRouteEvent) bool {
	return event.button == glfw.MouseButtonLeft && event.action == glfw.Press && a.mouseCapturePane == 0 && a.beginDividerDrag(position.x, position.y)
}

func (a *App) routeScrollbarButton(event buttonRouteEvent, position cursorRouteEvent) bool {
	fx, fy := a.windowToFramebuffer(position.x, position.y)
	if !a.handleScrollbarButton(event.button, event.action, fx, fy) {
		return false
	}
	a.clearDividerCursor()
	return true
}

func (a *App) routeSelectionButton(event buttonRouteEvent, position cursorRouteEvent) {
	if event.button != glfw.MouseButtonLeft {
		return
	}
	point := a.pointFromPixels(float32(position.x), float32(position.y))
	if event.action == glfw.Press {
		a.captureLinkPress(point)
		a.selection.dragging = true
		a.selection.active = false
		a.selection.start = point
		a.selection.end = point
		a.clearHover()
		a.requestAccessibilityRedraw()
		return
	}
	if event.action == glfw.Release {
		a.selection.end = point
		a.selection.dragging = false
		if !a.selection.active && a.handleLinkClick(point) {
			a.requestRedraw()
			return
		}
		a.requestAccessibilityRedraw()
	}
}

func (a *App) routeModalCursor(position cursorRouteEvent) bool {
	return a.handleModalCursorPos(position.x, position.y)
}

func (a *App) routeTabBarCursor(position cursorRouteEvent) bool {
	if !a.pointerOverTabBar(position.x, position.y) {
		return false
	}
	a.clearDividerCursor()
	return true
}

func (a *App) routeCapturedCursor(position cursorRouteEvent) bool {
	if a.mouseCapturePane == 0 {
		return false
	}
	a.clearDividerCursor()
	a.sendMouseMove(position.x, position.y)
	return true
}

func (a *App) routeConfiguredDrag(position cursorRouteEvent) bool {
	return a.handleConfiguredMouseDrag(position.x, position.y)
}

func (a *App) routeDividerDrag(position cursorRouteEvent) bool {
	return a.dragDivider(position.x, position.y)
}

func (a *App) routeScrollbarCursor(position cursorRouteEvent) bool {
	fx, fy := a.windowToFramebuffer(position.x, position.y)
	if !a.handleScrollbarMove(fx, fy) {
		return false
	}
	a.clearDividerCursor()
	return true
}

func (a *App) routeTerminalMouseMove(position cursorRouteEvent) bool {
	return a.sendMouseMove(position.x, position.y)
}

func (a *App) routeDividerCursor(position cursorRouteEvent) bool {
	return a.updateDividerCursor(position.x, position.y)
}

func (a *App) routeSelectionCursor(position cursorRouteEvent) {
	if !a.selection.dragging {
		if pane, _, ok := a.paneAtWindowPosition(position.x, position.y); ok {
			a.updateHoverForPane(pane, position.x, position.y)
		} else {
			a.clearHover()
		}
		return
	}
	a.selection.end = a.pointFromPixels(float32(position.x), float32(position.y))
	a.selection.active = true
	a.requestAccessibilityRedraw()
}

func (a *App) routeModalWheel(event wheelRouteEvent) bool {
	return a.handleModalScroll(event.xoff, event.yoff)
}

func (a *App) routeTabBarWheel(position cursorRouteEvent) bool {
	return a.pointerOverTabBar(position.x, position.y)
}

func (a *App) routeReportedOrConfiguredWheel(event wheelRouteEvent, position cursorRouteEvent) bool {
	return a.handleConfiguredMouseWheel(event.yoff, position.x, position.y)
}

func (a *App) routeZoomWheel(event wheelRouteEvent) bool {
	return a.handleZoomWheel(event.yoff)
}

func (a *App) routeScrollbarWheel(event wheelRouteEvent, position cursorRouteEvent) bool {
	fx, fy := a.windowToFramebuffer(position.x, position.y)
	return a.handleScrollbarWheel(event.yoff, fx, fy)
}

func (a *App) routeTerminalWheel(event wheelRouteEvent) {
	rows := scrollRowsFromWheelDelta(event.yoff, a.cfg.Scrolling.WheelMultiplier)
	if rows == 0 {
		return
	}
	moved, _ := a.mux.ScrollViewport(a.focusedPane, rows)
	if moved {
		a.scrollbar.lastActivity = time.Now()
		a.requestAccessibilityRedraw()
		a.markScrollEvent()
	}
}

func (a *App) recordInputFocus(focused bool) {
	a.recordNativeFocus(focused)
}

func (a *App) cleanupBlurInput() {
	a.compositionNativeFocusChanged(false)
	a.keyTable.cancel()
	a.finishDividerDrag()
	a.clearDividerCursor()
	a.cancelMouseCapture()
	a.mouseBindingCapture = mouseBindingCapture{}
}

func (a *App) routeScriptFocus(focused bool) {
	a.fireScriptEvent(func() error { return a.fireScriptFocus(a.hostForFocused().pane, focused) })
}

func (a *App) routeTerminalFocus(focused bool) {
	_, view, ok := a.focusedView()
	enabled := ok && view.FocusEvents
	if !enabled {
		return
	}
	if focused {
		a.writeInput("\x1b[I")
	} else {
		a.writeInput("\x1b[O")
	}
}
