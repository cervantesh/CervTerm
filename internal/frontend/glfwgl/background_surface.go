//go:build glfw

package glfwgl

import (
	"fmt"
	"image"
	"image/color"
	"math"

	backgroundcore "cervterm/internal/background"
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

func prepareRGBABackgroundSurface(renderer gpu.Renderer, pixels *image.RGBA) (gpu.BackgroundSurface, error) {
	capability, ok := renderer.(gpu.BackgroundSurfaceRenderer)
	if !ok {
		return nil, nil
	}
	surface, err := capability.PrepareBackgroundSurface(pixels)
	if err != nil {
		return nil, fmt.Errorf("prepare background surface: %w", err)
	}
	return surface, nil
}

func solidBackgroundPixels(cfg config.Config) *image.RGBA {
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.SetRGBA(0, 0, effectiveSolidBackground(cfg))
	return pixel
}

func prepareSolidBackgroundSurface(renderer gpu.Renderer, cfg config.Config) (gpu.BackgroundSurface, error) {
	return prepareRGBABackgroundSurface(renderer, solidBackgroundPixels(cfg))
}

func backgroundGPUTransferBytes(activeBytes uint64, surface *image.RGBA) (uint64, error) {
	if surface == nil {
		return 0, fmt.Errorf("background GPU budget: surface is required")
	}
	candidateBytes, err := backgroundcore.SurfaceBytes(surface.Bounds().Dx(), surface.Bounds().Dy())
	if err != nil {
		return 0, err
	}
	if activeBytes > backgroundcore.MaxAggregateGPUBytes || candidateBytes > backgroundcore.MaxAggregateGPUBytes-activeBytes {
		return 0, fmt.Errorf("background GPU aggregate budget: exceeds limit")
	}
	return candidateBytes, nil
}

func (a *App) prepareInitialBackgroundSurface() error {
	if len(a.cfg.Background.Layers) == 0 {
		surface, err := prepareSolidBackgroundSurface(a.r, a.cfg)
		if err != nil {
			return err
		}
		a.backgroundSurface = surface
		if surface != nil {
			a.configReloadAsync.activeGPUBytes = 4
		}
		return nil
	}
	width, height := a.window.GetFramebufferSize()
	result := make(chan struct {
		cpu *preparedBackgroundCPU
		err error
	}, 1)
	baseDir := backgroundLayerBase(a.composedProvenance, a.configPath)
	pool := a.ensureBackgroundResourcePool()
	dpi := effectiveDPI(a.contentScaleX, a.contentScaleY)
	go func() {
		cpu, err := pool.prepare(a.cfg.Clone(), baseDir, width, height, dpi)
		result <- struct {
			cpu *preparedBackgroundCPU
			err error
		}{cpu: cpu, err: err}
	}()
	prepared := <-result
	if prepared.err != nil {
		return prepared.err
	}
	defer prepared.cpu.Close()
	gpuBytes, err := backgroundGPUTransferBytes(0, prepared.cpu.surface)
	if err != nil {
		return err
	}
	surface, err := prepareRGBABackgroundSurface(a.r, prepared.cpu.surface)
	if err != nil {
		return err
	}
	if surface == nil {
		return fmt.Errorf("background layers: renderer capability unavailable")
	}
	a.backgroundSurface = surface
	a.configReloadAsync.activeGPUBytes = gpuBytes
	a.configReloadAsync.activeBackgroundDPI = prepared.cpu.dpi
	a.configReloadAsync.requestedBackgroundDPI = prepared.cpu.dpi
	a.backgroundSurfaceWidth, a.backgroundSurfaceHeight = width, height
	a.backgroundRequestedWidth, a.backgroundRequestedHeight = width, height
	a.registerInitialBackgroundDependencies(prepared.cpu)
	return nil
}

func (a *App) registerInitialBackgroundDependencies(prepared *preparedBackgroundCPU) {
	if prepared == nil || len(prepared.watchPaths) == 0 {
		return
	}
	if a.configWatchHashes == nil {
		a.configWatchHashes = make(map[string][32]byte)
	}
	for path, hash := range prepared.watchHashes {
		a.configWatchHashes[path] = hash
	}
	paths := append(append([]string(nil), a.configWatch.activePaths...), prepared.watchPaths...)
	a.configWatch.acknowledgeSuccess(paths)
}

func (a *App) closeBackgroundSurface() {
	if a.backgroundSurface == nil {
		return
	}
	_ = a.backgroundSurface.Close()
	a.backgroundSurfaceWidth, a.backgroundSurfaceHeight = 0, 0
	a.backgroundSurface = nil
	a.configReloadAsync.activeGPUBytes = 0
	a.configReloadAsync.activeBackgroundDPI = 0
	a.configReloadAsync.requestedBackgroundDPI = 0
}

func (a *App) restoreBackgroundSurface(fallback color.RGBA, width, height int) {
	if len(a.cfg.Background.Layers) > 0 && (a.backgroundSurfaceWidth != width || a.backgroundSurfaceHeight != height) {
		a.r.Clear(fallback)
		return
	}
	capability, ok := a.r.(gpu.BackgroundSurfaceRenderer)
	if !ok || a.backgroundSurface == nil {
		a.r.Clear(fallback)
		return
	}
	if err := capability.ReplaceBackgroundRect(a.backgroundSurface, gpu.ClipRect{Width: width, Height: height}); err != nil {
		a.r.Clear(fallback)
	}
}
