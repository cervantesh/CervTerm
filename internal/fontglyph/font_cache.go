package fontglyph

import (
	"strconv"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// parsedFontData is the size-independent result of parsing a font file: the
// glyph outlines (sfnt) plus any color-glyph tables. Only opentype.Face depends
// on the point size, so this can be parsed once and reused across zoom steps.
type parsedFontData struct {
	sfnt   *sfnt.Font
	tables ColorTables
	sbix   *sbixExtractor
	cbdt   *cbdtExtractor
	colr   *colrParser
	svg    *svgExtractor
}

var (
	fontParseMu    sync.Mutex
	fontParseCache = map[string]*parsedFontData{}
)

// parseFontData does the expensive, size-independent work: parse the SFNT
// collection and extract color-glyph tables. Kept separate from face creation
// so the result can be cached.
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
		if sbixData, ok, err := getSFNTTable(data, "sbix"); err == nil && ok {
			pf.sbix, _ = newSbixExtractor(sbixData, parsed.NumGlyphs())
		}
	}
	if tables.HasCBDT && tables.HasCBLC {
		cbdtData, hasCBDT, cbdtErr := getSFNTTable(data, "CBDT")
		cblcData, hasCBLC, cblcErr := getSFNTTable(data, "CBLC")
		if cbdtErr == nil && cblcErr == nil && hasCBDT && hasCBLC {
			pf.cbdt, _ = newCBDTExtractor(cbdtData, cblcData)
		}
	}
	if tables.HasRenderableLayerColor() {
		colrData, hasCOLR, colrErr := getSFNTTable(data, "COLR")
		cpalData, hasCPAL, cpalErr := getSFNTTable(data, "CPAL")
		if colrErr == nil && cpalErr == nil && hasCOLR && hasCPAL {
			pf.colr, _ = newCOLRParser(colrData, cpalData)
		}
	}
	if tables.HasSVG {
		if svgData, ok, err := getSFNTTable(data, "SVG "); err == nil && ok {
			pf.svg, _ = newSVGExtractor(svgData)
		}
	}
	return pf, nil
}

// faceFromParsed creates the size-dependent opentype.Face for a spec and pairs
// it with the shared, cached parse result.
func faceFromParsed(pf *parsedFontData, spec Spec) (loadedFace, font.Metrics, error) {
	face, err := opentype.NewFace(pf.sfnt, &opentype.FaceOptions{Size: spec.Size, DPI: spec.DPI, Hinting: font.HintingFull})
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf := loadedFace{face: face, sfnt: pf.sfnt, tables: pf.tables, sbix: pf.sbix, cbdt: pf.cbdt, colr: pf.colr, svg: pf.svg}
	return lf, face.Metrics(), nil
}

// cachedParsedFont returns the parsed data for key+index, parsing (via load) and
// caching it on first use. The parse result is immutable and size-independent,
// so the same font is parsed at most once regardless of how many times the user
// zooms. load is only called on a cache miss, so a hit avoids the disk read too.
func cachedParsedFont(key string, index int, load func() ([]byte, error)) (*parsedFontData, error) {
	ck := key + "#" + strconv.Itoa(index)
	fontParseMu.Lock()
	pf := fontParseCache[ck]
	fontParseMu.Unlock()
	if pf != nil {
		return pf, nil
	}
	data, err := load()
	if err != nil {
		return nil, err
	}
	pf, err = parseFontData(data, index)
	if err != nil {
		return nil, err
	}
	fontParseMu.Lock()
	fontParseCache[ck] = pf
	fontParseMu.Unlock()
	return pf, nil
}

// loadCachedFaceIndex builds a face for spec, reusing the cached parse for key so
// repeated size changes (zoom) don't re-read or re-parse the font.
func loadCachedFaceIndex(key string, index int, spec Spec, load func() ([]byte, error)) (loadedFace, font.Metrics, error) {
	pf, err := cachedParsedFont(key, index, load)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf, metrics, err := faceFromParsed(pf, spec)
	if err != nil {
		return loadedFace{}, font.Metrics{}, err
	}
	lf.faceIndex = index
	return lf, metrics, nil
}
