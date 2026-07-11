package fontglyph

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSbixExtractorExtractsPNGGlyph(t *testing.T) {
	pngData := testPNG(t)
	table := fakeSbixTable(3, 24, 1, pngData)
	extractor, err := newSbixExtractor(table, 3)
	if err != nil {
		t.Fatalf("newSbixExtractor: %v", err)
	}
	glyph, ok := extractor.glyph(1, 20)
	if !ok {
		t.Fatalf("expected glyph")
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

func TestSbixExtractorMissingGlyph(t *testing.T) {
	table := fakeSbixTable(3, 24, 1, testPNG(t))
	extractor, err := newSbixExtractor(table, 3)
	if err != nil {
		t.Fatalf("newSbixExtractor: %v", err)
	}
	if _, ok := extractor.glyph(2, 24); ok {
		t.Fatalf("unexpected glyph for empty sbix slot")
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	img.SetRGBA(1, 0, color.RGBA{0, 255, 0, 255})
	img.SetRGBA(0, 1, color.RGBA{0, 0, 255, 255})
	img.SetRGBA(1, 1, color.RGBA{255, 255, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func fakeSbixTable(numGlyphs int, ppem uint16, glyphID uint16, pngData []byte) []byte {
	strikeOffset := uint32(12)
	offsetsLen := (numGlyphs + 1) * 4
	glyphStart := uint32(4 + offsetsLen)
	glyphEnd := glyphStart + uint32(8+len(pngData))
	table := make([]byte, int(strikeOffset)+int(glyphEnd))
	binary.BigEndian.PutUint16(table[0:2], 1) // version
	binary.BigEndian.PutUint32(table[4:8], 1) // strikes
	binary.BigEndian.PutUint32(table[8:12], strikeOffset)

	strike := table[strikeOffset:]
	binary.BigEndian.PutUint16(strike[0:2], ppem)
	binary.BigEndian.PutUint16(strike[2:4], 72)
	for i := 0; i < numGlyphs+1; i++ {
		binary.BigEndian.PutUint32(strike[4+i*4:8+i*4], glyphStart)
	}
	binary.BigEndian.PutUint32(strike[4+int(glyphID)*4:8+int(glyphID)*4], glyphStart)
	binary.BigEndian.PutUint32(strike[4+int(glyphID+1)*4:8+int(glyphID+1)*4], glyphEnd)

	glyph := strike[glyphStart:glyphEnd]
	binary.BigEndian.PutUint16(glyph[0:2], uint16(2))
	binary.BigEndian.PutUint16(glyph[2:4], uint16(18))
	copy(glyph[4:8], []byte("png "))
	copy(glyph[8:], pngData)
	return table
}
