package fontglyph

import "errors"

var ErrInvalidFontData = errors.New("fontglyph: invalid sfnt font data")

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
	tableList, err := listSFNTTables(fontData)
	if err != nil {
		return ColorTables{}, err
	}
	var tables ColorTables
	for _, table := range tableList {
		switch table.Tag {
		case "CBDT":
			tables.HasCBDT = true
		case "CBLC":
			tables.HasCBLC = true
		case "sbix":
			tables.HasSbix = true
		case "COLR":
			tables.HasCOLR = true
		case "CPAL":
			tables.HasCPAL = true
		case "SVG ":
			tables.HasSVG = true
		}
	}
	if tables.HasCOLR {
		if data, ok, err := getSFNTTable(fontData, "COLR"); err == nil && ok && len(data) >= 2 {
			tables.HasCOLRVersion = true
			tables.COLRVersion = uint16(data[0])<<8 | uint16(data[1])
		}
	}
	return tables, nil
}
