package fontglyph

import (
	"cervterm/internal/fontglyph/cache"
	rasterpkg "cervterm/internal/fontglyph/raster"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// parsedFontData is the size-independent result of parsing a font file.
type parsedFontData struct {
	sfnt        *sfnt.Font
	tables      ColorTables
	rasterColor *rasterpkg.ColorFace
}

var (
	errFontCacheCapacity = cache.ErrCapacity
	errFontFileGrew      = cache.ErrFileGrew
)

// parseFontData does the expensive, size-independent work.
func parseFontData(data []byte, index int) (*parsedFontData, error) {
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	parsed, err := collection.Font(index)
	if err != nil {
		return nil, err
	}
	pf := &parsedFontData{sfnt: parsed}
	if index != 0 {
		return pf, nil
	}
	colorFace, err := rasterpkg.NewColorFace(data, parsed)
	if err != nil {
		return pf, nil
	}
	pf.rasterColor = colorFace
	pf.tables = colorTablesFromRaster(colorFace.Tables())
	return pf, nil
}
