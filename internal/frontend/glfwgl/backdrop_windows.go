//go:build glfw && windows

package glfwgl

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	dwmwaSystemBackdropType = 38
	dwmsbtNone              = 1
	dwmsbtTransientWindow   = 3 // acrylic-like backdrop on supported Windows 11
)

func applyBackdrop(w *glfw.Window, enabled bool) error {
	hwnd := uintptr(unsafe.Pointer(w.GetWin32Window()))
	if hwnd == 0 {
		return fmt.Errorf("%w: missing HWND", errBlurUnsupported)
	}
	backdrop := int32(dwmsbtNone)
	if enabled {
		backdrop = dwmsbtTransientWindow
	}
	ret, _, _ := procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(dwmwaSystemBackdropType),
		uintptr(unsafe.Pointer(&backdrop)),
		unsafe.Sizeof(backdrop),
	)
	if ret != 0 {
		return fmt.Errorf("%w (DwmSetWindowAttribute HRESULT %#x)", errBlurUnsupported, ret)
	}
	return nil
}
