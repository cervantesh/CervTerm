package fontglyph

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
)

var (
	ErrNoSbixTable        = errors.New("fontglyph: font has no sbix table")
	ErrInvalidSbixTable   = errors.New("fontglyph: invalid sbix table")
	ErrBitmapGlyphMissing = errors.New("fontglyph: bitmap glyph missing")
	ErrUnsupportedBitmap  = errors.New("fontglyph: unsupported bitmap glyph format")
)

type bitmapGlyph struct {
	Image         image.Image
	PPEM          uint16
	OriginOffsetX int16
	OriginOffsetY int16
	Format        string
}

type sbixExtractor struct {
	data      []byte
	numGlyphs int
	strikes   []sbixStrike
}

type sbixStrike struct {
	PPEM   uint16
	Offset uint32
}

func newSbixExtractor(table []byte, numGlyphs int) (*sbixExtractor, error) {
	if len(table) == 0 {
		return nil, ErrNoSbixTable
	}
	if len(table) < 8 || numGlyphs <= 0 {
		return nil, ErrInvalidSbixTable
	}
	version := binary.BigEndian.Uint16(table[0:2])
	if version > 1 {
		return nil, ErrInvalidSbixTable
	}
	numStrikes := int(binary.BigEndian.Uint32(table[4:8]))
	if numStrikes <= 0 || 8+numStrikes*4 > len(table) {
		return nil, ErrInvalidSbixTable
	}
	strikes := make([]sbixStrike, 0, numStrikes)
	for i := 0; i < numStrikes; i++ {
		offset := binary.BigEndian.Uint32(table[8+i*4 : 12+i*4])
		if offset == 0 || int(offset)+4+(numGlyphs+1)*4 > len(table) {
			continue
		}
		ppem := binary.BigEndian.Uint16(table[offset : offset+2])
		strikes = append(strikes, sbixStrike{PPEM: ppem, Offset: offset})
	}
	if len(strikes) == 0 {
		return nil, ErrInvalidSbixTable
	}
	return &sbixExtractor{data: table, numGlyphs: numGlyphs, strikes: strikes}, nil
}

func (e *sbixExtractor) glyph(glyphID uint16, ppem uint16) (bitmapGlyph, bool) {
	if int(glyphID) >= e.numGlyphs {
		return bitmapGlyph{}, false
	}
	strike, ok := e.selectStrike(ppem)
	if !ok {
		return bitmapGlyph{}, false
	}
	base := int(strike.Offset) + 4
	entry := base + int(glyphID)*4
	if entry+8 > len(e.data) {
		return bitmapGlyph{}, false
	}
	start := binary.BigEndian.Uint32(e.data[entry : entry+4])
	end := binary.BigEndian.Uint32(e.data[entry+4 : entry+8])
	if start == end {
		return bitmapGlyph{}, false
	}
	absStart := int(strike.Offset + start)
	absEnd := int(strike.Offset + end)
	if absStart+8 > len(e.data) || absEnd > len(e.data) || absStart >= absEnd {
		return bitmapGlyph{}, false
	}
	originX := int16(binary.BigEndian.Uint16(e.data[absStart : absStart+2]))
	originY := int16(binary.BigEndian.Uint16(e.data[absStart+2 : absStart+4]))
	format := string(e.data[absStart+4 : absStart+8])
	payload := e.data[absStart+8 : absEnd]
	img, err := decodeBitmapPayload(format, payload)
	if err != nil {
		return bitmapGlyph{}, false
	}
	return bitmapGlyph{Image: img, PPEM: strike.PPEM, OriginOffsetX: originX, OriginOffsetY: originY, Format: format}, true
}

func (e *sbixExtractor) selectStrike(ppem uint16) (sbixStrike, bool) {
	if len(e.strikes) == 0 {
		return sbixStrike{}, false
	}
	best := e.strikes[0]
	bestDelta := absInt(int(best.PPEM) - int(ppem))
	for _, strike := range e.strikes[1:] {
		delta := absInt(int(strike.PPEM) - int(ppem))
		if delta < bestDelta || delta == bestDelta && strike.PPEM > best.PPEM {
			best = strike
			bestDelta = delta
		}
	}
	return best, true
}

func decodeBitmapPayload(format string, payload []byte) (image.Image, error) {
	switch format {
	case "png ":
		return png.Decode(bytes.NewReader(payload))
	case "jpg ", "jpeg":
		return jpeg.Decode(bytes.NewReader(payload))
	default:
		return nil, ErrUnsupportedBitmap
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
