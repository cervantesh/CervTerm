package fontglyph

import (
	"image"

	rasterpkg "cervterm/internal/fontglyph/raster"
)

var (
	ErrNoSbixTable        = rasterpkg.ErrNoSbixTable
	ErrInvalidSbixTable   = rasterpkg.ErrInvalidSbixTable
	ErrBitmapGlyphMissing = rasterpkg.ErrBitmapGlyphMissing
	ErrUnsupportedBitmap  = rasterpkg.ErrUnsupportedBitmap
	ErrNoCBDTTable        = rasterpkg.ErrNoCBDTTable
	ErrNoCBLCTable        = rasterpkg.ErrNoCBLCTable
	ErrInvalidCBLCTable   = rasterpkg.ErrInvalidCBLCTable
	ErrInvalidCBDTTable   = rasterpkg.ErrInvalidCBDTTable
	ErrNoCOLRTable        = rasterpkg.ErrNoCOLRTable
	ErrNoCPALTable        = rasterpkg.ErrNoCPALTable
	ErrInvalidCOLRTable   = rasterpkg.ErrInvalidCOLRTable
	ErrInvalidCPALTable   = rasterpkg.ErrInvalidCPALTable
	ErrUnsupportedCOLR    = rasterpkg.ErrUnsupportedCOLR
	ErrCOLRGlyphNotFound  = rasterpkg.ErrCOLRGlyphNotFound
	ErrNoSVGTable         = rasterpkg.ErrNoSVGTable
	ErrInvalidSVGTable    = rasterpkg.ErrInvalidSVGTable
)

type bitmapGlyph struct {
	Image         image.Image
	PPEM          uint16
	OriginOffsetX int16
	OriginOffsetY int16
	Format        string
}
