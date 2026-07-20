//go:build glfw && windows

package glfwgl

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var messageBeep = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBeep")

func nativeAudibleBell() error {
	result, _, callErr := messageBeep.Call(0xFFFFFFFF)
	if result == 0 {
		return fmt.Errorf("MessageBeep: %w", callErr)
	}
	return nil
}
