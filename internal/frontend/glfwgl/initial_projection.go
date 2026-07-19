//go:build glfw

package glfwgl

import (
	termmux "cervterm/internal/mux"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) closeUnadoptedProjectionResources() {
	a.closeDividerCursors()
	if a.atlas != nil {
		a.atlas.close()
		a.atlas = nil
	}
	a.closeBackgroundSurface()
	if a.r != nil {
		a.r.Destroy()
		a.r = nil
	}
	if a.blurProvider != nil {
		_ = a.blurProvider.Close()
		a.blurProvider = nil
	}
}

func (a *App) adoptInitialProjection(window *glfw.Window) error {
	bundle := &nativeProjectionBundle{
		host: window, app: a, handle: a.applyMuxEvents,
		resources: []projectionResource{
			projectionResourceFunc(func() error {
				a.closeDividerCursors()
				if a.blurProvider == nil {
					return nil
				}
				err := a.blurProvider.Close()
				a.blurProvider = nil
				return err
			}),
			projectionResourceFunc(func() error {
				if a.r != nil {
					a.r.Destroy()
					a.r = nil
				}
				return nil
			}),
			projectionResourceFunc(func() error { a.closeBackgroundSurface(); return nil }),
			projectionResourceFunc(func() error {
				if a.atlas != nil {
					a.atlas.close()
					a.atlas = nil
				}
				return nil
			}),
		},
	}
	return a.controller.adoptProjectionBundle(termmux.WindowID(initialWindowID), bundle)
}
