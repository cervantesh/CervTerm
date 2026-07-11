//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func specFromGLFW(key glfw.Key, mods glfw.ModifierKey) (script.Spec, bool) {
	name, ok := keyNameFromGLFW(key)
	if !ok {
		return script.Spec{}, false
	}
	var scriptMods script.Mod
	if mods&glfw.ModControl != 0 {
		scriptMods |= script.ModCtrl
	}
	if mods&glfw.ModAlt != 0 {
		scriptMods |= script.ModAlt
	}
	if mods&glfw.ModShift != 0 {
		scriptMods |= script.ModShift
	}
	if mods&glfw.ModSuper != 0 {
		scriptMods |= script.ModSuper
	}
	return script.Spec{Key: name, Mods: scriptMods}, true
}

func keyNameFromGLFW(key glfw.Key) (string, bool) {
	if key >= glfw.KeyA && key <= glfw.KeyZ {
		return string(rune('a' + key - glfw.KeyA)), true
	}
	if key >= glfw.Key0 && key <= glfw.Key9 {
		return string(rune('0' + key - glfw.Key0)), true
	}
	if key >= glfw.KeyF1 && key <= glfw.KeyF12 {
		return fmt.Sprintf("f%d", int(key-glfw.KeyF1)+1), true
	}
	switch key {
	case glfw.KeyEnter:
		return "enter", true
	case glfw.KeyTab:
		return "tab", true
	case glfw.KeyEscape:
		return "escape", true
	case glfw.KeySpace:
		return "space", true
	case glfw.KeyBackspace:
		return "backspace", true
	case glfw.KeyDelete:
		return "delete", true
	case glfw.KeyInsert:
		return "insert", true
	case glfw.KeyHome:
		return "home", true
	case glfw.KeyEnd:
		return "end", true
	case glfw.KeyPageUp:
		return "pageup", true
	case glfw.KeyPageDown:
		return "pagedown", true
	case glfw.KeyUp:
		return "up", true
	case glfw.KeyDown:
		return "down", true
	case glfw.KeyLeft:
		return "left", true
	case glfw.KeyRight:
		return "right", true
	case glfw.KeyMinus:
		return "minus", true
	case glfw.KeyEqual:
		return "equal", true
	case glfw.KeyComma:
		return "comma", true
	case glfw.KeyPeriod:
		return "period", true
	case glfw.KeySlash:
		return "slash", true
	case glfw.KeyBackslash:
		return "backslash", true
	case glfw.KeySemicolon:
		return "semicolon", true
	case glfw.KeyApostrophe:
		return "apostrophe", true
	case glfw.KeyGraveAccent:
		return "grave", true
	default:
		return "", false
	}
}
