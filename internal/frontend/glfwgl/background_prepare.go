//go:build glfw

package glfwgl

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sort"

	backgroundcore "cervterm/internal/background"
	"cervterm/internal/config"
)

type preparedBackgroundCPU struct {
	surface       *image.RGBA
	watchPaths    []string
	watchHashes   map[string][32]byte
	pool          *backgroundResourcePool
	residentBytes uint64
	dpi           float64
	closed        bool
}

func (p *preparedBackgroundCPU) Close() {
	if p == nil || p.closed {
		return
	}
	p.closed = true
	if p.pool != nil {
		p.pool.releaseComposition(p.residentBytes)
	}
	p.pool = nil
	p.residentBytes = 0
}

func backgroundLayerBase(records []config.ProvenanceRecord, fallback string) string {
	for _, record := range records {
		if record.Path == "background.layers" && record.Winner.CanonicalSource != "" {
			return filepath.Dir(record.Winner.CanonicalSource)
		}
	}
	if fallback != "" {
		return filepath.Dir(fallback)
	}
	return ""
}

func prepareBackgroundCPU(cfg config.Config, baseDir string, width, height int) (*preparedBackgroundCPU, error) {
	return newBackgroundResourcePool().prepare(cfg, baseDir, width, height, 96)
}

func (p *backgroundResourcePool) prepare(cfg config.Config, baseDir string, width, height int, dpi float64) (*preparedBackgroundCPU, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(cfg.Background.Layers) == 0 {
		return nil, nil
	}
	budget := backgroundcore.NewBudget()
	layers := make([]backgroundcore.Layer, 0, len(cfg.Background.Layers))
	leases := make([]*backgroundcore.Lease, 0, backgroundcore.MaxImageLayers)
	watchHashes := make(map[string][32]byte)
	defer func() {
		for _, lease := range leases {
			_ = p.cache.Release(lease)
		}
	}()
	imageIndex := 0
	for index, spec := range cfg.Background.Layers {
		layer := backgroundcore.Layer{Opacity: spec.Opacity}
		switch spec.Kind {
		case "solid":
			layer.Solid = &backgroundcore.Solid{Color: configColor(spec.Color, color.RGBA{})}
		case "linear_gradient":
			stops := make([]backgroundcore.GradientStop, len(spec.Colors))
			for stopIndex, value := range spec.Colors {
				offset := float64(stopIndex) / float64(len(spec.Colors)-1)
				stops[stopIndex] = backgroundcore.GradientStop{Offset: offset, Color: configColor(value, color.RGBA{})}
			}
			layer.LinearGradient = &backgroundcore.LinearGradient{Angle: spec.Angle, Stops: stops}
		case "image":
			imageIndex++
			path := spec.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			variant := backgroundCacheVariant(spec, width, height, dpi)
			lease, source, err := p.resolveLocked(imageIndex, path, variant, budget)
			if err != nil {
				return nil, fmt.Errorf("background layer %d: %w", index+1, err)
			}
			leases = append(leases, lease)
			if !backgroundSourceDigestMatches(imageIndex, source) {
				return nil, fmt.Errorf("background layer %d: image changed while decoding", index+1)
			}
			watchHash, hashErr := config.FileSourceWatchHash(source.CanonicalPath())
			if hashErr != nil {
				return nil, fmt.Errorf("background layer %d: image watch digest failed", index+1)
			}
			if !backgroundSourceDigestMatches(imageIndex, source) {
				return nil, fmt.Errorf("background layer %d: image changed while decoding", index+1)
			}
			watchHashes[source.CanonicalPath()] = watchHash
			layer.Image = &backgroundcore.Image{Source: source, Fit: backgroundcore.Fit(spec.Fit), Horizontal: backgroundcore.HorizontalAlignment(spec.HorizontalAlign), Vertical: backgroundcore.VerticalAlignment(spec.VerticalAlign)}
		}
		layers = append(layers, layer)
	}
	base := configColor(cfg.Colors.Background, color.RGBA{R: 0x08, G: 0x0B, B: 0x12, A: 0xFF})
	outputBytes, err := backgroundcore.SurfaceBytes(width, height)
	if err != nil {
		return nil, err
	}
	if err := p.reserveCompositionLocked(outputBytes); err != nil {
		return nil, err
	}
	compositionOwned := true
	defer func() {
		if compositionOwned {
			p.composedBytes -= outputBytes
		}
	}()
	surface, err := backgroundcore.Compose(width, height, base, layers, budget)
	if err != nil {
		return nil, err
	}
	if cfg.Window.BackgroundOpacity != 1 {
		for offset := 3; offset < len(surface.Pix); offset += 4 {
			surface.Pix[offset] = applyOpacity(color.RGBA{A: surface.Pix[offset]}, cfg.Window.BackgroundOpacity).A
		}
	}
	paths := make([]string, 0, len(watchHashes))
	for path := range watchHashes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	compositionOwned = false
	return &preparedBackgroundCPU{surface: surface, watchPaths: paths, watchHashes: watchHashes, pool: p, residentBytes: outputBytes, dpi: dpi}, nil
}

func backgroundSourceDigestMatches(imageIndex int, source *backgroundcore.Source) bool {
	if source == nil {
		return false
	}
	_, digest, err := backgroundcore.FileDigest(imageIndex, source.CanonicalPath())
	return err == nil && digest == source.Digest()
}

type backgroundResizeResult struct {
	generation    uint64
	width, height int
	dpi           float64
	cpu           *preparedBackgroundCPU
	err           error
}

func (a *App) requestBackgroundResize(width, height int) {
	if len(a.cfg.Background.Layers) == 0 || width <= 0 || height <= 0 {
		return
	}
	dpi := effectiveDPI(a.contentScaleX, a.contentScaleY)
	if a.backgroundSurfaceWidth == width && a.backgroundSurfaceHeight == height && a.configReloadAsync.activeBackgroundDPI == dpi {
		return
	}
	if a.backgroundRequestedWidth == width && a.backgroundRequestedHeight == height && a.configReloadAsync.requestedBackgroundDPI == dpi {
		return
	}
	if a.backgroundResizeResults == nil {
		a.backgroundResizeResults = make(chan backgroundResizeResult, 8)
	}
	a.backgroundGeneration++
	generation := a.backgroundGeneration
	a.backgroundRequestedWidth, a.backgroundRequestedHeight = width, height
	a.configReloadAsync.requestedBackgroundDPI = dpi
	cfg := a.cfg.Clone()
	baseDir := backgroundLayerBase(a.composedProvenance, a.configPath)
	results := a.backgroundResizeResults
	pool := a.ensureBackgroundResourcePool()
	a.configReloadAsync.resizeWorkers++
	go func() {
		cpu, err := pool.prepare(cfg, baseDir, width, height, dpi)
		results <- backgroundResizeResult{generation: generation, width: width, height: height, dpi: dpi, cpu: cpu, err: err}
	}()
}

func (a *App) applyPreparedBackgroundResize() {
	for a.backgroundResizeResults != nil {
		select {
		case result := <-a.backgroundResizeResults:
			if a.configReloadAsync.resizeWorkers > 0 {
				a.configReloadAsync.resizeWorkers--
			}
			if result.generation != a.backgroundGeneration {
				if result.cpu != nil {
					result.cpu.Close()
				}
				continue
			}
			if result.err != nil {
				a.backgroundRequestedWidth, a.backgroundRequestedHeight = 0, 0
				a.Notify("background resize failed: " + result.err.Error())
				if result.cpu != nil {
					result.cpu.Close()
				}
				continue
			}
			gpuBytes, err := backgroundGPUTransferBytes(a.configReloadAsync.activeGPUBytes, result.cpu.surface)
			if err != nil {
				a.backgroundRequestedWidth, a.backgroundRequestedHeight = 0, 0
				a.Notify("background resize failed: " + err.Error())
				result.cpu.Close()
				continue
			}
			surface, err := prepareRGBABackgroundSurface(a.r, result.cpu.surface)
			if err != nil {
				a.backgroundRequestedWidth, a.backgroundRequestedHeight = 0, 0
				a.Notify("background resize failed: " + err.Error())
				result.cpu.Close()
				continue
			}
			if surface == nil {
				a.backgroundRequestedWidth, a.backgroundRequestedHeight = 0, 0
				a.Notify("background resize failed: renderer capability unavailable")
				result.cpu.Close()
				continue
			}
			result.cpu.Close()
			old := a.backgroundSurface
			a.backgroundSurface = surface
			a.backgroundSurfaceWidth, a.backgroundSurfaceHeight = result.width, result.height
			a.configReloadAsync.activeGPUBytes = gpuBytes
			a.configReloadAsync.activeBackgroundDPI = result.dpi
			if old != nil {
				_ = old.Close()
			}
			a.damage.valid = false
			a.requestRedraw()
		default:
			return
		}
	}
}
