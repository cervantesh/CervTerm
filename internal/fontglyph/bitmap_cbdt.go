package fontglyph

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"
)

var (
	ErrNoCBDTTable      = errors.New("fontglyph: font has no CBDT table")
	ErrNoCBLCTable      = errors.New("fontglyph: font has no CBLC table")
	ErrInvalidCBLCTable = errors.New("fontglyph: invalid CBLC table")
	ErrInvalidCBDTTable = errors.New("fontglyph: invalid CBDT table")
)

type cbdtExtractor struct {
	cbdt    []byte
	strikes []cbdtStrike
}

type cbdtStrike struct {
	ppemX                   uint8
	ppemY                   uint8
	startGlyph              uint16
	endGlyph                uint16
	indexSubTableArray      uint32
	numberOfIndexSubTables  uint32
	indexSubtablesPopulated bool
	indexSubtables          []cbdtIndexSubtable
}

type cbdtIndexSubtable struct {
	firstGlyphID    uint16
	lastGlyphID     uint16
	indexFormat     uint16
	imageFormat     uint16
	imageDataOffset uint32
	data            []byte
}

type cbdtMetrics struct {
	width    uint8
	height   uint8
	bearingX int8
	bearingY int8
	advance  uint8
}

func newCBDTExtractor(cbdtData, cblcData []byte) (*cbdtExtractor, error) {
	if len(cbdtData) == 0 {
		return nil, ErrNoCBDTTable
	}
	if len(cblcData) == 0 {
		return nil, ErrNoCBLCTable
	}
	if len(cblcData) < 8 {
		return nil, ErrInvalidCBLCTable
	}
	major := binary.BigEndian.Uint16(cblcData[0:2])
	if major != 3 {
		return nil, ErrInvalidCBLCTable
	}
	numSizes := int(binary.BigEndian.Uint32(cblcData[4:8]))
	if 8+numSizes*48 > len(cblcData) {
		return nil, ErrInvalidCBLCTable
	}
	extractor := &cbdtExtractor{cbdt: cbdtData}
	for i := 0; i < numSizes; i++ {
		offset := 8 + i*48
		record := cblcData[offset : offset+48]
		strike := cbdtStrike{
			indexSubTableArray:     binary.BigEndian.Uint32(record[0:4]),
			numberOfIndexSubTables: binary.BigEndian.Uint32(record[8:12]),
			startGlyph:             binary.BigEndian.Uint16(record[40:42]),
			endGlyph:               binary.BigEndian.Uint16(record[42:44]),
			ppemX:                  record[44],
			ppemY:                  record[45],
		}
		if strike.numberOfIndexSubTables > 0 && int(strike.indexSubTableArray)+int(strike.numberOfIndexSubTables)*8 <= len(cblcData) {
			extractor.strikes = append(extractor.strikes, strike)
		}
	}
	if len(extractor.strikes) == 0 {
		return nil, ErrInvalidCBLCTable
	}
	if err := extractor.populateSubtables(cblcData); err != nil {
		return nil, err
	}
	return extractor, nil
}

func (e *cbdtExtractor) glyph(glyphID uint16, ppem uint16) (bitmapGlyph, bool) {
	strike, ok := e.selectStrike(ppem)
	if !ok || glyphID < strike.startGlyph || glyphID > strike.endGlyph {
		return bitmapGlyph{}, false
	}
	for _, subtable := range strike.indexSubtables {
		if glyphID < subtable.firstGlyphID || glyphID > subtable.lastGlyphID {
			continue
		}
		payload, metrics, ok := e.glyphPayload(subtable, glyphID)
		if !ok {
			return bitmapGlyph{}, false
		}
		img, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			return bitmapGlyph{}, false
		}
		return bitmapGlyph{
			Image:         img,
			PPEM:          uint16(strike.ppemY),
			OriginOffsetX: int16(metrics.bearingX),
			OriginOffsetY: int16(metrics.bearingY),
			Format:        "png ",
		}, true
	}
	return bitmapGlyph{}, false
}

func (e *cbdtExtractor) populateSubtables(cblc []byte) error {
	for si := range e.strikes {
		strike := &e.strikes[si]
		base := int(strike.indexSubTableArray)
		for i := uint32(0); i < strike.numberOfIndexSubTables; i++ {
			entry := base + int(i)*8
			first := binary.BigEndian.Uint16(cblc[entry : entry+2])
			last := binary.BigEndian.Uint16(cblc[entry+2 : entry+4])
			additional := binary.BigEndian.Uint32(cblc[entry+4 : entry+8])
			subOffset := base + int(additional)
			if subOffset+8 > len(cblc) {
				return ErrInvalidCBLCTable
			}
			indexFormat := binary.BigEndian.Uint16(cblc[subOffset : subOffset+2])
			imageFormat := binary.BigEndian.Uint16(cblc[subOffset+2 : subOffset+4])
			imageDataOffset := binary.BigEndian.Uint32(cblc[subOffset+4 : subOffset+8])
			strike.indexSubtables = append(strike.indexSubtables, cbdtIndexSubtable{
				firstGlyphID:    first,
				lastGlyphID:     last,
				indexFormat:     indexFormat,
				imageFormat:     imageFormat,
				imageDataOffset: imageDataOffset,
				data:            cblc[subOffset:],
			})
		}
		strike.indexSubtablesPopulated = true
	}
	return nil
}

func (e *cbdtExtractor) glyphPayload(st cbdtIndexSubtable, glyphID uint16) ([]byte, cbdtMetrics, bool) {
	start, end, metrics, ok := st.glyphLocation(glyphID)
	if !ok || start == end {
		return nil, cbdtMetrics{}, false
	}
	absStart := int(st.imageDataOffset + start)
	absEnd := int(st.imageDataOffset + end)
	if absStart < 0 || absEnd > len(e.cbdt) || absStart >= absEnd {
		return nil, cbdtMetrics{}, false
	}
	data := e.cbdt[absStart:absEnd]
	switch st.imageFormat {
	case 17:
		if len(data) < 9 {
			return nil, cbdtMetrics{}, false
		}
		metrics = parseSmallMetrics(data[:5])
		dataLen := int(binary.BigEndian.Uint32(data[5:9]))
		if 9+dataLen > len(data) {
			return nil, cbdtMetrics{}, false
		}
		return data[9 : 9+dataLen], metrics, true
	case 18:
		if len(data) < 12 {
			return nil, cbdtMetrics{}, false
		}
		metrics = parseBigMetrics(data[:8])
		dataLen := int(binary.BigEndian.Uint32(data[8:12]))
		if 12+dataLen > len(data) {
			return nil, cbdtMetrics{}, false
		}
		return data[12 : 12+dataLen], metrics, true
	case 19:
		if len(data) < 4 {
			return nil, cbdtMetrics{}, false
		}
		dataLen := int(binary.BigEndian.Uint32(data[0:4]))
		if 4+dataLen > len(data) {
			return nil, cbdtMetrics{}, false
		}
		return data[4 : 4+dataLen], metrics, true
	default:
		return nil, cbdtMetrics{}, false
	}
}

func (st cbdtIndexSubtable) glyphLocation(glyphID uint16) (start, end uint32, metrics cbdtMetrics, ok bool) {
	count := int(st.lastGlyphID-st.firstGlyphID) + 1
	idx := int(glyphID - st.firstGlyphID)
	switch st.indexFormat {
	case 1:
		if 8+(count+1)*4 > len(st.data) {
			return 0, 0, metrics, false
		}
		start = binary.BigEndian.Uint32(st.data[8+idx*4 : 12+idx*4])
		end = binary.BigEndian.Uint32(st.data[12+idx*4 : 16+idx*4])
		return start, end, metrics, true
	case 2:
		if len(st.data) < 20 {
			return 0, 0, metrics, false
		}
		imageSize := binary.BigEndian.Uint32(st.data[8:12])
		metrics = parseBigMetrics(st.data[12:20])
		start = uint32(idx) * imageSize
		end = start + imageSize
		return start, end, metrics, true
	case 3:
		if 8+(count+1)*2 > len(st.data) {
			return 0, 0, metrics, false
		}
		start = uint32(binary.BigEndian.Uint16(st.data[8+idx*2 : 10+idx*2]))
		end = uint32(binary.BigEndian.Uint16(st.data[10+idx*2 : 12+idx*2]))
		return start, end, metrics, true
	default:
		return 0, 0, metrics, false
	}
}

func (e *cbdtExtractor) selectStrike(ppem uint16) (cbdtStrike, bool) {
	if len(e.strikes) == 0 {
		return cbdtStrike{}, false
	}
	best := e.strikes[0]
	bestDelta := absInt(int(best.ppemY) - int(ppem))
	for _, strike := range e.strikes[1:] {
		delta := absInt(int(strike.ppemY) - int(ppem))
		if delta < bestDelta || delta == bestDelta && strike.ppemY > best.ppemY {
			best = strike
			bestDelta = delta
		}
	}
	return best, true
}

func parseSmallMetrics(data []byte) cbdtMetrics {
	return cbdtMetrics{height: data[0], width: data[1], bearingX: int8(data[2]), bearingY: int8(data[3]), advance: data[4]}
}

func parseBigMetrics(data []byte) cbdtMetrics {
	return cbdtMetrics{height: data[0], width: data[1], bearingX: int8(data[2]), bearingY: int8(data[3]), advance: data[4]}
}
