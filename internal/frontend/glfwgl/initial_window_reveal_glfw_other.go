//go:build glfw && !windows

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

// GLFW does not expose a reliable runtime opacity-support query across every
// non-Windows window system. Keep the ordered hidden/visible double-present
// fallback rather than risking a platform error or altered framebuffer alpha.
func newInitialWindowReveal(*glfw.Window) initialWindowReveal {
	return unsupportedInitialWindowReveal{}
}
