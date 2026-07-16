//go:build glfw

package glfwgl

import (
	"cervterm/internal/input"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) inputMode() input.Mode {
	_, view, ok := a.focusedView()
	if !ok {
		return input.Mode{}
	}
	return input.Mode{ApplicationCursor: view.ApplicationCursor}
}

func inputEventFromGLFW(key glfw.Key, mods glfw.ModifierKey) (input.Event, bool) {
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

	if terminalMods&input.ModCtrl != 0 && key >= glfw.KeyA && key <= glfw.KeyZ {
		return input.Event{Rune: rune('a' + key - glfw.KeyA), Mods: terminalMods}, true
	}

	switch key {
	case glfw.KeyEnter:
		return input.Event{Key: input.KeyEnter, Mods: terminalMods}, true
	case glfw.KeyBackspace:
		return input.Event{Key: input.KeyBackspace, Mods: terminalMods}, true
	case glfw.KeyTab:
		return input.Event{Key: input.KeyTab, Mods: terminalMods}, true
	case glfw.KeyEscape:
		return input.Event{Key: input.KeyEscape, Mods: terminalMods}, true
	case glfw.KeyUp:
		return input.Event{Key: input.KeyUp, Mods: terminalMods}, true
	case glfw.KeyDown:
		return input.Event{Key: input.KeyDown, Mods: terminalMods}, true
	case glfw.KeyRight:
		return input.Event{Key: input.KeyRight, Mods: terminalMods}, true
	case glfw.KeyLeft:
		return input.Event{Key: input.KeyLeft, Mods: terminalMods}, true
	case glfw.KeyHome:
		return input.Event{Key: input.KeyHome, Mods: terminalMods}, true
	case glfw.KeyEnd:
		return input.Event{Key: input.KeyEnd, Mods: terminalMods}, true
	case glfw.KeyPageUp:
		return input.Event{Key: input.KeyPageUp, Mods: terminalMods}, true
	case glfw.KeyPageDown:
		return input.Event{Key: input.KeyPageDown, Mods: terminalMods}, true
	case glfw.KeyInsert:
		return input.Event{Key: input.KeyInsert, Mods: terminalMods}, true
	case glfw.KeyDelete:
		return input.Event{Key: input.KeyDelete, Mods: terminalMods}, true
	case glfw.KeyF1:
		return input.Event{Key: input.KeyF1, Mods: terminalMods}, true
	case glfw.KeyF2:
		return input.Event{Key: input.KeyF2, Mods: terminalMods}, true
	case glfw.KeyF3:
		return input.Event{Key: input.KeyF3, Mods: terminalMods}, true
	case glfw.KeyF4:
		return input.Event{Key: input.KeyF4, Mods: terminalMods}, true
	case glfw.KeyF5:
		return input.Event{Key: input.KeyF5, Mods: terminalMods}, true
	case glfw.KeyF6:
		return input.Event{Key: input.KeyF6, Mods: terminalMods}, true
	case glfw.KeyF7:
		return input.Event{Key: input.KeyF7, Mods: terminalMods}, true
	case glfw.KeyF8:
		return input.Event{Key: input.KeyF8, Mods: terminalMods}, true
	case glfw.KeyF9:
		return input.Event{Key: input.KeyF9, Mods: terminalMods}, true
	case glfw.KeyF10:
		return input.Event{Key: input.KeyF10, Mods: terminalMods}, true
	case glfw.KeyF11:
		return input.Event{Key: input.KeyF11, Mods: terminalMods}, true
	case glfw.KeyF12:
		return input.Event{Key: input.KeyF12, Mods: terminalMods}, true
	default:
		return input.Event{}, false
	}
}
