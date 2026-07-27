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
