package input

import "fmt"

type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
)

type MouseAction int

const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMove
)

type MouseEvent struct {
	Button MouseButton
	Action MouseAction
	Row    int
	Col    int
	Mods   Mod
	SGR    bool
}

func EncodeMouse(event MouseEvent) ([]byte, bool) {
	if !event.SGR || event.Row < 0 || event.Col < 0 {
		return nil, false
	}
	code, ok := mouseButtonCode(event.Button)
	if !ok {
		return nil, false
	}
	if event.Action == MouseRelease {
		code = 3
	} else if event.Action == MouseMove {
		code += 32
	}
	code += mouseModifierCode(event.Mods)

	final := 'M'
	if event.Action == MouseRelease {
		final = 'm'
	}
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", code, event.Col+1, event.Row+1, final)), true
}

func mouseButtonCode(button MouseButton) (int, bool) {
	switch button {
	case MouseLeft:
		return 0, true
	case MouseMiddle:
		return 1, true
	case MouseRight:
		return 2, true
	case MouseWheelUp:
		return 64, true
	case MouseWheelDown:
		return 65, true
	default:
		return 0, false
	}
}

func mouseModifierCode(mods Mod) int {
	code := 0
	if mods&ModShift != 0 {
		code += 4
	}
	if mods&ModAlt != 0 {
		code += 8
	}
	if mods&ModCtrl != 0 {
		code += 16
	}
	return code
}
