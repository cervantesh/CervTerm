//go:build glfw

package glfwgl

import (
	"errors"
	"time"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

var errInitialPresentationSkipped = errors.New("initial window frame was not presented")

func (a *App) createHiddenInitialWindow() (*glfw.Window, error) {
	return createWindowHidden(
		func(initialDefaults bool) {
			value := glfw.False
			if initialDefaults {
				value = glfw.True
			}
			glfw.WindowHint(glfw.Visible, value)
			glfw.WindowHint(glfw.FocusOnShow, value)
		},
		func() (*glfw.Window, error) {
			return glfw.CreateWindow(a.cfg.Window.Width, a.cfg.Window.Height, "CervTerm", nil, nil)
		},
	)
}

func (a *App) runFreshInitialWindow() error {
	var prepared *preparedFontInstallation
	projectionAdopted := false
	defer a.closeInitialWindowController()
	defer func() {
		if !projectionAdopted {
			a.closeUnadoptedProjectionResources()
		}
	}()
	defer func() {
		if prepared != nil {
			prepared.Close()
		}
	}()

	window, err := runInitialWindowLifecycle(
		a.createHiddenInitialWindow,
		func(w *glfw.Window) error {
			a.window = w
			if err := a.attachInitialWindowController(w); err != nil {
				return err
			}
			a.transparentFramebuffer = w.GetAttrib(glfw.TransparentFramebuffer) == glfw.True
			a.blurProvider = newBlurProvider(w)
			a.configureNativeWindow(w)
			a.applyWindowAppearance()
			if a.host == nil {
				return errWindowProjectionMissing
			}
			if err := a.host.activate(initialWindowID); err != nil {
				return err
			}
			swapInterval := 1
			if !a.cfg.Render.VSync {
				swapInterval = 0
			}
			glfw.SwapInterval(swapInterval)
			if err := gl.Init(); err != nil {
				return err
			}
			a.r = newGLRenderer(w)
			if err := a.prepareInitialBackgroundSurface(); err != nil {
				return err
			}
			a.lastFBW, a.lastFBH = -1, -1
			sx, sy := w.GetContentScale()
			a.applyScale(sx, sy)
			stages := defaultFontInstallationStages()
			plan, err := newStartupFontInstallationPlan(a.cfg, effectiveDPI(sx, sy), a.effectiveTextRaster(), a.safeFonts)
			if err != nil {
				return err
			}
			prepared, err = prepareFontInstallation(plan, stages)
			if err != nil {
				return err
			}
			atlas, err := prepared.adopt(a.r, stages)
			if err != nil {
				return err
			}
			a.atlas = atlas
			a.ligaturesActive = atlas.supportsLigatures(a.cfg.Font.Ligatures)
			a.cellW = float32(atlas.cellW)
			a.cellH = float32(atlas.cellH)
			if err := a.applyInitialGridWindowPlan(w, sx, sy); err != nil {
				return err
			}
			if err := a.activateInitialTerminalImages(a.commitStartupConfiguration); err != nil {
				return err
			}
			a.syncProcessServices()
			a.installCallbacks()
			a.spawnInitialPTY(w)
			if err := a.adoptInitialProjection(w); err != nil {
				return err
			}
			projectionAdopted = true
			a.needsRedraw = true
			return nil
		},
		func(*glfw.Window) error {
			return a.presentInitialFrame(false)
		},
		func(*glfw.Window) error {
			// The visible presentation is a deliberate second frame even in
			// on-demand mode. It uses the normal throttling, swap, accounting, and
			// damage acknowledgement paths rather than creating a startup-only frame.
			a.requestRedraw()
			return a.presentInitialFrame(true)
		},
		func(w *glfw.Window) initialWindowReveal { return newInitialWindowReveal(w) },
		newInitialWindowCompositor(),
	)
	if err != nil {
		return err
	}
	return a.runLoop(window)
}

func (a *App) presentInitialFrame(continuous bool) error {
	if !a.ensureRenderController().renderProjection(continuous) {
		return errInitialPresentationSkipped
	}
	// SwapBuffers only submits the frame. Complete the GPU work before either
	// arming visibility or asking the desktop compositor to consume it.
	gl.Finish()
	a.acknowledgePresentedFrame(initialWindowID, a, time.Now())
	return nil
}
