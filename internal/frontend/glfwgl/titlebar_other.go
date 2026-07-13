//go:build glfw && !windows

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

// applyDarkTitleBar is a no-op outside Windows; other window managers theme the
// frame themselves.
func applyDarkTitleBar(_ *glfw.Window) {}
