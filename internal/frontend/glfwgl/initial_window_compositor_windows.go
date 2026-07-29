//go:build windows

package glfwgl

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var initialWindowDwmFlushProc = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmFlush")

type windowsInitialWindowCompositor struct {
	flush func() (uintptr, error)
}

func newInitialWindowCompositor() initialWindowCompositor {
	return windowsInitialWindowCompositor{flush: callInitialWindowDwmFlush}
}

func callInitialWindowDwmFlush() (uintptr, error) {
	if err := initialWindowDwmFlushProc.Find(); err != nil {
		return 0, err
	}
	hresult, _, _ := initialWindowDwmFlushProc.Call()
	return hresult, nil
}

func (c windowsInitialWindowCompositor) Flush() error {
	hresult, err := c.flush()
	if err != nil {
		return fmt.Errorf("wait for initial desktop composition: %w", err)
	}
	if hresult != 0 {
		return fmt.Errorf("wait for initial desktop composition: DwmFlush HRESULT 0x%08X", uint32(hresult))
	}
	return nil
}
