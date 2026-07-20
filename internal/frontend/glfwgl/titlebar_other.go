//go:build glfw && !windows

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

// applyConfiguredTitlebar degrades dark to the platform-managed system titlebar.
func applyConfiguredTitlebar(_ *glfw.Window, mode string) bool { return mode == "system" }
