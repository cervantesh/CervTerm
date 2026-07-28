//go:build glfw && windows

package glfwgl

import (
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func newInitialWindowReveal(window *glfw.Window) initialWindowReveal {
	return windowsInitialWindowReveal{
		hwnd:         uintptr(unsafe.Pointer(window.GetWin32Window())),
		setAttribute: callInitialWindowDwmSetWindowAttribute,
	}
}
