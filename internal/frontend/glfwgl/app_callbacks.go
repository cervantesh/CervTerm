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
		if a.handleModalMouseButton(button, action, mods) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.handleTabBarButton(button, action, x, y) {
			return
		}
		if a.handleConfiguredMouseButton(button, action, mods, x, y) {
			return
		}
		if a.divider.active {
			if button == glfw.MouseButtonLeft && action == glfw.Release {
				a.finishDividerDrag()
				a.updateDividerCursor(x, y)
			}
			return
		}
		if button == glfw.MouseButtonLeft && action == glfw.Press && a.mouseCapturePane == 0 && a.beginDividerDrag(x, y) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarButton(button, action, fx, fy) {
			a.clearDividerCursor()
			return
		}
		if button != glfw.MouseButtonLeft {
			return
		}
		point := a.pointFromPixels(float32(x), float32(y))
		if action == glfw.Press {
			a.captureLinkPress(point)
			a.selection.dragging = true
			a.selection.active = false
			a.selection.start = point
			a.selection.end = point
			a.clearHover()
			a.requestAccessibilityRedraw()
			return
		}
		if action == glfw.Release {
			a.selection.end = point
			a.selection.dragging = false
			if !a.selection.active && a.handleLinkClick(point) {
				a.requestRedraw()
				return
			}
			a.requestAccessibilityRedraw()
		}
	})
	a.window.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		if a.handleModalCursorPos(x, y) {
			return
		}
		if a.pointerOverTabBar(x, y) {
			a.clearDividerCursor()
			return
		}
		if a.mouseCapturePane != 0 {
			a.clearDividerCursor()
			a.sendMouseMove(x, y)
			return
		}
		if a.handleConfiguredMouseDrag(x, y) {
			return
		}
		if a.dragDivider(x, y) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarMove(fx, fy) {
			a.clearDividerCursor()
			return
		}
		reported := a.sendMouseMove(x, y)
		if a.updateDividerCursor(x, y) {
			return
		}
		if reported {
			return
		}
		if !a.selection.dragging {
			if pane, _, ok := a.paneAtWindowPosition(x, y); ok {
				a.updateHoverForPane(pane, x, y)
			} else {
				a.clearHover()
			}
			return
		}
		a.selection.end = a.pointFromPixels(float32(x), float32(y))
		a.selection.active = true
		a.requestAccessibilityRedraw()
	})
	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		if a.handleModalScroll(xoff, yoff) {
			return
		}
		x, y := a.window.GetCursorPos()
		if a.pointerOverTabBar(x, y) {
			return
		}
		if a.handleConfiguredMouseWheel(yoff, x, y) {
			return
		}
		if a.handleZoomWheel(yoff) {
			return
		}
		fx, fy := a.windowToFramebuffer(x, y)
		if a.handleScrollbarWheel(yoff, fx, fy) {
			return
		}
		rows := scrollRowsFromWheelDelta(yoff, a.cfg.Scrolling.WheelMultiplier)
		if rows == 0 {
			return
		}
		moved, _ := a.mux.ScrollViewport(a.focusedPane, rows)
		if moved {
			a.scrollbar.lastActivity = time.Now()
			a.requestAccessibilityRedraw()
			a.markScrollEvent()
		}
	})
	a.window.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		a.recordNativeFocus(focused)
		if !focused {
			a.compositionNativeFocusChanged(false)
			a.keyTable.cancel()
			a.finishDividerDrag()
			a.clearDividerCursor()
			a.cancelMouseCapture()
			a.mouseBindingCapture = mouseBindingCapture{}
		}
		a.fireScriptEvent(func() error { return a.scriptRT.FireFocus(a.hostForFocused(), focused) })
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
	})
}
