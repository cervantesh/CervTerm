//go:build glfw

package glfwgl

import (
	"image"
	"image/draw"
	"math"

	"cervterm/internal/core"
	"cervterm/internal/fontglyph"

	"github.com/go-gl/gl/v2.1/gl"
)

type glyphAtlas struct {
	tex       uint32
	cellW     int
	cellH     int
	cols      int
	rows      int
	baseline  int
	firstRune rune
	backend   fontglyph.Backend
	glyphs    map[rune]glyphTexture
	clusters  map[string]glyphTexture
}

type glyphTexture struct {
	tex      uint32
	colored  bool
	cellSpan int
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

	const first, last = rune(32), rune(126)
	const cols = 16
	rows := int(math.Ceil(float64(last-first+1) / float64(cols)))
	img := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
	for r := first; r <= last; r++ {
		glyph, ok := backend.Rasterize(r, 1)
		if !ok {
			continue
		}
		i := int(r - first)
		x := (i % cols) * cellW
		y := (i / cols) * cellH
		draw.Draw(img, image.Rect(x, y, x+cellW, y+cellH), glyph.Image, glyph.Image.Bounds().Min, draw.Src)
	}

	tex := uploadTexture(img)
	return &glyphAtlas{
		tex:       tex,
		cellW:     cellW,
		cellH:     cellH,
		cols:      cols,
		rows:      rows,
		baseline:  baseline,
		firstRune: first,
		backend:   backend,
		glyphs:    make(map[rune]glyphTexture),
		clusters:  make(map[string]glyphTexture),
	}, nil
}

func (a *glyphAtlas) drawRune(r rune, x, y float32, scale, skew float32) {
	if r >= 32 && r <= 126 {
		a.drawASCII(r, x, y, scale, skew)
		return
	}
	glyph, ok := a.cachedGlyph(r)
	if !ok {
		a.drawASCII('?', x, y, scale, skew)
		return
	}
	a.drawTexture(glyph.tex, 0, 0, 1, 1, x, y, scale, skew, glyph.colored, glyph.cellSpan)
}

func (a *glyphAtlas) drawCluster(cluster string, cellSpan int, x, y float32, scale, skew float32) bool {
	glyph, ok := a.cachedCluster(cluster, cellSpan)
	if !ok {
		return false
	}
	a.drawTexture(glyph.tex, 0, 0, 1, 1, x, y, scale, skew, glyph.colored, glyph.cellSpan)
	return true
}

func (a *glyphAtlas) drawASCII(r rune, x, y, scale, skew float32) {
	i := int(r - a.firstRune)
	col := i % a.cols
	row := i / a.cols
	tw := float32(a.cols * a.cellW)
	th := float32(a.rows * a.cellH)
	u0 := float32(col*a.cellW) / tw
	v0 := float32(row*a.cellH) / th
	u1 := float32((col+1)*a.cellW) / tw
	v1 := float32((row+1)*a.cellH) / th
	a.drawTexture(a.tex, u0, v0, u1, v1, x, y, scale, skew, false, 1)
}

func (a *glyphAtlas) cachedGlyph(r rune) (glyphTexture, bool) {
	if glyph, ok := a.glyphs[r]; ok {
		return glyph, true
	}
	span := max(1, core.RuneWidth(r))
	rasterized, ok := a.backend.Rasterize(r, span)
	if !ok {
		return glyphTexture{}, false
	}
	glyph := glyphTexture{tex: uploadTexture(rasterized.Image), colored: rasterized.HasColor, cellSpan: rasterized.CellSpan}
	a.glyphs[r] = glyph
	return glyph, true
}

func (a *glyphAtlas) cachedCluster(cluster string, cellSpan int) (glyphTexture, bool) {
	if cluster == "" {
		return glyphTexture{}, false
	}
	cellSpan = max(1, cellSpan)
	key := cluster + "\x00" + string(rune(cellSpan))
	if glyph, ok := a.clusters[key]; ok {
		return glyph, true
	}
	rasterized, ok := a.backend.RasterizeCluster(cluster, cellSpan)
	if !ok {
		return glyphTexture{}, false
	}
	glyph := glyphTexture{tex: uploadTexture(rasterized.Image), colored: rasterized.HasColor, cellSpan: rasterized.CellSpan}
	a.clusters[key] = glyph
	return glyph, true
}

func uploadTexture(img *image.RGBA) uint32 {
	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(img.Bounds().Dx()), int32(img.Bounds().Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
	return tex
}

func (a *glyphAtlas) drawTexture(tex uint32, u0, v0, u1, v1 float32, x, y, scale, skew float32, colored bool, cellSpan int) {
	w := float32(a.cellW*max(1, cellSpan)) * scale
	h := float32(a.cellH) * scale
	if colored {
		gl.Color4ub(255, 255, 255, 255)
	}
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(u0, v0)
	gl.Vertex2f(x+skew, y)
	gl.TexCoord2f(u1, v0)
	gl.Vertex2f(x+w+skew, y)
	gl.TexCoord2f(u1, v1)
	gl.Vertex2f(x+w, y+h)
	gl.TexCoord2f(u0, v1)
	gl.Vertex2f(x, y+h)
	gl.End()
}
