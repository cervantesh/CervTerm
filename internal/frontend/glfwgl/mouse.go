//go:build glfw

package glfwgl

import (
	"cervterm/internal/core"
	"cervterm/internal/input"

	"github.com/go-gl/glfw/v3.3/glfw"
)

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
		a.mouseReportDown = false
	} else if action == glfw.Press {
		a.mouseReportDown = true
		a.mouseReportButton = mouseButton
		a.mouseReportMods = mouseModsFromGLFW(mods)
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
	button := a.mouseReportButton
	mods := a.mouseReportMods
	if !a.mouseReportDown {
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
