//go:build windows

package glfwgl

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const initialWindowDwmwaCloak uint32 = 13

type initialWindowSetAttributeCall func(hwnd uintptr, attribute uint32, value *int32, valueSize uint32) (uintptr, error)

type windowsInitialWindowReveal struct {
	hwnd         uintptr
	setAttribute initialWindowSetAttributeCall
}

var initialWindowDwmSetWindowAttributeProc = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

func callInitialWindowDwmSetWindowAttribute(hwnd uintptr, attribute uint32, value *int32, valueSize uint32) (uintptr, error) {
	if err := initialWindowDwmSetWindowAttributeProc.Find(); err != nil {
		return 0, err
	}
	hresult, _, _ := initialWindowDwmSetWindowAttributeProc.Call(
		hwnd,
		uintptr(attribute),
		uintptr(unsafe.Pointer(value)),
		uintptr(valueSize),
	)
	return hresult, nil
}

func (r windowsInitialWindowReveal) Conceal() (bool, error) {
	return true, r.setCloaked(true)
}

func (r windowsInitialWindowReveal) Restore() error {
	return r.setCloaked(false)
}

func (r windowsInitialWindowReveal) setCloaked(cloaked bool) error {
	if r.hwnd == 0 {
		return fmt.Errorf("set initial window DWMWA_CLOAK=%t: missing HWND", cloaked)
	}
	value := int32(0)
	if cloaked {
		value = 1
	}
	hresult, err := r.setAttribute(r.hwnd, initialWindowDwmwaCloak, &value, uint32(unsafe.Sizeof(value)))
	if err != nil {
		return fmt.Errorf("set initial window DWMWA_CLOAK=%t: %w", cloaked, err)
	}
	if hresult != 0 {
		return fmt.Errorf("set initial window DWMWA_CLOAK=%t: DwmSetWindowAttribute HRESULT 0x%08X", cloaked, uint32(hresult))
	}
	return nil
}
