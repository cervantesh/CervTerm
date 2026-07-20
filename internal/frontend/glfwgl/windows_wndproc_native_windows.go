//go:build glfw && windows

package glfwgl

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
	"golang.org/x/sys/windows"
)

const gwlpWndProc = -4

var (
	errWndProcNativeCall  = errors.New("WndProc native call failed")
	user32WndProcDLL      = windows.NewLazySystemDLL("user32.dll")
	kernel32WndProcDLL    = windows.NewLazySystemDLL("kernel32.dll")
	getWindowLongPtrWProc = user32WndProcDLL.NewProc(windowLongProcName("GetWindowLongPtrW", "GetWindowLongW"))
	setWindowLongPtrWProc = user32WndProcDLL.NewProc(windowLongProcName("SetWindowLongPtrW", "SetWindowLongW"))
	callWindowProcWProc   = user32WndProcDLL.NewProc("CallWindowProcW")
	setLastErrorProc      = kernel32WndProcDLL.NewProc("SetLastError")
	getLastErrorProc      = kernel32WndProcDLL.NewProc("GetLastError")

	wndProcSetLastErrorCall = func(value uintptr) { _, _, _ = setLastErrorProc.Call(value) }
	wndProcGetLastErrorCall = func() uintptr { value, _, _ := getLastErrorProc.Call(); return value }
	wndProcGetCall          = func(hwnd uintptr) uintptr {
		value, _, _ := getWindowLongPtrWProc.Call(hwnd, ^uintptr(3))
		return value
	}
	wndProcSetCall = func(hwnd, procedure uintptr) uintptr {
		value, _, _ := setWindowLongPtrWProc.Call(hwnd, ^uintptr(3), procedure)
		return value
	}
	wndProcChainCall = func(procedure, hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		value, _, _ := callWindowProcWProc.Call(procedure, hwnd, uintptr(message), wParam, lParam)
		return value
	}
	wndProcNewCallbackCall = func(callback wndProcCallback) uintptr { return syscall.NewCallback(callback) }
	wndProcRegistryMu      sync.RWMutex
	wndProcRegistry        = make(map[uintptr]wndProcCallback)
	wndProcSharedCallback  uintptr
)

type nativeWndProcBackend struct{}

func (nativeWndProcBackend) GetWindowProc(hwnd uintptr) (value uintptr, err error) {
	defer recoverWndProcNative(&err)
	wndProcSetLastErrorCall(0)
	value = wndProcGetCall(hwnd)
	lastError := wndProcGetLastErrorCall()
	if value == 0 && lastError != 0 {
		return 0, fmt.Errorf("%w: %d", errWndProcNativeCall, lastError)
	}
	return value, nil
}

func (nativeWndProcBackend) SetWindowProc(hwnd, procedure uintptr) (value uintptr, err error) {
	defer recoverWndProcNative(&err)
	wndProcSetLastErrorCall(0)
	value = wndProcSetCall(hwnd, procedure)
	lastError := wndProcGetLastErrorCall()
	if value == 0 && lastError != 0 {
		return 0, fmt.Errorf("%w: %d", errWndProcNativeCall, lastError)
	}
	return value, nil
}

func (nativeWndProcBackend) CallWindowProc(procedure, hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	return wndProcChainCall(procedure, hwnd, message, wParam, lParam)
}

func (nativeWndProcBackend) NewCallback(hwnd uintptr, callback wndProcCallback) (pointer uintptr, err error) {
	defer recoverWndProcNative(&err)
	if hwnd == 0 || callback == nil {
		return 0, errWndProcHostInvalid
	}
	wndProcRegistryMu.Lock()
	defer wndProcRegistryMu.Unlock()
	if _, exists := wndProcRegistry[hwnd]; exists {
		return 0, errWndProcHostInvalid
	}
	if wndProcSharedCallback == 0 {
		wndProcSharedCallback = wndProcNewCallbackCall(sharedWndProcDispatch)
		if wndProcSharedCallback == 0 {
			return 0, errWndProcNativeCall
		}
	}
	wndProcRegistry[hwnd] = callback
	return wndProcSharedCallback, nil
}

func (nativeWndProcBackend) ReleaseCallback(hwnd, callback uintptr) (err error) {
	defer recoverWndProcNative(&err)
	wndProcRegistryMu.Lock()
	defer wndProcRegistryMu.Unlock()
	if hwnd == 0 || callback == 0 || callback != wndProcSharedCallback {
		return errWndProcHostInvalid
	}
	if _, exists := wndProcRegistry[hwnd]; !exists {
		return errWndProcHostInvalid
	}
	delete(wndProcRegistry, hwnd)
	return nil
}

func sharedWndProcDispatch(hwnd uintptr, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	wndProcRegistryMu.RLock()
	callback := wndProcRegistry[hwnd]
	wndProcRegistryMu.RUnlock()
	if callback == nil {
		return 0
	}
	return callback(hwnd, message, wParam, lParam)
}

func windowLongProcName(pointerName, legacyName string) string {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return legacyName
	}
	return pointerName
}

func glfwWindowHWND(window *glfw.Window) (uintptr, error) {
	if window == nil {
		return 0, errWndProcHostInvalid
	}
	hwnd := uintptr(unsafe.Pointer(window.GetWin32Window()))
	if hwnd == 0 {
		return 0, errWndProcHostInvalid
	}
	return hwnd, nil
}

func recoverWndProcNative(err *error) {
	if recovered := recover(); recovered != nil {
		*err = errWndProcCallbackPanic
	}
}

var _ wndProcBackend = nativeWndProcBackend{}
