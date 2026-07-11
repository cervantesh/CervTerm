package fontglyph

import "image/color"

type COLRGlyph struct {
	GlyphID uint16
	Layers  []COLRLayer
}

type COLRLayer struct {
	GlyphID        uint16
	PaletteIndex   uint16
	Color          color.RGBA
	Foreground     bool
	Transform      COLRTransform
	Fill           COLRFillKind
	LinearGradient COLRLinearGradient
	RadialGradient COLRRadialGradient
	SweepGradient  COLRSweepGradient
	CompositeMode  int
	Source         []COLRLayer
	Backdrop       []COLRLayer
}

type colrParser struct {
	data            []byte
	version         uint16
	baseGlyphs      []colrBaseGlyph
	layers          []colrLayerRecord
	basePaints      []colrBaseGlyphPaint
	layerPaints     []uint32
	palettes        [][]color.RGBA
	variationStore  *colrVariationStore
	variationCoords []float64
}

type colrBaseGlyph struct {
	glyphID    uint16
	firstLayer uint16
	numLayers  uint16
}

type colrLayerRecord struct {
	glyphID      uint16
	paletteIndex uint16
}

type colrBaseGlyphPaint struct {
	glyphID     uint16
	paintOffset uint32
}
