//go:build glfw

package glfwgl

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
)

func applyOpacity(c color.RGBA, opacity float64) color.RGBA {
	if opacity <= 0 {
		c.A = 0
		return c
	}
	if opacity >= 1 {
		return c
	}
	c.A = uint8(math.Round(float64(c.A) * opacity))
	return c
}

// composeSolidPane preserves the pre-surface solid-background pixels at opacity 1:
// the pane color was historically blended once over the configured clear color.
// The new background multiplier is then applied once to that canonical result.
func composeSolidPane(configured, pane color.RGBA, opacity float64) color.RGBA {
	sourceAlpha := float64(pane.A) / 255
	blend := func(source, destination uint8) uint8 {
		return uint8(math.Round(float64(source)*sourceAlpha + float64(destination)*(1-sourceAlpha)))
	}
	result := color.RGBA{
		R: blend(pane.R, configured.R),
		G: blend(pane.G, configured.G),
		B: blend(pane.B, configured.B),
		A: uint8(math.Round((sourceAlpha + float64(configured.A)/255*(1-sourceAlpha)) * 255)),
	}
	return applyOpacity(result, opacity)
}

func effectivePaneBackground(configured, pane color.RGBA, oscOverride bool, opacity float64) color.RGBA {
	if oscOverride {
		return applyOpacity(pane, opacity)
	}
	return composeSolidPane(configured, pane, opacity)
}

func effectiveSolidBackground(cfg config.Config) color.RGBA {
	return applyOpacity(configColor(cfg.Colors.Background, color.RGBA{0x08, 0x0B, 0x12, 0xFF}), cfg.Window.BackgroundOpacity)
}

func prepareSolidBackgroundSurface(renderer gpu.Renderer, cfg config.Config) (gpu.BackgroundSurface, error) {
	capability, ok := renderer.(gpu.BackgroundSurfaceRenderer)
	if !ok {
		return nil, nil
	}
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.SetRGBA(0, 0, effectiveSolidBackground(cfg))
	surface, err := capability.PrepareBackgroundSurface(pixel)
	if err != nil {
		return nil, fmt.Errorf("prepare solid background surface: %w", err)
	}
	return surface, nil
}

func (a *App) prepareInitialBackgroundSurface() error {
	surface, err := prepareSolidBackgroundSurface(a.r, a.cfg)
	if err != nil {
		return err
	}
	a.backgroundSurface = surface
	return nil
}

func (a *App) closeBackgroundSurface() {
	if a.backgroundSurface == nil {
		return
	}
	_ = a.backgroundSurface.Close()
	a.backgroundSurface = nil
}

func (a *App) restoreBackgroundSurface(fallback color.RGBA, width, height int) {
	capability, ok := a.r.(gpu.BackgroundSurfaceRenderer)
	if !ok || a.backgroundSurface == nil {
		a.r.Clear(fallback)
		return
	}
	if err := capability.ReplaceBackgroundRect(a.backgroundSurface, gpu.ClipRect{Width: width, Height: height}); err != nil {
		a.r.Clear(fallback)
	}
}
