package fontglyph

import (
	"cervterm/internal/fontglyph/cache"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// parsedFontData is the size-independent result of parsing a font file.
type parsedFontData struct {
	sfnt   *sfnt.Font
	tables ColorTables
	sbix   *sbixExtractor
	cbdt   *cbdtExtractor
	colr   *colrParser
	svg    *svgExtractor
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
	tables, err := DetectColorTables(data)
	if err != nil {
		return pf, nil
	}
	pf.tables = tables
	if tables.HasSbix && parsed != nil {
		if table, ok, tableErr := getSFNTTable(data, "sbix"); tableErr == nil && ok {
			pf.sbix, _ = newSbixExtractor(table, parsed.NumGlyphs())
		}
	}
	if tables.HasCBDT && tables.HasCBLC {
		cbdt, hasCBDT, cbdtErr := getSFNTTable(data, "CBDT")
		cblc, hasCBLC, cblcErr := getSFNTTable(data, "CBLC")
		if cbdtErr == nil && cblcErr == nil && hasCBDT && hasCBLC {
			pf.cbdt, _ = newCBDTExtractor(cbdt, cblc)
		}
	}
	if tables.HasRenderableLayerColor() {
		colr, hasCOLR, colrErr := getSFNTTable(data, "COLR")
		cpal, hasCPAL, cpalErr := getSFNTTable(data, "CPAL")
		if colrErr == nil && cpalErr == nil && hasCOLR && hasCPAL {
			pf.colr, _ = newCOLRParser(colr, cpal)
		}
	}
	if tables.HasSVG {
		if table, ok, tableErr := getSFNTTable(data, "SVG "); tableErr == nil && ok {
			pf.svg, _ = newSVGExtractor(table)
		}
	}
	return pf, nil
}
