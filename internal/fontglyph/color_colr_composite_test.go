package fontglyph

import (
	"encoding/binary"
	"errors"
	"image/color"
	"testing"
)

func TestCOLRParserExtractsV1PaintCompositeSrcOver(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeSrcOver), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 2 {
		t.Fatalf("layers = %d, want 2: %#v", len(glyph.Layers), glyph.Layers)
	}
	if glyph.Layers[0].GlyphID != 12 || glyph.Layers[0].Color != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("unexpected backdrop layer: %#v", glyph.Layers[0])
	}
	if glyph.Layers[1].GlyphID != 11 || glyph.Layers[1].Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("unexpected source layer: %#v", glyph.Layers[1])
	}
}

func TestCOLRParserExtractsV1PaintCompositeDestOver(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeDestOver), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 2 {
		t.Fatalf("layers = %d, want 2: %#v", len(glyph.Layers), glyph.Layers)
	}
	if glyph.Layers[0].GlyphID != 11 || glyph.Layers[1].GlyphID != 12 {
		t.Fatalf("expected source then backdrop for DEST_OVER, got %#v", glyph.Layers)
	}
}

func TestCOLRParserExtractsV1PaintCompositeDest(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeDest), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 || glyph.Layers[0].GlyphID != 12 {
		t.Fatalf("expected only backdrop for DEST, got %#v", glyph.Layers)
	}
}

func TestCOLRParserExtractsV1PaintCompositeClear(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeClear), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 0 {
		t.Fatalf("expected no layers for CLEAR, got %#v", glyph.Layers)
	}
}

func TestCOLRParserExtractsV1PaintCompositeAdvancedNode(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeMultiply), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1 composite node: %#v", len(glyph.Layers), glyph.Layers)
	}
	layer := glyph.Layers[0]
	if layer.Fill != COLRFillComposite || layer.CompositeMode != colrCompositeMultiply {
		t.Fatalf("unexpected composite layer: %#v", layer)
	}
	if len(layer.Source) != 1 || layer.Source[0].GlyphID != 11 || len(layer.Backdrop) != 1 || layer.Backdrop[0].GlyphID != 12 {
		t.Fatalf("unexpected composite children: %#v", layer)
	}
}

func TestCOLRParserExtractsV1PaintCompositeHSLNode(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeHSLColor), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 || glyph.Layers[0].Fill != COLRFillComposite || glyph.Layers[0].CompositeMode != colrCompositeHSLColor {
		t.Fatalf("unexpected HSL composite node: %#v", glyph.Layers)
	}
}

func TestCompositeCOLRPixelPorterDuffAndBlendModes(t *testing.T) {
	src := color.RGBA{R: 255, A: 255}
	dst := color.RGBA{G: 255, A: 255}
	if got := compositeCOLRPixel(src, dst, colrCompositeSrcIn); got != src {
		t.Fatalf("src-in opaque = %#v, want %#v", got, src)
	}
	if got := compositeCOLRPixel(src, dst, colrCompositeDestOut); got.A != 0 {
		t.Fatalf("dest-out opaque alpha = %d, want 0", got.A)
	}
	if got := compositeCOLRPixel(src, dst, colrCompositeMultiply); got != (color.RGBA{A: 255}) {
		t.Fatalf("multiply red/green = %#v, want black opaque", got)
	}
	if got := compositeCOLRPixel(src, dst, colrCompositeScreen); got != (color.RGBA{R: 255, G: 255, A: 255}) {
		t.Fatalf("screen red/green = %#v, want yellow opaque", got)
	}
	if got := compositeCOLRPixel(color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}, colrCompositeHSLColor); got.A != 255 || got.R == 0 || got.B != 0 {
		t.Fatalf("HSL color red over blue = %#v, want opaque red-family color with backdrop luminosity", got)
	}
}

func TestCOLRParserRejectsUnsupportedCompositeMode(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintComposite(colrCompositeLast+1), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	_, err = parser.glyph(10, 0)
	if !errors.Is(err, ErrUnsupportedCOLR) {
		t.Fatalf("expected ErrUnsupportedCOLR, got %v", err)
	}
}

func fakeCOLRv1PaintComposite(mode int) []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 74)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 32 // PaintComposite
	writeOffset24(data[45:48], 8)
	data[48] = byte(mode)
	writeOffset24(data[49:52], 19)

	data[52] = 10 // source PaintGlyph
	writeOffset24(data[53:56], 6)
	binary.BigEndian.PutUint16(data[56:58], 11)
	data[58] = 2
	binary.BigEndian.PutUint16(data[59:61], 0)
	binary.BigEndian.PutUint16(data[61:63], 0x4000)

	data[63] = 10 // backdrop PaintGlyph
	writeOffset24(data[64:67], 6)
	binary.BigEndian.PutUint16(data[67:69], 12)
	data[69] = 2
	binary.BigEndian.PutUint16(data[70:72], 1)
	binary.BigEndian.PutUint16(data[72:74], 0x4000)
	return data
}
