package fontglyph

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type ShapedGlyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

type Shaper interface {
	Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)
}

func (b *OpenTypeBackend) SetShaper(shaper Shaper) {
	b.shaper = shaper
}

func (b *OpenTypeBackend) rasterizeShapedCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	if glyph, ok := b.rasterizeShapedBitmapColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	if glyph, ok := b.rasterizeShapedColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	img := image.NewRGBA(image.Rect(0, 0, b.cellW*cellSpan, b.cellH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	var buf sfnt.Buffer
	pen := 0.0
	maxAdvance := 0.0
	ppem := fixed.I(int(b.ppem))
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			continue
		}
		segments, err := lf.sfnt.LoadGlyph(&buf, sfnt.GlyphIndex(glyph.GlyphID), ppem, nil)
		if err != nil {
			return RasterizedGlyph{}, false
		}
		drawSegments(img, segments, color.RGBA{255, 255, 255, 255}, 1+int(math.Round(pen+glyph.XOffset)), b.baseline+int(math.Round(glyph.YOffset)), identityCOLRTransform())
		pen += glyph.XAdvance
		if pen > maxAdvance {
			maxAdvance = pen
		}
	}
	if !hasVisibleRGBA(img) {
		return RasterizedGlyph{}, false
	}
	if maxAdvance <= 0 {
		maxAdvance = float64(b.cellW * cellSpan)
	}
	return RasterizedGlyph{
		Image:    img,
		Width:    max(1, int(math.Ceil(maxAdvance))),
		Height:   b.cellH,
		BearingX: 1,
		BearingY: b.baseline,
		AdvanceX: maxAdvance,
		CellSpan: cellSpan,
		HasColor: false,
	}, true
}

func (b *OpenTypeBackend) rasterizeShapedBitmapColorCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if len(shaped) != 1 || shaped[0].GlyphID == 0 {
		return RasterizedGlyph{}, false
	}
	bitmap, ok := bitmapColorGlyph(lf, shaped[0].GlyphID, b.ppem)
	if !ok {
		return RasterizedGlyph{}, false
	}
	cellSpan = max(1, cellSpan)
	canvasW := b.cellW * cellSpan
	canvasH := b.cellH
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)

	srcBounds := bitmap.Image.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 {
		return RasterizedGlyph{}, false
	}
	scale := math.Min(float64(canvasW)/float64(srcBounds.Dx()), float64(canvasH)/float64(srcBounds.Dy()))
	if scale <= 0 {
		return RasterizedGlyph{}, false
	}
	dstW := max(1, int(math.Round(float64(srcBounds.Dx())*scale)))
	dstH := max(1, int(math.Round(float64(srcBounds.Dy())*scale)))
	dstX := (canvasW - dstW) / 2
	dstY := (canvasH - dstH) / 2
	dst := image.Rect(dstX, dstY, dstX+dstW, dstY+dstH)
	xdraw.CatmullRom.Scale(img, dst, bitmap.Image, srcBounds, xdraw.Over, nil)

	advance := shaped[0].XAdvance
	if advance <= 0 {
		advance = float64(canvasW)
	}
	return RasterizedGlyph{
		Image:    img,
		Width:    dstW,
		Height:   dstH,
		BearingX: dstX,
		BearingY: canvasH - dstY,
		AdvanceX: advance,
		CellSpan: cellSpan,
		HasColor: true,
	}, true
}

func hasVisibleRGBA(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.RGBAAt(x, y).A != 0 {
				return true
			}
		}
	}
	return false
}
