//go:build glfw

package glfwgl

import (
	"math"

	"cervterm/internal/core"
	"cervterm/internal/input"
	termsel "cervterm/internal/selection"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// selectionState holds the text-selection state. Main-thread only.
type selectionState struct {
	dragging bool // a drag is in progress
	active   bool // a selection exists
	start    termsel.Point
	end      termsel.Point
}

// mouseReportState holds the in-flight mouse-reporting state (button held down
// for drag reports). Main-thread only.
type mouseReportState struct {
	down   bool
	button input.MouseButton
	mods   input.Mod
}

// metrics snapshots the focused pane's local grid geometry.
func (a *App) metrics() gridMetrics {
	_, view, ok := a.focusedView()
	if !ok {
		return gridMetrics{cellW: a.cellW, cellH: a.cellH, paddingX: a.paddingX, paddingY: a.paddingY, cols: a.cols, rows: a.rows}
	}
	return gridMetrics{cellW: a.cellW, cellH: a.cellH, paddingX: a.paddingX, paddingY: a.paddingY, cols: view.Geometry.Cols, rows: view.Geometry.Rows}
}

func (a *App) pointFromPixels(x, y float32) termsel.Point {
	if point, ok := a.pointForPaneWindowPosition(a.focusedPane, float64(x), float64(y)); ok {
		return point
	}
	row, col := a.metrics().cellAt(x, y)
	return termsel.Point{Row: row, Col: col}
}

func scrollRowsFromWheelDelta(yoff float64) int {
	if yoff == 0 {
		return 0
	}
	rows := int(math.Round(yoff * 3))
	if rows == 0 {
		if yoff > 0 {
			return 1
		}
		return -1
	}
	return rows
}

func (a *App) cancelMouseCapture() {
	id := a.mouseCapturePane
	if id == 0 {
		return
	}
	state := a.ensurePaneUI(id)
	state.mouseReport.down = false
	if id == a.focusedPane {
		a.mouseReport = state.mouseReport
	}
	a.mouseCapturePane = 0
}

func (a *App) mouseMode() core.MouseMode {
	_, view, ok := a.focusedView()
	if !ok {
		return core.MouseMode{}
	}
	return view.MouseMode
}

func (a *App) sendMouseButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
	mouseButton, ok := mouseButtonFromGLFW(button)
	if !ok {
		return false
	}
	target := a.focusedPane
	mode := a.mouseMode()
	report := &a.mouseReport
	if action == glfw.Release && a.mouseCapturePane != 0 {
		target = a.mouseCapturePane
		view, exists := a.mux.PaneView(target)
		if !exists {
			a.cancelMouseCapture()
			return false
		}
		mode = view.MouseMode
		report = &a.ensurePaneUI(target).mouseReport
		mouseButton = report.button
	} else if !mode.ReportsMouse() || mods&glfw.ModShift != 0 {
		return false
	}
	if action != glfw.Press && action != glfw.Release {
		return false
	}
	x, y := a.window.GetCursorPos()
	point, ok := a.pointForPaneWindowPosition(target, x, y)
	if !ok {
		if action == glfw.Release {
			a.cancelMouseCapture()
		}
		return false
	}
	mouseAction := input.MousePress
	if action == glfw.Release {
		mouseAction = input.MouseRelease
	} else {
		report.down = true
		report.button = mouseButton
		report.mods = mouseModsFromGLFW(mods)
		a.mouseCapturePane = target
		a.ensurePaneUI(target).mouseReport = *report
	}
	encoded, ok := input.EncodeMouse(input.MouseEvent{Button: mouseButton, Action: mouseAction, Row: point.Row, Col: point.Col, Mods: mouseModsFromGLFW(mods), SGR: mode.SGR})
	if action == glfw.Release {
		report.down = false
		a.ensurePaneUI(target).mouseReport = *report
		a.mouseCapturePane = 0
		if target == a.focusedPane {
			a.mouseReport = *report
		}
	}
	if !ok {
		return false
	}
	return a.writePaneInput(target, encoded) == nil
}

func (a *App) sendMouseMove(x, y float64) bool {
	target := a.mouseCapturePane
	button, mods := input.MouseNone, input.ModNone
	var mode core.MouseMode
	if target != 0 {
		view, ok := a.mux.PaneView(target)
		if !ok {
			a.mouseCapturePane = 0
			return false
		}
		mode = view.MouseMode
		report := a.ensurePaneUI(target).mouseReport
		if !report.down || (!mode.ButtonEventTracking && !mode.AnyEventTracking) {
			return false
		}
		button, mods = report.button, report.mods
	} else {
		pane, _, ok := a.paneAtWindowPosition(x, y)
		if !ok {
			return false
		}
		target = pane
		view, ok := a.mux.PaneView(target)
		if !ok || !view.MouseMode.AnyEventTracking {
			return false
		}
		mode = view.MouseMode
	}
	point, ok := a.pointForPaneWindowPosition(target, x, y)
	if !ok {
		return false
	}
	encoded, ok := input.EncodeMouse(input.MouseEvent{Button: button, Action: input.MouseMove, Row: point.Row, Col: point.Col, Mods: mods, SGR: mode.SGR})
	if !ok {
		return false
	}
	return a.writePaneInput(target, encoded) == nil
}

func (a *App) sendMouseWheel(yoff float64, mods glfw.ModifierKey) bool {
	mode := a.mouseMode()
	if !mode.ReportsMouse() || mods&glfw.ModShift != 0 || yoff == 0 {
		return false
	}
	button := input.MouseWheelDown
	if yoff > 0 {
		button = input.MouseWheelUp
	}
	x, y := a.window.GetCursorPos()
	point := a.pointFromPixels(float32(x), float32(y))
	encoded, ok := input.EncodeMouse(input.MouseEvent{Button: button, Action: input.MousePress, Row: point.Row, Col: point.Col, Mods: mouseModsFromGLFW(mods), SGR: mode.SGR})
	if !ok {
		return false
	}
	a.writeInputBytes(encoded)
	return true
}

func mouseButtonFromGLFW(button glfw.MouseButton) (input.MouseButton, bool) {
	switch button {
	case glfw.MouseButtonLeft:
		return input.MouseLeft, true
	case glfw.MouseButtonMiddle:
		return input.MouseMiddle, true
	case glfw.MouseButtonRight:
		return input.MouseRight, true
	default:
		return input.MouseLeft, false
	}
}

func mouseModsFromGLFW(mods glfw.ModifierKey) input.Mod {
	terminalMods := input.ModNone
	if mods&glfw.ModControl != 0 {
		terminalMods |= input.ModCtrl
	}
	if mods&glfw.ModAlt != 0 {
		terminalMods |= input.ModAlt
	}
	if mods&glfw.ModShift != 0 {
		terminalMods |= input.ModShift
	}
	return terminalMods
}
