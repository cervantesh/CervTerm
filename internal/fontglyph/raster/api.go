package raster

import (
	"image"
	"image/draw"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// BitmapGlyph is a detached bitmap strike result.
type BitmapGlyph struct {
	Image         image.Image
	PPEM          uint16
	OriginOffsetX int16
	OriginOffsetY int16
	Format        string
}

// NewColorFace parses the raster-private color tables once for one immutable SFNT face.
func NewColorFace(fontData []byte, parsed *sfnt.Font) (*ColorFace, error) {
	tables, err := DetectColorTables(fontData)
	if err != nil {
		return nil, err
	}
	face := &ColorFace{tables: tables}
	if tables.HasSbix && parsed != nil {
		if table, ok, tableErr := getSFNTTable(fontData, "sbix"); tableErr == nil && ok {
			face.sbix, _ = newSbixExtractor(table, parsed.NumGlyphs())
		}
	}
	if tables.HasCBDT && tables.HasCBLC {
		cbdt, hasCBDT, cbdtErr := getSFNTTable(fontData, "CBDT")
		cblc, hasCBLC, cblcErr := getSFNTTable(fontData, "CBLC")
		if cbdtErr == nil && cblcErr == nil && hasCBDT && hasCBLC {
			face.cbdt, _ = newCBDTExtractor(cbdt, cblc)
		}
	}
	if tables.HasRenderableLayerColor() {
		colr, hasCOLR, colrErr := getSFNTTable(fontData, "COLR")
		cpal, hasCPAL, cpalErr := getSFNTTable(fontData, "CPAL")
		if colrErr == nil && cpalErr == nil && hasCOLR && hasCPAL {
			face.colr, _ = newCOLRParser(colr, cpal)
		}
	}
	if tables.HasSVG {
		if table, ok, tableErr := getSFNTTable(fontData, "SVG "); tableErr == nil && ok {
			face.svg, _ = newSVGExtractor(table)
		}
	}
	return face, nil
}

// Tables returns a detached color-table capability projection.
func (f *ColorFace) Tables() ColorTables {
	if f == nil {
		return ColorTables{}
	}
	return f.tables
}

// HasCOLR reports whether a renderable COLR parser was constructed.
func (f *ColorFace) HasCOLR() bool { return f != nil && f.colr != nil }

// HasSVG reports whether an SVG extractor was constructed.
func (f *ColorFace) HasSVG() bool { return f != nil && f.svg != nil }

// Bitmap returns the preferred sbix then CBDT strike, preserving selection order.
func (f *ColorFace) Bitmap(glyphID uint16, ppem uint16) (BitmapGlyph, bool) {
	if f == nil {
		return BitmapGlyph{}, false
	}
	var glyph bitmapGlyph
	var ok bool
	if f.sbix != nil {
		glyph, ok = f.sbix.glyph(glyphID, ppem)
	}
	if !ok && f.cbdt != nil {
		glyph, ok = f.cbdt.glyph(glyphID, ppem)
	}
	if !ok {
		return BitmapGlyph{}, false
	}
	return BitmapGlyph{
		Image: glyph.Image, PPEM: glyph.PPEM,
		OriginOffsetX: glyph.OriginOffsetX, OriginOffsetY: glyph.OriginOffsetY, Format: glyph.Format,
	}, true
}

// COLRGlyph returns deterministic paint/layer order for one glyph.
func (f *ColorFace) COLRGlyph(glyphID uint16, preferV0 bool) (COLRGlyph, error) {
	if f == nil || f.colr == nil {
		return COLRGlyph{}, ErrNoCOLRTable
	}
	if preferV0 && len(f.colr.palettes) > 0 {
		if glyph, err := f.colr.glyphV0(glyphID, f.colr.palettes[0]); err == nil && len(glyph.Layers) > 0 {
			return glyph, nil
		}
	}
	return f.colr.glyph(glyphID, 0)
}

// RasterizeSVG rasterizes one SVG glyph document into an exact RGBA canvas.
func (f *ColorFace) RasterizeSVG(glyphID uint16, width, height int) (*image.RGBA, bool) {
	if f == nil || f.svg == nil {
		return nil, false
	}
	doc, ok := f.svg.document(glyphID)
	if !ok {
		return nil, false
	}
	rendered, ok := rasterizeSVGDocument(doc, width, height)
	if !ok {
		return nil, false
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(out, out.Bounds(), image.Transparent, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), rendered, image.Point{}, draw.Over)
	return out, true
}

// GlyphIndex resolves one glyph through the supplied immutable SFNT parser.
func GlyphIndex(parsed *sfnt.Font, value rune) (uint16, bool) {
	if parsed == nil {
		return 0, false
	}
	var buffer sfnt.Buffer
	glyph, err := parsed.GlyphIndex(&buffer, value)
	return uint16(glyph), err == nil && glyph != 0
}

// AdvancePixels projects a 26.6 advance without changing historical precision.
func AdvancePixels(advance fixed.Int26_6) float64 { return float64(advance) / 64 }

// RasterizeSVGTableGlyph extracts and rasterizes one SVG table glyph.
func RasterizeSVGTableGlyph(table []byte, glyphID uint16, width, height int) (*image.RGBA, bool) {
	extractor, err := newSVGExtractor(table)
	if err != nil {
		return nil, false
	}
	document, ok := extractor.document(glyphID)
	if !ok {
		return nil, false
	}
	return rasterizeSVGDocument(document, width, height)
}
