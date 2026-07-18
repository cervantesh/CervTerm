//go:build glfw

package glfwgl

import (
	"log"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) applyNativeWindowCreationHints() {
	// Request transparency up front so live alpha changes need no recreation.
	glfw.WindowHint(glfw.TransparentFramebuffer, glfw.True)
	if a.cfg.Window.Decorations == "none" {
		glfw.WindowHint(glfw.Decorated, glfw.False)
	} else {
		glfw.WindowHint(glfw.Decorated, glfw.True)
	}
}

func (a *App) configureNativeWindow(w *glfw.Window) {
	if icons := windowIcons(); len(icons) > 0 {
		w.SetIcon(icons)
	}
	if !applyConfiguredTitlebar(w, a.cfg.Window.Titlebar) {
		log.Printf("window.titlebar=%q is unsupported on this platform; using system titlebar", a.cfg.Window.Titlebar)
	}
}
