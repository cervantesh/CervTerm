//go:build glfw

package glfwgl

import (
	"log"

	"cervterm/internal/core"
	"cervterm/internal/fontglyph"

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
	generation   uint64
	boundTexture uint32
}

func newGlyphAtlas() (*glyphAtlas, error) {
	return newGlyphAtlasWithSpec(fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96})
}

func newGlyphAtlasWithSpec(spec fontglyph.Spec) (*glyphAtlas, error) {
	backend, err := fontglyph.NewOpenTypeBackend(spec)
	if err != nil {
		return nil, err
	}
	cellW, cellH, baseline := backend.CellMetrics()
	a := &glyphAtlas{
		cellW: cellW, cellH: cellH, baseline: baseline,
		backend: backend, entries: make(map[atlasKey]atlasEntry), generation: 1,
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
	a.generation++
	a.boundTexture = 0
	log.Printf("glyph atlas generation reset: generation=%d", a.generation)
}

func (a *glyphAtlas) close() {
	for i := range a.pages {
		if a.pages[i].tex != 0 {
			gl.DeleteTextures(1, &a.pages[i].tex)
			a.pages[i].tex = 0
		}
	}
	a.entries = nil
}

func (a *glyphAtlas) drawRune(r rune, x, y float32, scale, skew float32) {
	entry, ok := a.cachedRune(r)
	if !ok && r != '?' {
		entry, ok = a.cachedRune('?')
	}
	if ok {
		a.drawEntry(entry, x, y, scale, skew)
	}
}

func (a *glyphAtlas) drawCluster(cluster string, cellSpan int, x, y float32, scale, skew float32) bool {
	entry, ok := a.cachedCluster(cluster, cellSpan)
	if ok {
		a.drawEntry(entry, x, y, scale, skew)
	}
	return ok
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
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(glyph.Image.Pix))
		a.boundTexture = a.pages[pageIndex].tex
		entry := atlasEntry{
			page: pageIndex, u0: float32(x) / atlasPageSize, v0: float32(y) / atlasPageSize,
			u1: float32(x+w) / atlasPageSize, v1: float32(y+h) / atlasPageSize,
			colored: glyph.HasColor, cellSpan: glyph.CellSpan, generation: a.generation,
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

func (a *glyphAtlas) drawEntry(entry atlasEntry, x, y, scale, skew float32) {
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
