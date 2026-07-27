// Package raster owns renderer-neutral glyph rasterization and private SFNT
// color-table extraction. It has no dependency on the fontglyph facade or on
// platform-native adapters.
package raster

import (
	"image"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/sfnt"
)

// Glyph is the detached renderer-neutral result of one raster operation.
// Image retains its exact Pix, Stride, Rect, and RGBA format.
type Glyph struct {
	Image    *image.RGBA
	Width    int
	Height   int
	BearingX int
	BearingY int
	AdvanceX float64
	CellSpan int
	HasColor bool
	Subpixel bool
}

// ShapeGlyph is the minimum detached positioning input needed for a shaped
// color-glyph run.
type ShapeGlyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

// Face describes immutable parsed/raster state supplied by the root owner.
// Color is intentionally opaque so only this package can retain color-table
// parser state.
type Face struct {
	Parsed     *sfnt.Font
	Color      *ColorFace
	SourcePath string
	FaceIndex  int
}

// Metrics are the complete cell-space inputs to renderer-neutral raster paths.
type Metrics struct {
	CellWidth  int
	CellHeight int
	Baseline   int
	PPEM       uint16
}

// FeaturePolicy is kept as a named seam so the facade can project feature
// configuration without raster importing shape or platform.
type FeaturePolicy struct {
	Features fontdesc.FeatureSet
}

// ColorFace privately owns parsed bitmap/COLR/SVG state.
type ColorFace struct {
	tables ColorTables
	sbix   *sbixExtractor
	cbdt   *cbdtExtractor
	colr   *colrParser
	svg    *svgExtractor
}
