package fontglyph

import (
	"encoding/binary"
	"testing"
)

func TestCBDTExtractorExtractsFormat17PNG(t *testing.T) {
	cbdt, cblc := fakeCBDTCBLC(testPNG(t))
	extractor, err := newCBDTExtractor(cbdt, cblc)
	if err != nil {
		t.Fatalf("newCBDTExtractor: %v", err)
	}
	glyph, ok := extractor.glyph(1, 24)
	if !ok {
		t.Fatalf("expected CBDT glyph")
	}
	if glyph.PPEM != 24 {
		t.Fatalf("PPEM = %d, want 24", glyph.PPEM)
	}
	if glyph.Format != "png " {
		t.Fatalf("format = %q, want png", glyph.Format)
	}
	if glyph.OriginOffsetX != 2 || glyph.OriginOffsetY != 18 {
		t.Fatalf("origin = (%d,%d), want (2,18)", glyph.OriginOffsetX, glyph.OriginOffsetY)
	}
	if glyph.Image.Bounds().Dx() != 2 || glyph.Image.Bounds().Dy() != 2 {
		t.Fatalf("image size = %v, want 2x2", glyph.Image.Bounds().Size())
	}
}

func TestCBDTExtractorMissingGlyph(t *testing.T) {
	cbdt, cblc := fakeCBDTCBLC(testPNG(t))
	extractor, err := newCBDTExtractor(cbdt, cblc)
	if err != nil {
		t.Fatalf("newCBDTExtractor: %v", err)
	}
	if _, ok := extractor.glyph(2, 24); ok {
		t.Fatalf("unexpected glyph outside CBDT range")
	}
}

func fakeCBDTCBLC(pngData []byte) (cbdt []byte, cblc []byte) {
	glyphDataLen := 5 + 4 + len(pngData)
	cbdt = make([]byte, 4+glyphDataLen)
	binary.BigEndian.PutUint16(cbdt[0:2], 3)
	binary.BigEndian.PutUint16(cbdt[2:4], 0)
	glyph := cbdt[4:]
	glyph[0] = 2  // height
	glyph[1] = 2  // width
	glyph[2] = 2  // bearingX
	glyph[3] = 18 // bearingY
	glyph[4] = 20 // advance
	binary.BigEndian.PutUint32(glyph[5:9], uint32(len(pngData)))
	copy(glyph[9:], pngData)

	const (
		sizeRecordOffset = 8
		arrayOffset      = 56
		subtableOffset   = 64
	)
	cblc = make([]byte, subtableOffset+8+8)
	binary.BigEndian.PutUint16(cblc[0:2], 3)
	binary.BigEndian.PutUint16(cblc[2:4], 0)
	binary.BigEndian.PutUint32(cblc[4:8], 1)

	record := cblc[sizeRecordOffset : sizeRecordOffset+48]
	binary.BigEndian.PutUint32(record[0:4], arrayOffset)
	binary.BigEndian.PutUint32(record[4:8], uint32(len(cblc)-arrayOffset))
	binary.BigEndian.PutUint32(record[8:12], 1)
	binary.BigEndian.PutUint16(record[40:42], 1)
	binary.BigEndian.PutUint16(record[42:44], 1)
	record[44] = 24
	record[45] = 24
	record[46] = 32
	record[47] = 1

	entry := cblc[arrayOffset : arrayOffset+8]
	binary.BigEndian.PutUint16(entry[0:2], 1)
	binary.BigEndian.PutUint16(entry[2:4], 1)
	binary.BigEndian.PutUint32(entry[4:8], subtableOffset-arrayOffset)

	subtable := cblc[subtableOffset:]
	binary.BigEndian.PutUint16(subtable[0:2], 1)  // indexFormat
	binary.BigEndian.PutUint16(subtable[2:4], 17) // imageFormat
	binary.BigEndian.PutUint32(subtable[4:8], 4)  // imageDataOffset into CBDT after header
	binary.BigEndian.PutUint32(subtable[8:12], 0)
	binary.BigEndian.PutUint32(subtable[12:16], uint32(glyphDataLen))
	return cbdt, cblc
}
