package fontglyph

import (
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func (b *OpenTypeBackend) rasterizeSVGColorGlyph(lf loadedFace, r rune, cellSpan int, advance fixed.Int26_6) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || lf.rasterColor == nil || !lf.rasterColor.HasSVG() {
		return RasterizedGlyph{}, false
	}
	var buf sfnt.Buffer
	glyphID, err := lf.sfnt.GlyphIndex(&buf, r)
	if err != nil || glyphID == 0 {
		return RasterizedGlyph{}, false
	}
	canvasW := b.cellW * max(1, cellSpan)
	canvasH := b.cellH
	svgImg, ok := lf.rasterColor.RasterizeSVG(uint16(glyphID), canvasW, canvasH)
	if !ok {
		return RasterizedGlyph{}, false
	}
	img := svgImg
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
