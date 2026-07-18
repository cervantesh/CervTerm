//go:build glfw

package glfwgl

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/render"
)

const (
	atlasPageSize        = 2048
	atlasPageCount       = 2
	maxAtlasFontContexts = fontdesc.MaxRetainedContexts
)

type atlasKey struct {
	spec atlasFontKey
	face fontdesc.ResolvedFaceKey
	kind byte
	r    rune   // single-rune glyphs (kind 'r'); 0 for clusters/runs
	span int32  // cell span for clusters/runs (kind 'c'/'l'); 0 for single runes
	text string // cluster/run text (kind 'c'/'l'); "" for single runes
}

type atlasEntry struct {
	page       int
	u0, v0     float32
	u1, v1     float32
	colored    bool
	subpixel   bool
	cellSpan   int
	cellW      int
	cellH      int
	generation uint64
}

type atlasPage struct {
	packer shelfPacker
}

type atlasNegativeKey struct {
	reason byte
	key    atlasKey
}

const (
	negativeRun byte = iota + 1
	negativeRaster
	negativeInsertion
)

type atlasNegativeCache struct {
	generation uint64
	entries    map[atlasNegativeKey]struct{}
	ring       []atlasNegativeKey
	next       int
}

func (c *atlasNegativeCache) ensureGeneration(generation uint64) {
	if c.generation == generation && c.entries != nil {
		return
	}
	c.generation = generation
	c.entries = make(map[atlasNegativeKey]struct{})
	c.ring = nil
	c.next = 0
}

func (c *atlasNegativeCache) contains(generation uint64, reason byte, key atlasKey) bool {
	c.ensureGeneration(generation)
	_, ok := c.entries[atlasNegativeKey{reason: reason, key: key}]
	return ok
}

func (c *atlasNegativeCache) record(generation uint64, reason byte, key atlasKey) {
	c.ensureGeneration(generation)
	entry := atlasNegativeKey{reason: reason, key: key}
	if _, exists := c.entries[entry]; exists {
		return
	}
	if len(c.ring) < fontdesc.MaxNegativeEntries {
		c.ring = append(c.ring, entry)
	} else {
		delete(c.entries, c.ring[c.next])
		c.ring[c.next] = entry
		c.next = (c.next + 1) % len(c.ring)
	}
	c.entries[entry] = struct{}{}
}

type glyphAtlas struct {
	cellW, cellH int
	baseline     int
	backend      fontglyph.Backend
	coverageLUT  *[256]uint8
	r            gpu.Renderer
	pages        [atlasPageCount]atlasPage
	entries      map[atlasKey]atlasEntry
	generation   uint64

	contexts       map[atlasFontKey]*atlasFontContext
	activeContext  *atlasFontContext
	pinnedContexts map[atlasFontKey]struct{}
	contextClock   uint64
	backendFactory atlasBackendFactory
	prewarming     bool
	closed         bool
}

func newGlyphAtlas(r gpu.Renderer) (*glyphAtlas, error) {
	defaults := config.Defaults().Render
	return newGlyphAtlasWithSpec(r, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: defaults.TextRaster}, defaults.TextGamma, defaults.TextDarken)
}

func newGlyphAtlasWithSpec(r gpu.Renderer, spec fontglyph.Spec, textGamma, textDarken float64) (*glyphAtlas, error) {
	return newGlyphAtlasWithBackendFactory(r, spec, textGamma, textDarken, newAtlasBackend)
}

func (a *glyphAtlas) Reset() {
	// Reset can fire mid-frame: insertRaster calls it when a glyph won't fit, and
	// glyphs are inserted lazily during drawRow. At that point this frame has
	// already issued immediate-mode quads that sample these atlas textures. The
	// texture reallocation/clear below (and the re-uploads that follow) would then
	// race those still-pending draws — a well-behaved driver serializes the read
	// before the write, but many do not, and the earlier-drawn glyphs (higher rows,
	// i.e. the ones "further back") sample cleared/overwritten texels and vanish.
	// ClearAtlasPage drains the pipeline (glFinish) so every pending draw completes
	// against the old atlas before we touch it. Reset is rare (zoom reconfigure or
	// atlas overflow), so the full stall is not a hot-path cost.
	for i := range a.pages {
		a.pages[i].packer.Reset()
		a.r.ClearAtlasPage(i)
	}
	clear(a.entries)
	a.generation++
	for _, ctx := range a.contexts {
		ctx.prewarmed = false
		ctx.negatives.ensureGeneration(a.generation)
	}
	log.Printf("glyph atlas generation reset: generation=%d", a.generation)
}

func (a *glyphAtlas) resolveStyle(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode) {
	if a == nil {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone
	}
	return a.activeContext.resolveStyle(request)
}

func (a *glyphAtlas) resolveRuneStyle(request fontdesc.RequestedFaceStyle, value rune) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode) {
	if a == nil {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone
	}
	return a.activeContext.resolveRuneStyle(request, value)
}

func (a *glyphAtlas) resolveClusterStyle(request fontdesc.RequestedFaceStyle, cluster string) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode) {
	if a == nil {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone
	}
	return a.activeContext.resolveClusterStyle(request, cluster)
}

func (a *glyphAtlas) drawRune(r rune, x, y float32, fg color.RGBA, scale, skew float32) {
	a.drawRuneStyle(fontdesc.RequestedFaceStyleNormal, r, x, y, fg, scale, skew)
}

func (a *glyphAtlas) drawRuneStyle(request fontdesc.RequestedFaceStyle, r rune, x, y float32, fg color.RGBA, scale, skew float32) {
	entry, ok := a.cachedRuneStyle(request, r)
	if !ok && r != '?' {
		entry, ok = a.cachedRuneStyle(request, '?')
	}
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
}

func (a *glyphAtlas) drawCluster(cluster string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	return a.drawClusterStyle(fontdesc.RequestedFaceStyleNormal, cluster, cellSpan, x, y, fg, scale, skew)
}

func (a *glyphAtlas) drawClusterStyle(request fontdesc.RequestedFaceStyle, cluster string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	entry, ok := a.cachedClusterStyle(request, cluster, cellSpan)
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
	return ok
}

// supportsLigatures reports whether the active context's shaper can substitute
// ligature glyphs. Probed once by the App so no per-frame reflection happens.
func (a *glyphAtlas) supportsLigatures(compatibility bool) bool {
	ctx := a.activeContext
	backend, ok := activeLigatureBackend(ctx)
	if !ok {
		return false
	}
	if ctx.features.IsZero() {
		return compatibility && backend.SupportsLigatures()
	}
	return ctx.features.RequiresRunShaping() && backend.SupportsLigatures()
}

// drawRun draws a shaped ligature spanning cellSpan cells, returning false when
// the run has no ligature so the caller renders it per-cell. Both outcomes are
// cached (positive as an atlas entry, negative in runNegative) so a run is
// shaped at most once per atlas generation.
func (a *glyphAtlas) drawRun(run string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	return a.drawRunStyle(fontdesc.RequestedFaceStyleNormal, run, cellSpan, x, y, fg, scale, skew)
}

func (a *glyphAtlas) drawRunStyle(request fontdesc.RequestedFaceStyle, run string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	entry, ok := a.cachedRunStyle(request, run, cellSpan)
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
	return ok
}

func (a *glyphAtlas) cachedRun(run string, cellSpan int) (atlasEntry, bool) {
	return a.cachedRunStyle(fontdesc.RequestedFaceStyleNormal, run, cellSpan)
}

func (a *glyphAtlas) cachedRunStyle(request fontdesc.RequestedFaceStyle, run string, cellSpan int) (atlasEntry, bool) {
	ctx := a.activeContext
	if run == "" || ctx == nil {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	face, _ := ctx.resolveClusterStyle(request, run)
	key := atlasKey{spec: ctx.key, face: face, kind: 'l', text: run, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if ctx.negatives.contains(a.generation, negativeRun, key) {
		return atlasEntry{}, false
	}
	var rasterized fontglyph.RasterizedGlyph
	var ligated bool
	if backend, ok := activeStyledBackend(ctx); ok {
		rasterized, ligated = backend.RasterizeRunStyle(request, run, cellSpan)
	} else if backend, ok := activeLigatureBackend(ctx); ok {
		rasterized, ligated = backend.RasterizeRun(run, cellSpan)
	} else {
		return atlasEntry{}, false
	}
	if !ligated {
		ctx.negatives.record(a.generation, negativeRun, key)
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) cachedRune(r rune) (atlasEntry, bool) {
	return a.cachedRuneStyle(fontdesc.RequestedFaceStyleNormal, r)
}

func (a *glyphAtlas) cachedRuneStyle(request fontdesc.RequestedFaceStyle, r rune) (atlasEntry, bool) {
	ctx := a.activeContext
	if ctx == nil {
		return atlasEntry{}, false
	}
	face, _ := ctx.resolveRuneStyle(request, r)
	// Key on the rune directly; the old atlasKey{text: string(r)} allocated a
	// string on every glyph lookup — i.e. per visible cell per frame.
	key := atlasKey{spec: ctx.key, face: face, kind: 'r', r: r}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if ctx.negatives.contains(a.generation, negativeRaster, key) {
		return atlasEntry{}, false
	}
	span := max(1, core.RuneWidth(r))
	var rasterized fontglyph.RasterizedGlyph
	var ok bool
	if backend, styled := activeStyledBackend(ctx); styled {
		rasterized, ok = backend.RasterizeStyle(request, r, span)
	} else {
		rasterized, ok = ctx.backend.Rasterize(r, span)
	}
	if !ok {
		ctx.negatives.record(a.generation, negativeRaster, key)
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) cachedCluster(cluster string, cellSpan int) (atlasEntry, bool) {
	return a.cachedClusterStyle(fontdesc.RequestedFaceStyleNormal, cluster, cellSpan)
}

func (a *glyphAtlas) cachedClusterStyle(request fontdesc.RequestedFaceStyle, cluster string, cellSpan int) (atlasEntry, bool) {
	ctx := a.activeContext
	if cluster == "" || ctx == nil {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	face, _ := ctx.resolveClusterStyle(request, cluster)
	key := atlasKey{spec: ctx.key, face: face, kind: 'c', text: cluster, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if ctx.negatives.contains(a.generation, negativeRaster, key) {
		return atlasEntry{}, false
	}
	var rasterized fontglyph.RasterizedGlyph
	var ok bool
	if backend, styled := activeStyledBackend(ctx); styled {
		rasterized, ok = backend.RasterizeClusterStyle(request, cluster, cellSpan)
	} else {
		rasterized, ok = ctx.backend.RasterizeCluster(cluster, cellSpan)
	}
	if !ok {
		ctx.negatives.record(a.generation, negativeRaster, key)
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) currentEntry(key atlasKey) (atlasEntry, bool) {
	entry, ok := a.entries[key]
	return entry, ok && entryGenerationValid(entry.generation, a.generation)
}

func (a *glyphAtlas) insertionFailedThisGeneration(ctx *atlasFontContext, key atlasKey) bool {
	return ctx != nil && ctx.negatives.contains(a.generation, negativeInsertion, key)
}

func (a *glyphAtlas) recordInsertionFailure(ctx *atlasFontContext, key atlasKey) {
	if ctx != nil {
		ctx.negatives.record(a.generation, negativeInsertion, key)
	}
}

func projectRasterToContext(glyph fontglyph.RasterizedGlyph, ctx *atlasFontContext) fontglyph.RasterizedGlyph {
	if glyph.Image == nil || ctx == nil {
		return glyph
	}
	span := max(1, glyph.CellSpan)
	targetW, targetH := ctx.cellW*span, ctx.cellH
	bounds := glyph.Image.Bounds()
	offsetX := (targetW-bounds.Dx())/2 + int(math.Round(ctx.metrics.GlyphOffsetX))
	offsetY := (targetH-bounds.Dy())/2 + int(math.Round(ctx.metrics.BaselineOffset+ctx.metrics.GlyphOffsetY))
	if bounds.Dx() == targetW && bounds.Dy() == targetH && offsetX == 0 && offsetY == 0 {
		return glyph
	}
	projected := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.Draw(projected, image.Rect(offsetX, offsetY, offsetX+bounds.Dx(), offsetY+bounds.Dy()), glyph.Image, bounds.Min, draw.Src)
	glyph.Image = projected
	glyph.Width, glyph.Height = targetW, targetH
	glyph.CellSpan = span
	glyph.AdvanceX = float64(targetW)
	return glyph
}

func (a *glyphAtlas) insertRaster(key atlasKey, glyph fontglyph.RasterizedGlyph, ctx *atlasFontContext) (atlasEntry, bool) {
	if a.insertionFailedThisGeneration(ctx, key) || glyph.Image == nil {
		return atlasEntry{}, false
	}
	sourceW, sourceH := glyph.Image.Bounds().Dx(), glyph.Image.Bounds().Dy()
	if sourceW <= 0 || sourceH <= 0 || sourceW > atlasPageSize || sourceH > atlasPageSize {
		a.recordInsertionFailure(ctx, key)
		return atlasEntry{}, false
	}
	glyph = projectRasterToContext(glyph, ctx)
	w, h := glyph.Image.Bounds().Dx(), glyph.Image.Bounds().Dy()
	if w <= 0 || h <= 0 || w > atlasPageSize || h > atlasPageSize {
		a.recordInsertionFailure(ctx, key)
		return atlasEntry{}, false
	}
	entry, ok := a.tryInsert(key, glyph, ctx)
	if ok {
		return entry, true
	}
	if a.prewarming {
		return atlasEntry{}, false
	}
	a.Reset()
	a.prewarmASCII()
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	entry, ok = a.tryInsert(key, glyph, ctx)
	if !ok {
		a.recordInsertionFailure(ctx, key)
	}
	return entry, ok
}

func (a *glyphAtlas) tryInsert(key atlasKey, glyph fontglyph.RasterizedGlyph, ctx *atlasFontContext) (atlasEntry, bool) {
	w, h := glyph.Image.Bounds().Dx(), glyph.Image.Bounds().Dy()
	for pageIndex := range a.pages {
		x, y, ok := a.pages[pageIndex].packer.Insert(w, h)
		if !ok {
			continue
		}
		pixels := glyph.Image.Pix
		if ctx.coverageLUT != nil && !glyph.HasColor {
			pixels = append([]byte(nil), pixels...)
			render.ApplyCoverageLUT(pixels, ctx.coverageLUT)
		}
		a.r.UploadAtlasRegion(pageIndex, x, y, w, h, pixels)
		entry := atlasEntry{
			page: pageIndex, u0: float32(x) / atlasPageSize, v0: float32(y) / atlasPageSize,
			u1: float32(x+w) / atlasPageSize, v1: float32(y+h) / atlasPageSize,
			colored: glyph.HasColor, subpixel: glyph.Subpixel, cellSpan: glyph.CellSpan,
			cellW: ctx.cellW, cellH: ctx.cellH, generation: a.generation,
		}
		a.entries[key] = entry
		return entry, true
	}
	return atlasEntry{}, false
}

func (a *glyphAtlas) drawEntry(entry atlasEntry, x, y float32, fg color.RGBA, scale, skew float32) {
	// Snap the glyph origin to whole pixels. The bitmap is one texel per pixel,
	// so an integer origin keeps texel-to-pixel 1:1 and the LINEAR filter returns
	// exact texels; a fractional origin (from HiDPI padding/advance) would blur.
	// The fractional part is identical for every cell (cellW/cellH are integers),
	// so rounding preserves uniform spacing.
	x = float32(math.Round(float64(x)))
	y = float32(math.Round(float64(y)))
	w := float32(entry.cellW*max(1, entry.cellSpan)) * scale
	h := float32(entry.cellH) * scale
	// The renderer owns tint/blend/binding; the atlas only selects the glyph mode.
	// The subpixel two-pass, colored white-tint, and skew semantics live in
	// glRenderer.DrawGlyph (bit-identical to the former inline GL emit). Precedence
	// matches the original drawEntry: subpixel wins over colored — the old code set
	// the colored white tint first but the subpixel branch ran (and returned) with
	// its own two-pass colors, so a colored+subpixel glyph took the subpixel path.
	mode := gpu.GlyphMask
	if entry.subpixel {
		mode = gpu.GlyphSubpixel
	} else if entry.colored {
		mode = gpu.GlyphColor
	}
	a.r.DrawGlyph(entry.page, mode, x, y, w, h, skew, entry.u0, entry.v0, entry.u1, entry.v1, fg)
}
