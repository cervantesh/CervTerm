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

// metrics snapshots the App's current grid geometry as a plain value object.
func (a *App) metrics() gridMetrics {
	return gridMetrics{cellW: a.cellW, cellH: a.cellH, paddingX: a.paddingX, paddingY: a.paddingY, cols: a.cols, rows: a.rows}
}

// pointFromPixels maps a window pixel to the grid cell under it, clamped to the
// visible grid. The geometry itself lives in gridMetrics; this thin wrapper just
// adapts it to the selection Point type callers expect.
func (a *App) pointFromPixels(x, y float32) termsel.Point {
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

func (a *App) mouseMode() core.MouseMode {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.MouseMode()
}

func (a *App) sendMouseButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
	mode := a.mouseMode()
	if !mode.ReportsMouse() || mods&glfw.ModShift != 0 {
		return false
	}
	mouseButton, ok := mouseButtonFromGLFW(button)
	if !ok {
		return false
	}
	x, y := a.window.GetCursorPos()
	point := a.pointFromPixels(float32(x), float32(y))
	mouseAction := input.MousePress
	if action == glfw.Release {
		mouseAction = input.MouseRelease
		a.mouseReport.down = false
	} else if action == glfw.Press {
		a.mouseReport.down = true
		a.mouseReport.button = mouseButton
		a.mouseReport.mods = mouseModsFromGLFW(mods)
	} else {
		return false
	}
	encoded, ok := input.EncodeMouse(input.MouseEvent{Button: mouseButton, Action: mouseAction, Row: point.Row, Col: point.Col, Mods: mouseModsFromGLFW(mods), SGR: mode.SGR})
	if !ok {
		return false
	}
	a.writeInputBytes(encoded)
	return true
}

func (a *App) sendMouseMove(x, y float64) bool {
	mode := a.mouseMode()
	if !mode.ButtonEventTracking && !mode.AnyEventTracking {
		return false
	}
	button := a.mouseReport.button
	mods := a.mouseReport.mods
	if !a.mouseReport.down {
		if !mode.AnyEventTracking {
			return false
		}
		button = input.MouseNone
		mods = input.ModNone
	}
	point := a.pointFromPixels(float32(x), float32(y))
	encoded, ok := input.EncodeMouse(input.MouseEvent{Button: button, Action: input.MouseMove, Row: point.Row, Col: point.Col, Mods: mods, SGR: mode.SGR})
	if !ok {
		return false
	}
	a.writeInputBytes(encoded)
	return true
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
