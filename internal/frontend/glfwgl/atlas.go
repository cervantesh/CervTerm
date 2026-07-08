//go:build glfw

package glfwgl

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/go-gl/gl/v2.1/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type fontSpec struct {
	Family string
	Size   float64
	DPI    float64
}

type glyphAtlas struct {
	tex       uint32
	cellW     int
	cellH     int
	cols      int
	rows      int
	firstRune rune
}

func newGlyphAtlas() (*glyphAtlas, error) {
	return newGlyphAtlasWithSpec(fontSpec{Family: "Go Mono", Size: 14, DPI: 96})
}

func newGlyphAtlasWithSpec(spec fontSpec) (*glyphAtlas, error) {
	face, metrics, err := newOpenTypeFace(spec)
	if err != nil {
		return nil, err
	}
	defer face.Close()

	const first, last = rune(32), rune(126)
	const cols = 16
	rows := int(math.Ceil(float64(last-first+1) / float64(cols)))
	cellW := max(1, font.MeasureString(face, "W").Ceil()+2)
	cellH := max(1, (metrics.Ascent+metrics.Descent).Ceil()+2)
	baseline := 1 + metrics.Ascent.Ceil()

	img := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.RGBA{255, 255, 255, 255}), Face: face}
	for r := first; r <= last; r++ {
		i := int(r - first)
		x := (i % cols) * cellW
		y := (i / cols) * cellH
		d.Dot = fixed.P(x+1, y+baseline)
		d.DrawString(string(r))
	}

	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(img.Bounds().Dx()), int32(img.Bounds().Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))

	return &glyphAtlas{tex: tex, cellW: cellW, cellH: cellH, cols: cols, rows: rows, firstRune: first}, nil
}

func newOpenTypeFace(spec fontSpec) (font.Face, font.Metrics, error) {
	parsed, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, font.Metrics{}, err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: spec.Size, DPI: spec.DPI, Hinting: font.HintingFull})
	if err != nil {
		return nil, font.Metrics{}, err
	}
	return face, face.Metrics(), nil
}

func (a *glyphAtlas) drawRune(r rune, x, y, scale float32) {
	if r < 32 || r > 126 {
		r = '?'
	}
	i := int(r - a.firstRune)
	col := i % a.cols
	row := i / a.cols
	tw := float32(a.cols * a.cellW)
	th := float32(a.rows * a.cellH)
	u0 := float32(col*a.cellW) / tw
	v0 := float32(row*a.cellH) / th
	u1 := float32((col+1)*a.cellW) / tw
	v1 := float32((row+1)*a.cellH) / th
	w := float32(a.cellW) * scale
	h := float32(a.cellH) * scale

	gl.BindTexture(gl.TEXTURE_2D, a.tex)
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(u0, v0)
	gl.Vertex2f(x, y)
	gl.TexCoord2f(u1, v0)
	gl.Vertex2f(x+w, y)
	gl.TexCoord2f(u1, v1)
	gl.Vertex2f(x+w, y+h)
	gl.TexCoord2f(u0, v1)
	gl.Vertex2f(x, y+h)
	gl.End()
}
