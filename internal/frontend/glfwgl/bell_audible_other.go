//go:build glfw && !windows

package glfwgl

import "errors"

func nativeAudibleBell() error {
	return errors.New("native audible bell is not available on this platform")
}
