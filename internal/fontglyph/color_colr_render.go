package fontglyph

import (
	"image"

	rasterpkg "cervterm/internal/fontglyph/raster"

	"golang.org/x/image/math/fixed"
)

func (b *OpenTypeBackend) rasterizeCOLRGlyph(lf loadedFace, r rune, cellSpan int, bounds fixed.Rectangle26_6, advance fixed.Int26_6) (RasterizedGlyph, bool) {
	renderer := rasterpkg.Renderer{Metrics: rasterMetrics(b)}
	glyph, ok := renderer.RasterizeCOLRGlyph(rasterFace(lf), r, cellSpan, bounds, advance)
	return rasterGlyphToRoot(glyph), ok
}

func (b *OpenTypeBackend) rasterizeShapedColorCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	renderer := rasterpkg.Renderer{Metrics: rasterMetrics(b)}
	glyph, ok := renderer.RasterizeShapedColorCluster(rasterFace(lf), rasterShapeGlyphs(shaped), cellSpan)
	return rasterGlyphToRoot(glyph), ok
}

func rasterMetrics(backend *OpenTypeBackend) rasterpkg.Metrics {
	if backend == nil {
		return rasterpkg.Metrics{}
	}
	return rasterpkg.Metrics{CellWidth: backend.cellW, CellHeight: backend.cellH, Baseline: backend.baseline, PPEM: backend.ppem}
}

func rasterFace(face loadedFace) rasterpkg.Face {
	return rasterpkg.Face{Parsed: face.sfnt, Color: face.rasterColor, SourcePath: face.sourcePath, FaceIndex: face.faceIndex}
}

func rasterShapeGlyphs(shaped []ShapedGlyph) []rasterpkg.ShapeGlyph {
	if len(shaped) == 0 {
		return nil
	}
	out := make([]rasterpkg.ShapeGlyph, len(shaped))
	for i, glyph := range shaped {
		out[i] = rasterpkg.ShapeGlyph{GlyphID: glyph.GlyphID, XOffset: glyph.XOffset, YOffset: glyph.YOffset, XAdvance: glyph.XAdvance}
	}
	return out
}

func rasterGlyphToRoot(glyph rasterpkg.Glyph) RasterizedGlyph {
	return RasterizedGlyph{
		Image: glyph.Image, Width: glyph.Width, Height: glyph.Height,
		BearingX: glyph.BearingX, BearingY: glyph.BearingY, AdvanceX: glyph.AdvanceX,
		CellSpan: glyph.CellSpan, HasColor: glyph.HasColor, Subpixel: glyph.Subpixel,
	}
}

func visibleRGBABounds(img *image.RGBA) (image.Rectangle, bool) {
	return rasterpkg.VisibleRGBABounds(img)
}
