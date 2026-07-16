package input

import "fmt"

type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
	// MouseNone reports motion with no button held (any-event tracking).
	MouseNone
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
	if event.Row < 0 || event.Col < 0 {
		return nil, false
	}
	code, ok := mouseButtonCode(event.Button)
	if !ok && event.Action == MouseMove {
		code = 3
		ok = true
	}
	if !ok {
		return nil, false
	}
	if event.Action == MouseRelease && !event.SGR {
		code = 3
	} else if event.Action == MouseMove {
		code += 32
	}
	code += mouseModifierCode(event.Mods)

	if !event.SGR {
		return []byte{0x1b, '[', 'M', byte(code + 32), byte(clampMouseCoord(event.Col+1) + 32), byte(clampMouseCoord(event.Row+1) + 32)}, true
	}
	final := 'M'
	if event.Action == MouseRelease {
		final = 'm'
	}
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", code, event.Col+1, event.Row+1, final)), true
}

func clampMouseCoord(v int) int {
	if v < 1 {
		return 1
	}
	if v > 222 {
		return 222
	}
	return v
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
