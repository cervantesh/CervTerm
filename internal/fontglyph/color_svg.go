package fontglyph

import (
	"encoding/binary"
	"errors"
)

var (
	ErrNoSVGTable      = errors.New("fontglyph: font has no SVG table")
	ErrInvalidSVGTable = errors.New("fontglyph: invalid SVG table")
)

type svgExtractor struct {
	documents []svgDocumentRecord
}

type svgDocumentRecord struct {
	startGlyphID uint16
	endGlyphID   uint16
	document     []byte
}

func newSVGExtractor(svgData []byte) (*svgExtractor, error) {
	if len(svgData) == 0 {
		return nil, ErrNoSVGTable
	}
	if len(svgData) < 10 {
		return nil, ErrInvalidSVGTable
	}
	version := binary.BigEndian.Uint16(svgData[0:2])
	if version != 0 {
		return nil, ErrInvalidSVGTable
	}
	documentListOffset := int(binary.BigEndian.Uint32(svgData[2:6]))
	if documentListOffset < 0 || documentListOffset+2 > len(svgData) {
		return nil, ErrInvalidSVGTable
	}
	numEntries := int(binary.BigEndian.Uint16(svgData[documentListOffset : documentListOffset+2]))
	entriesOffset := documentListOffset + 2
	if entriesOffset+numEntries*12 > len(svgData) {
		return nil, ErrInvalidSVGTable
	}
	extractor := &svgExtractor{documents: make([]svgDocumentRecord, 0, numEntries)}
	for i := 0; i < numEntries; i++ {
		o := entriesOffset + i*12
		startGlyphID := binary.BigEndian.Uint16(svgData[o : o+2])
		endGlyphID := binary.BigEndian.Uint16(svgData[o+2 : o+4])
		docOffset := documentListOffset + int(binary.BigEndian.Uint32(svgData[o+4:o+8]))
		docLength := int(binary.BigEndian.Uint32(svgData[o+8 : o+12]))
		if startGlyphID > endGlyphID || docOffset < documentListOffset || docLength < 0 || docOffset+docLength > len(svgData) {
			return nil, ErrInvalidSVGTable
		}
		extractor.documents = append(extractor.documents, svgDocumentRecord{
			startGlyphID: startGlyphID,
			endGlyphID:   endGlyphID,
			document:     svgData[docOffset : docOffset+docLength],
		})
	}
	return extractor, nil
}

func (e *svgExtractor) document(glyphID uint16) ([]byte, bool) {
	for _, doc := range e.documents {
		if glyphID >= doc.startGlyphID && glyphID <= doc.endGlyphID {
			return doc.document, true
		}
	}
	return nil, false
}
