//go:build glfw

package glfwgl

import (
	"image/color"
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
	atlasPageSize  = 2048
	atlasPageCount = 2
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

type glyphAtlas struct {
	cellW, cellH   int
	baseline       int
	backend        fontglyph.Backend
	coverageLUT    *[256]uint8
	r              gpu.Renderer
	pages          [atlasPageCount]atlasPage
	entries        map[atlasKey]atlasEntry
	runNegative    map[atlasKey]uint64 // run key -> generation of a proven no-ligature result
	insertNegative map[atlasKey]uint64 // key -> generation where a capacity retry already failed
	generation     uint64

	contexts       map[atlasFontKey]*atlasFontContext
	activeContext  *atlasFontContext
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
	clear(a.runNegative)
	clear(a.insertNegative)
	for _, ctx := range a.contexts {
		ctx.prewarmed = false
	}
	a.generation++
	log.Printf("glyph atlas generation reset: generation=%d", a.generation)
}

func (a *glyphAtlas) drawRune(r rune, x, y float32, fg color.RGBA, scale, skew float32) {
	entry, ok := a.cachedRune(r)
	if !ok && r != '?' {
		entry, ok = a.cachedRune('?')
	}
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
}

func (a *glyphAtlas) drawCluster(cluster string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	entry, ok := a.cachedCluster(cluster, cellSpan)
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
	return ok
}

// supportsLigatures reports whether the active context's shaper can substitute
// ligature glyphs. Probed once by the App so no per-frame reflection happens.
func (a *glyphAtlas) supportsLigatures() bool {
	ctx := a.activeContext
	backend, ok := activeLigatureBackend(ctx)
	return ok && backend.SupportsLigatures()
}

// drawRun draws a shaped ligature spanning cellSpan cells, returning false when
// the run has no ligature so the caller renders it per-cell. Both outcomes are
// cached (positive as an atlas entry, negative in runNegative) so a run is
// shaped at most once per atlas generation.
func (a *glyphAtlas) drawRun(run string, cellSpan int, x, y float32, fg color.RGBA, scale, skew float32) bool {
	entry, ok := a.cachedRun(run, cellSpan)
	if ok {
		a.drawEntry(entry, x, y, fg, scale, skew)
	}
	return ok
}

func (a *glyphAtlas) cachedRun(run string, cellSpan int) (atlasEntry, bool) {
	ctx := a.activeContext
	if run == "" || ctx == nil {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	key := atlasKey{spec: ctx.key, face: ctx.resolvedFace, kind: 'l', text: run, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if gen, ok := a.runNegative[key]; ok && entryGenerationValid(gen, a.generation) {
		return atlasEntry{}, false
	}
	backend, ok := activeLigatureBackend(ctx)
	if !ok {
		return atlasEntry{}, false
	}
	rasterized, ligated := backend.RasterizeRun(run, cellSpan)
	if !ligated {
		if a.runNegative == nil {
			a.runNegative = make(map[atlasKey]uint64)
		}
		a.runNegative[key] = a.generation
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) cachedRune(r rune) (atlasEntry, bool) {
	ctx := a.activeContext
	if ctx == nil {
		return atlasEntry{}, false
	}
	// Key on the rune directly; the old atlasKey{text: string(r)} allocated a
	// string on every glyph lookup — i.e. per visible cell per frame.
	key := atlasKey{spec: ctx.key, face: ctx.resolvedFace, kind: 'r', r: r}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	span := max(1, core.RuneWidth(r))
	rasterized, ok := ctx.backend.Rasterize(r, span)
	if !ok {
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) cachedCluster(cluster string, cellSpan int) (atlasEntry, bool) {
	ctx := a.activeContext
	if cluster == "" || ctx == nil {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	key := atlasKey{spec: ctx.key, face: ctx.resolvedFace, kind: 'c', text: cluster, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	rasterized, ok := ctx.backend.RasterizeCluster(cluster, cellSpan)
	if !ok {
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized, ctx)
}

func (a *glyphAtlas) currentEntry(key atlasKey) (atlasEntry, bool) {
	entry, ok := a.entries[key]
	return entry, ok && entryGenerationValid(entry.generation, a.generation)
}

func (a *glyphAtlas) insertionFailedThisGeneration(key atlasKey) bool {
	gen, ok := a.insertNegative[key]
	return ok && entryGenerationValid(gen, a.generation)
}

func (a *glyphAtlas) recordInsertionFailure(key atlasKey) {
	if a.insertNegative == nil {
		a.insertNegative = make(map[atlasKey]uint64)
	}
	a.insertNegative[key] = a.generation
}

func (a *glyphAtlas) insertRaster(key atlasKey, glyph fontglyph.RasterizedGlyph, ctx *atlasFontContext) (atlasEntry, bool) {
	if a.insertionFailedThisGeneration(key) || glyph.Image == nil {
		return atlasEntry{}, false
	}
	w, h := glyph.Image.Bounds().Dx(), glyph.Image.Bounds().Dy()
	if w <= 0 || h <= 0 || w > atlasPageSize || h > atlasPageSize {
		a.recordInsertionFailure(key)
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
		a.recordInsertionFailure(key)
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
