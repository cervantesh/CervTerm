//go:build !windows

package glfwgl

func newInitialWindowCompositor() initialWindowCompositor {
	return noopInitialWindowCompositor{}
}
