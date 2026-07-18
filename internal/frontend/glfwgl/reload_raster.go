//go:build glfw

package glfwgl

import (
	"time"

	"cervterm/internal/fontglyph"
)

func (a *App) prepareRasterContexts(textRaster string) (map[atlasFontKey]*atlasFontContext, error) {
	prepared := make(map[atlasFontKey]*atlasFontContext)
	sizes := []float64{a.cfg.Font.Size}
	if a.mux != nil && len(a.mux.PaneIDs()) > 0 {
		sizes = sizes[:0]
		for _, id := range a.mux.PaneIDs() {
			size := a.cfg.Font.Size
			if state := a.paneUI[id]; state != nil && state.font.fontSize > 0 {
				size = state.font.fontSize
			}
			sizes = append(sizes, size)
		}
	}
	for _, size := range sizes {
		spec := fontglyph.Spec{Family: a.cfg.Font.Family, Size: size, DPI: effectiveDPI(a.contentScaleX, a.contentScaleY), TextRaster: textRaster}
		model := a.atlas.modelForSpec(spec)
		key, err := makeAtlasFontKeyWithModel(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, model)
		if err != nil {
			closePreparedRasterContexts(prepared)
			return nil, err
		}
		if _, ok := a.atlas.contexts[key]; ok {
			continue
		}
		if _, ok := prepared[key]; ok {
			continue
		}
		ctx, err := makeAtlasFontContextWithModel(spec, a.cfg.Render.TextGamma, a.cfg.Render.TextDarken, model, a.atlas.backendFactory)
		if err != nil {
			closePreparedRasterContexts(prepared)
			return nil, err
		}
		prepared[key] = ctx
	}
	return prepared, nil
}

func closePreparedRasterContexts(prepared map[atlasFontKey]*atlasFontContext) {
	closed := make([]fontglyph.Backend, 0, len(prepared))
	for _, ctx := range prepared {
		duplicate := false
		for _, backend := range closed {
			if sameAtlasBackend(ctx.backend, backend) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			ctx.backend.Close()
			closed = append(closed, ctx.backend)
		}
	}
}

func (a *App) activateInstalledRasterContexts() {
	if a.mux == nil || len(a.mux.PaneIDs()) == 0 {
		cellW, cellH, _, ok := a.atlas.useSpec(a.fontSpec(a.cfg.Font.Size, a.contentScaleX, a.contentScaleY), a.cfg.Render.TextGamma, a.cfg.Render.TextDarken)
		if ok {
			a.cellW, a.cellH = float32(cellW), float32(cellH)
			a.ligaturesActive = a.atlas.supportsLigatures(a.cfg.Font.Ligatures)
		}
		return
	}
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		gridChanged, applied := a.applyPaneFontVisual(id, state.font.fontSize, a.contentScaleX, a.contentScaleY)
		state.font.ptyDirty = state.font.ptyDirty || (applied && gridChanged)
	}
	now := time.Now()
	for _, id := range a.mux.PaneIDs() {
		state := a.ensurePaneUI(id)
		if !state.font.ptyDirty {
			continue
		}
		if a.applyPanePTYResize(id) {
			state.font.ptyDirty = false
			continue
		}
		a.schedulePanePTYResizeRetry(id, now)
	}
	a.restoreFocusedFontProjection()
}
