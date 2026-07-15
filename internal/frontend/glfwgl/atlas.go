//go:build glfw

package glfwgl

import (
	"image/color"
	"log"
	"math"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/render"
)

const (
	atlasPageSize  = 2048
	atlasPageCount = 2
)

type atlasKey struct {
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
	generation uint64
}

type atlasPage struct {
	packer shelfPacker
}

type glyphAtlas struct {
	cellW, cellH int
	baseline     int
	backend      fontglyph.Backend
	r            gpu.Renderer
	pages        [atlasPageCount]atlasPage
	entries      map[atlasKey]atlasEntry
	runNegative  map[atlasKey]uint64 // run key -> generation of a proven no-ligature result
	generation   uint64
	coverageLUT  *[256]uint8
}

func newGlyphAtlas(r gpu.Renderer) (*glyphAtlas, error) {
	defaults := config.Defaults().Render
	return newGlyphAtlasWithSpec(r, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: defaults.TextRaster}, defaults.TextGamma, defaults.TextDarken)
}

func newGlyphAtlasWithSpec(r gpu.Renderer, spec fontglyph.Spec, textGamma, textDarken float64) (*glyphAtlas, error) {
	backend, err := fontglyph.NewOpenTypeBackend(spec)
	if err != nil {
		return nil, err
	}
	cellW, cellH, baseline := backend.CellMetrics()
	a := &glyphAtlas{
		cellW: cellW, cellH: cellH, baseline: baseline,
		backend: backend, r: r, entries: make(map[atlasKey]atlasEntry), generation: 1,
	}
	if textGamma != 1 || textDarken != 0 {
		lut := render.CoverageLUT(textGamma, textDarken)
		a.coverageLUT = &lut
	}
	for i := range a.pages {
		a.pages[i].packer = newShelfPacker(atlasPageSize, atlasPageSize)
	}
	// The atlas owns the page geometry (atlasPageCount/atlasPageSize), so it
	// configures the renderer's atlas textures here rather than the app.
	r.ConfigureAtlas(atlasPageCount, atlasPageSize)
	a.prewarmASCII()
	return a, nil
}

func (a *glyphAtlas) prewarmASCII() {
	for r := rune(32); r <= 126; r++ {
		_, _ = a.cachedRune(r)
	}
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
	a.generation++
	log.Printf("glyph atlas generation reset: generation=%d", a.generation)
}

// reconfigure re-points the atlas at a new font spec (size/DPI/family) while
// reusing the existing GL textures instead of allocating a fresh
// atlasPageCount×atlasPageSize² pair per zoom step. The glyph cache is cleared
// and ASCII re-prewarmed at the new size. Returns false (leaving the atlas
// unchanged) if the new backend could not be built.
func (a *glyphAtlas) reconfigure(spec fontglyph.Spec, textGamma, textDarken float64) bool {
	backend, err := fontglyph.NewOpenTypeBackend(spec)
	if err != nil {
		return false
	}
	if a.backend != nil {
		a.backend.Close()
	}
	a.backend = backend
	a.cellW, a.cellH, a.baseline = backend.CellMetrics()
	if textGamma != 1 || textDarken != 0 {
		lut := render.CoverageLUT(textGamma, textDarken)
		a.coverageLUT = &lut
	} else {
		a.coverageLUT = nil
	}
	a.Reset() // clears the packer + textures + entries in place (keeps the textures)
	a.prewarmASCII()
	return true
}

func (a *glyphAtlas) close() {
	if a.backend != nil {
		a.backend.Close()
	}
	// The renderer owns the atlas textures now; Destroy releases them.
	a.r.Destroy()
	a.entries = nil
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

// supportsLigatures reports whether the backend's active shaper can substitute
// ligature glyphs. Probed once by the App so no per-frame reflection happens.
func (a *glyphAtlas) supportsLigatures() bool {
	otb, ok := a.backend.(*fontglyph.OpenTypeBackend)
	return ok && otb.SupportsLigatures()
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
	if run == "" {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	key := atlasKey{kind: 'l', text: run, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if gen, ok := a.runNegative[key]; ok && entryGenerationValid(gen, a.generation) {
		return atlasEntry{}, false
	}
	otb, ok := a.backend.(*fontglyph.OpenTypeBackend)
	if !ok {
		return atlasEntry{}, false
	}
	rasterized, ligated := otb.RasterizeRun(run, cellSpan)
	if !ligated {
		if a.runNegative == nil {
			a.runNegative = make(map[atlasKey]uint64)
		}
		a.runNegative[key] = a.generation
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized)
}

func (a *glyphAtlas) cachedRune(r rune) (atlasEntry, bool) {
	// Key on the rune directly; the old atlasKey{text: string(r)} allocated a
	// string on every glyph lookup — i.e. per visible cell per frame.
	key := atlasKey{kind: 'r', r: r}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	span := max(1, core.RuneWidth(r))
	rasterized, ok := a.backend.Rasterize(r, span)
	if !ok {
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized)
}

func (a *glyphAtlas) cachedCluster(cluster string, cellSpan int) (atlasEntry, bool) {
	if cluster == "" {
		return atlasEntry{}, false
	}
	cellSpan = max(1, cellSpan)
	key := atlasKey{kind: 'c', text: cluster, span: int32(cellSpan)}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	rasterized, ok := a.backend.RasterizeCluster(cluster, cellSpan)
	if !ok {
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized)
}

func (a *glyphAtlas) currentEntry(key atlasKey) (atlasEntry, bool) {
	entry, ok := a.entries[key]
	return entry, ok && entryGenerationValid(entry.generation, a.generation)
}

func (a *glyphAtlas) insertRaster(key atlasKey, glyph fontglyph.RasterizedGlyph) (atlasEntry, bool) {
	entry, ok := a.tryInsert(key, glyph)
	if ok {
		return entry, true
	}
	a.Reset()
	a.prewarmASCII()
	return a.tryInsert(key, glyph)
}

func (a *glyphAtlas) tryInsert(key atlasKey, glyph fontglyph.RasterizedGlyph) (atlasEntry, bool) {
	w, h := glyph.Image.Bounds().Dx(), glyph.Image.Bounds().Dy()
	for pageIndex := range a.pages {
		x, y, ok := a.pages[pageIndex].packer.Insert(w, h)
		if !ok {
			continue
		}
		if a.coverageLUT != nil && !glyph.HasColor {
			render.ApplyCoverageLUT(glyph.Image.Pix, a.coverageLUT)
		}
		a.r.UploadAtlasRegion(pageIndex, x, y, w, h, glyph.Image.Pix)
		entry := atlasEntry{
			page: pageIndex, u0: float32(x) / atlasPageSize, v0: float32(y) / atlasPageSize,
			u1: float32(x+w) / atlasPageSize, v1: float32(y+h) / atlasPageSize,
			colored: glyph.HasColor, subpixel: glyph.Subpixel, cellSpan: glyph.CellSpan, generation: a.generation,
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
	w := float32(a.cellW*max(1, entry.cellSpan)) * scale
	h := float32(a.cellH) * scale
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
