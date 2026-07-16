//go:build glfw && windows

package glfwgl

import "github.com/go-gl/glfw/v3.3/glfw"

func newBlurProvider(w *glfw.Window) BlurProvider {
	return &nativeBlurProvider{
		name: "windows-dwm-system-backdrop",
		set: func(enabled bool) error {
			return applyBackdrop(w, enabled)
		},
	}
}
