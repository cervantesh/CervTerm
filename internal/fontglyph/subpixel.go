package fontglyph

import (
	"image"

	rasterpkg "cervterm/internal/fontglyph/raster"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func applySubpixelFIR(samples []uint8, index int) uint8 {
	return rasterpkg.ApplySubpixelFIR(samples, index)
}

func (b *OpenTypeBackend) rasterizeSubpixel(lf loadedFace, glyphID sfnt.GlyphIndex, bounds fixed.Rectangle26_6, cellSpan int) (*image.RGBA, bool) {
	return rasterpkg.RasterizeSubpixel(lf.sfnt, glyphID, bounds, rasterMetrics(b), cellSpan)
}
