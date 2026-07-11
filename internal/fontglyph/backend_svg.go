package fontglyph

import (
	"image"
	"image/draw"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func (b *OpenTypeBackend) rasterizeSVGColorGlyph(lf loadedFace, r rune, cellSpan int, advance fixed.Int26_6) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || lf.svg == nil {
		return RasterizedGlyph{}, false
	}
	var buf sfnt.Buffer
	glyphID, err := lf.sfnt.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return RasterizedGlyph{}, false
	}
	doc, ok := lf.svg.document(uint16(glyphID))
	if !ok {
		return RasterizedGlyph{}, false
	}
	canvasW := b.cellW * max(1, cellSpan)
	canvasH := b.cellH
	svgImg, ok := rasterizeSVGDocument(doc, canvasW, canvasH)
	if !ok {
		return RasterizedGlyph{}, false
	}
	img := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	draw.Draw(img, img.Bounds(), svgImg, image.Point{}, draw.Over)
	return RasterizedGlyph{
		Image:    img,
		Width:    canvasW,
		Height:   canvasH,
		BearingX: 0,
		BearingY: canvasH,
		AdvanceX: float64(advance) / 64.0,
		CellSpan: max(1, cellSpan),
		HasColor: true,
	}, true
}
