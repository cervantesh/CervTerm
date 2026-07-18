//go:build glfw && windows

package glfwgl

import (
	"syscall"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// applyConfiguredTitlebar applies the requested mode and reports whether it was supported.
func applyConfiguredTitlebar(w *glfw.Window, mode string) bool {
	if mode == "system" {
		return true
	}
	hwnd := uintptr(unsafe.Pointer(w.GetWin32Window()))
	if hwnd == 0 {
		return false
	}
	var enabled int32 = 1
	// Attribute 20 (DWMWA_USE_IMMERSIVE_DARK_MODE) on Windows 10 2004+; older
	// 20H1 builds used 19. Try the current value first, then fall back.
	for _, attr := range []uintptr{20, 19} {
		ret, _, _ := procDwmSetWindowAttribute.Call(hwnd, attr,
			uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled))
		if ret == 0 { // S_OK
			return true
		}
	}
	return false
}
