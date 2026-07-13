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

// applyDarkTitleBar asks DWM to render the native window title bar in dark mode
// so it matches the terminal's dark theme instead of the light default. It is a
// best-effort call: unsupported Windows builds simply return an error we ignore.
func applyDarkTitleBar(w *glfw.Window) {
	hwnd := uintptr(unsafe.Pointer(w.GetWin32Window()))
	if hwnd == 0 {
		return
	}
	var enabled int32 = 1
	// Attribute 20 (DWMWA_USE_IMMERSIVE_DARK_MODE) on Windows 10 2004+; older
	// 20H1 builds used 19. Try the current value first, then fall back.
	for _, attr := range []uintptr{20, 19} {
		ret, _, _ := procDwmSetWindowAttribute.Call(hwnd, attr,
			uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled))
		if ret == 0 { // S_OK
			return
		}
	}
}
