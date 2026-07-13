//go:build glfw

package glfwgl

import (
	"image/color"
	"log"
	"math"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/fontglyph"
	"cervterm/internal/render"

	"github.com/go-gl/gl/v2.1/gl"
)

const (
	atlasPageSize  = 2048
	atlasPageCount = 2
)

type atlasKey struct {
	kind byte
	text string
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
	tex    uint32
	packer shelfPacker
}

type glyphAtlas struct {
	cellW, cellH int
	baseline     int
	backend      fontglyph.Backend
	pages        [atlasPageCount]atlasPage
	entries      map[atlasKey]atlasEntry
	runNegative  map[string]uint64 // run key -> generation of a proven no-ligature result
	generation   uint64
	boundTexture uint32
	coverageLUT  *[256]uint8
}

func newGlyphAtlas() (*glyphAtlas, error) {
	defaults := config.Defaults().Render
	return newGlyphAtlasWithSpec(fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: defaults.TextRaster}, defaults.TextGamma, defaults.TextDarken)
}

func newGlyphAtlasWithSpec(spec fontglyph.Spec, textGamma, textDarken float64) (*glyphAtlas, error) {
	backend, err := fontglyph.NewOpenTypeBackend(spec)
	if err != nil {
		return nil, err
	}
	cellW, cellH, baseline := backend.CellMetrics()
	a := &glyphAtlas{
		cellW: cellW, cellH: cellH, baseline: baseline,
		backend: backend, entries: make(map[atlasKey]atlasEntry), generation: 1,
	}
	if textGamma != 1 || textDarken != 0 {
		lut := render.CoverageLUT(textGamma, textDarken)
		a.coverageLUT = &lut
	}
	for i := range a.pages {
		a.pages[i].packer = newShelfPacker(atlasPageSize, atlasPageSize)
		a.pages[i].tex = createAtlasTexture()
	}
	a.prewarmASCII()
	return a, nil
}

func (a *glyphAtlas) prewarmASCII() {
	for r := rune(32); r <= 126; r++ {
		_, _ = a.cachedRune(r)
	}
}

func (a *glyphAtlas) Reset() {
	for i := range a.pages {
		a.pages[i].packer.Reset()
		clearAtlasTexture(a.pages[i].tex)
	}
	clear(a.entries)
	clear(a.runNegative)
	a.generation++
	a.boundTexture = 0
	log.Printf("glyph atlas generation reset: generation=%d", a.generation)
}

func (a *glyphAtlas) close() {
	if a.backend != nil {
		a.backend.Close()
	}
	for i := range a.pages {
		if a.pages[i].tex != 0 {
			gl.DeleteTextures(1, &a.pages[i].tex)
			a.pages[i].tex = 0
		}
	}
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
	keyText := run + "\x00" + string(rune(cellSpan))
	key := atlasKey{kind: 'l', text: keyText}
	if entry, ok := a.currentEntry(key); ok {
		return entry, true
	}
	if gen, ok := a.runNegative[keyText]; ok && entryGenerationValid(gen, a.generation) {
		return atlasEntry{}, false
	}
	otb, ok := a.backend.(*fontglyph.OpenTypeBackend)
	if !ok {
		return atlasEntry{}, false
	}
	rasterized, ligated := otb.RasterizeRun(run, cellSpan)
	if !ligated {
		if a.runNegative == nil {
			a.runNegative = make(map[string]uint64)
		}
		a.runNegative[keyText] = a.generation
		return atlasEntry{}, false
	}
	return a.insertRaster(key, rasterized)
}

func (a *glyphAtlas) cachedRune(r rune) (atlasEntry, bool) {
	key := atlasKey{kind: 'r', text: string(r)}
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
	key := atlasKey{kind: 'c', text: cluster + "\x00" + string(rune(cellSpan))}
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
		gl.BindTexture(gl.TEXTURE_2D, a.pages[pageIndex].tex)
		if a.coverageLUT != nil && !glyph.HasColor {
			render.ApplyCoverageLUT(glyph.Image.Pix, a.coverageLUT)
		}
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(glyph.Image.Pix))
		a.boundTexture = a.pages[pageIndex].tex
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

func createAtlasTexture() uint32 {
	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, atlasPageSize, atlasPageSize, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	return tex
}

func clearAtlasTexture(tex uint32) {
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, atlasPageSize, atlasPageSize, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
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
	if entry.colored {
		gl.Color4ub(255, 255, 255, 255)
	}
	tex := a.pages[entry.page].tex
	if a.boundTexture != tex {
		gl.BindTexture(gl.TEXTURE_2D, tex)
		a.boundTexture = tex
	}
	if entry.subpixel {
		gl.BlendFunc(gl.ZERO, gl.ONE_MINUS_SRC_COLOR)
		gl.Color4ub(255, 255, 255, 255)
		a.drawQuad(entry, x, y, w, h, skew)
		gl.BlendFunc(gl.ONE, gl.ONE)
		glColor(fg)
		a.drawQuad(entry, x, y, w, h, skew)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		return
	}
	a.drawQuad(entry, x, y, w, h, skew)
}

func (a *glyphAtlas) drawQuad(entry atlasEntry, x, y, w, h, skew float32) {
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(entry.u0, entry.v0)
	gl.Vertex2f(x+skew, y)
	gl.TexCoord2f(entry.u1, entry.v0)
	gl.Vertex2f(x+w+skew, y)
	gl.TexCoord2f(entry.u1, entry.v1)
	gl.Vertex2f(x+w, y+h)
	gl.TexCoord2f(entry.u0, entry.v1)
	gl.Vertex2f(x, y+h)
	gl.End()
}
