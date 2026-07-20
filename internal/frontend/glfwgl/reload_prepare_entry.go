//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	"cervterm/internal/frontend/gpu"
)

type preparedLiveConfig struct {
	next                  config.Config
	projectionBase        *config.Config
	preparedContexts      map[atlasFontKey]*atlasFontContext
	contextInstall        *atlasPreparedContextInstall
	backgroundSurface     gpu.BackgroundSurface
	backgroundWidth       int
	backgroundHeight      int
	backgroundBytes       uint64
	backgroundDPI         float64
	backgroundWatchPaths  []string
	backgroundWatchHashes map[string][32]byte
	rasterChanged         bool
	backgroundChanged     bool
	committed             bool
}

func (p *preparedLiveConfig) Close() {
	if p == nil || p.committed {
		return
	}
	closePreparedRasterContexts(p.preparedContexts)
	p.preparedContexts = nil
	if p.backgroundSurface != nil {
		_ = p.backgroundSurface.Close()
		p.backgroundSurface = nil
	}
}

func (a *App) prepareLiveConfig(next config.Config) (*preparedLiveConfig, error) {
	return a.prepareLiveConfigWithProvenance(next, a.composedProvenance)
}
