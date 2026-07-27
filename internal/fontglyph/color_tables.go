package fontglyph

import rasterpkg "cervterm/internal/fontglyph/raster"

var ErrInvalidFontData = rasterpkg.ErrInvalidFontData

type ColorTables struct {
	HasCBDT        bool
	HasCBLC        bool
	HasSbix        bool
	HasCOLR        bool
	HasCPAL        bool
	HasSVG         bool
	HasCOLRVersion bool
	COLRVersion    uint16
}

func (c ColorTables) HasBitmapColor() bool { return c.HasCBDT && c.HasCBLC || c.HasSbix }
func (c ColorTables) HasLayerColor() bool  { return c.HasCOLR && c.HasCPAL }
func (c ColorTables) HasRenderableLayerColor() bool {
	return c.HasCOLR && c.HasCPAL && c.HasCOLRVersion && c.COLRVersion <= 1
}
func (c ColorTables) HasAnyColor() bool { return c.HasBitmapColor() || c.HasLayerColor() || c.HasSVG }

func (c ColorTables) PreferredFormat() string {
	switch {
	case c.HasCBDT && c.HasCBLC:
		return "CBDT/CBLC"
	case c.HasSbix:
		return "sbix"
	case c.HasCOLR && c.HasCPAL:
		return "COLR/CPAL"
	case c.HasSVG:
		return "SVG"
	default:
		return ""
	}
}

func DetectColorTables(fontData []byte) (ColorTables, error) {
	tables, err := rasterpkg.DetectColorTables(fontData)
	if err != nil {
		return ColorTables{}, err
	}
	return colorTablesFromRaster(tables), nil
}

func colorTablesFromRaster(tables rasterpkg.ColorTables) ColorTables {
	return ColorTables{
		HasCBDT: tables.HasCBDT, HasCBLC: tables.HasCBLC, HasSbix: tables.HasSbix,
		HasCOLR: tables.HasCOLR, HasCPAL: tables.HasCPAL, HasSVG: tables.HasSVG,
		HasCOLRVersion: tables.HasCOLRVersion, COLRVersion: tables.COLRVersion,
	}
}
