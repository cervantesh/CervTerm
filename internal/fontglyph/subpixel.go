package fontglyph

import (
	"image"
	"image/color"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

var subpixelFIRKernel = [...]int{1, 2, 3, 2, 1}

// applySubpixelFIR applies the normalized five-tap LCD filter at sample index.
// Samples beyond either edge repeat the nearest edge sample.
func applySubpixelFIR(samples []uint8, index int) uint8 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0
	for tap, weight := range subpixelFIRKernel {
		i := index + tap - len(subpixelFIRKernel)/2
		i = max(0, min(len(samples)-1, i))
		sum += int(samples[i]) * weight
	}
	return uint8((sum + 4) / 9)
}

// rasterizeSubpixel renders an outline at three horizontal samples per pixel,
// then filters those samples into independent RGB coverage channels.
func (b *OpenTypeBackend) rasterizeSubpixel(lf loadedFace, glyphID sfnt.GlyphIndex, bounds fixed.Rectangle26_6, cellSpan int) (*image.RGBA, bool) {
	if lf.sfnt == nil {
		return nil, false
	}
	var buf sfnt.Buffer
	segments, err := lf.sfnt.LoadGlyph(&buf, glyphID, fixed.I(int(b.ppem)), nil)
	if err != nil {
		return nil, false
	}
	w := b.cellW * max(1, cellSpan)
	hiW := 3 * w
	mask := image.NewAlpha(image.Rect(0, 0, hiW, b.cellH))
	rasterizeSubpixelSegments(mask, segments, bounds, b.baseline)
	out := image.NewRGBA(image.Rect(0, 0, w, b.cellH))
	row := make([]uint8, hiW)
	for y := 0; y < b.cellH; y++ {
		for x := range hiW {
			row[x] = mask.AlphaAt(x, y).A
		}
		for x := 0; x < w; x++ {
			r := applySubpixelFIR(row, 3*x)
			g := applySubpixelFIR(row, 3*x+1)
			bl := applySubpixelFIR(row, 3*x+2)
			a := uint8((int(r) + int(g) + int(bl) + 1) / 3)
			out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: bl, A: a})
		}
	}
	return out, true
}

func rasterizeSubpixelSegments(dst *image.Alpha, segments sfnt.Segments, bounds fixed.Rectangle26_6, baseline int) {
	r := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	for _, segment := range segments {
		xy := func(i int) (float32, float32) {
			p := segment.Args[i]
			return 3 * (1 + float32(p.X-bounds.Min.X)/64), float32(baseline) + float32(p.Y)/64
		}
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			x, y := xy(0)
			r.MoveTo(x, y)
		case sfnt.SegmentOpLineTo:
			x, y := xy(0)
			r.LineTo(x, y)
		case sfnt.SegmentOpQuadTo:
			bx, by := xy(0)
			cx, cy := xy(1)
			r.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := xy(0)
			cx, cy := xy(1)
			dx, dy := xy(2)
			r.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	r.Draw(dst, dst.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
}
