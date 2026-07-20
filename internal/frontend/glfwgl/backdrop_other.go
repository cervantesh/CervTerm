//go:build glfw && !windows && !darwin && !linux

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

func newBlurProvider(_ *glfw.Window) BlurProvider {
	return unsupportedBlurProvider{name: "unsupported-platform"}
}
