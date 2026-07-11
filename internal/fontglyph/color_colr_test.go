package fontglyph

import (
	"encoding/binary"
	"errors"
	"image/color"
	"math"
	"testing"
)

func TestCOLRParserExtractsV0Layers(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv0(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if glyph.GlyphID != 10 || len(glyph.Layers) != 2 {
		t.Fatalf("unexpected glyph: %#v", glyph)
	}
	if glyph.Layers[0].GlyphID != 11 || glyph.Layers[0].PaletteIndex != 0 || glyph.Layers[0].Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("unexpected first layer: %#v", glyph.Layers[0])
	}
	if glyph.Layers[1].GlyphID != 12 || glyph.Layers[1].PaletteIndex != 1 || glyph.Layers[1].Color != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("unexpected second layer: %#v", glyph.Layers[1])
	}
}

func TestCOLRParserForegroundLayer(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv0Foreground(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 || !glyph.Layers[0].Foreground {
		t.Fatalf("expected foreground layer, got %#v", glyph.Layers)
	}
}

func TestCOLRParserExtractsV1PaintGlyphSolid(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintGlyphSolid(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if parser.version != 1 {
		t.Fatalf("version = %d, want 1", parser.version)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1: %#v", len(glyph.Layers), glyph.Layers)
	}
	layer := glyph.Layers[0]
	if layer.GlyphID != 11 || layer.PaletteIndex != 0 || layer.Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("unexpected layer: %#v", layer)
	}
}

func TestCOLRParserExtractsV1PaintColrLayers(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintColrLayers(), fakeCPAL())
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
	if glyph.Layers[0].GlyphID != 11 || glyph.Layers[0].Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("unexpected first layer: %#v", glyph.Layers[0])
	}
	if glyph.Layers[1].GlyphID != 12 || glyph.Layers[1].Color != (color.RGBA{G: 255, A: 128}) {
		t.Fatalf("unexpected second layer: %#v", glyph.Layers[1])
	}
}

func TestCOLRParserExtractsV1PaintTranslate(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintTranslate(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(glyph.Layers))
	}
	want := COLRTransform{XX: 1, YY: 1, DX: 5, DY: -3}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestCOLRParserExtractsV1PaintTransform(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintTransform(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	want := COLRTransform{XX: 2, YY: 2, DX: 5, DY: -3}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestCOLRParserExtractsV1PaintLinearGradient(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintLinearGradient(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(glyph.Layers))
	}
	layer := glyph.Layers[0]
	if layer.GlyphID != 11 || layer.Fill != COLRFillLinearGradient {
		t.Fatalf("unexpected gradient layer: %#v", layer)
	}
	if len(layer.LinearGradient.Stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(layer.LinearGradient.Stops))
	}
	if layer.LinearGradient.Stops[0].Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("first stop = %#v", layer.LinearGradient.Stops[0])
	}
	if layer.LinearGradient.Stops[1].Color != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("second stop = %#v", layer.LinearGradient.Stops[1])
	}
}

func TestCOLRParserExtractsV1PaintRadialGradient(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintRadialGradient(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(glyph.Layers))
	}
	layer := glyph.Layers[0]
	if layer.GlyphID != 11 || layer.Fill != COLRFillRadialGradient {
		t.Fatalf("unexpected radial layer: %#v", layer)
	}
	if len(layer.RadialGradient.Stops) != 2 || layer.RadialGradient.Radius1 != 10 {
		t.Fatalf("unexpected radial gradient: %#v", layer.RadialGradient)
	}
}

func TestCOLRParserExtractsV1PaintSweepGradient(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintSweepGradient(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(glyph.Layers))
	}
	layer := glyph.Layers[0]
	if layer.GlyphID != 11 || layer.Fill != COLRFillSweepGradient {
		t.Fatalf("unexpected sweep layer: %#v", layer)
	}
	if len(layer.SweepGradient.Stops) != 2 || layer.SweepGradient.CenterX != 10 {
		t.Fatalf("unexpected sweep gradient: %#v", layer.SweepGradient)
	}
}

func assertTransformClose(t *testing.T, got, want COLRTransform) {
	t.Helper()
	gotValues := []float64{got.XX, got.YX, got.XY, got.YY, got.DX, got.DY}
	wantValues := []float64{want.XX, want.YX, want.XY, want.YY, want.DX, want.DY}
	for i := range gotValues {
		if math.Abs(gotValues[i]-wantValues[i]) > 0.0001 {
			t.Fatalf("transform[%d] = %f, want %f (got %#v want %#v)", i, gotValues[i], wantValues[i], got, want)
		}
	}
}

func TestCOLRParserRejectsUnsupportedV1Paint(t *testing.T) {
	colr := fakeCOLRv1PaintGlyphSolid()
	colr[44] = 99
	parser, err := newCOLRParser(colr, fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	_, err = parser.glyph(10, 0)
	if !errors.Is(err, ErrUnsupportedCOLR) {
		t.Fatalf("expected ErrUnsupportedCOLR, got %v", err)
	}
}

func fakeCOLRv0() []byte {
	data := make([]byte, 14+6+8)
	binary.BigEndian.PutUint16(data[0:2], 0)  // version
	binary.BigEndian.PutUint16(data[2:4], 1)  // base glyphs
	binary.BigEndian.PutUint32(data[4:8], 14) // base offset
	binary.BigEndian.PutUint32(data[8:12], 20)
	binary.BigEndian.PutUint16(data[12:14], 2) // layers
	binary.BigEndian.PutUint16(data[14:16], 10)
	binary.BigEndian.PutUint16(data[16:18], 0)
	binary.BigEndian.PutUint16(data[18:20], 2)
	binary.BigEndian.PutUint16(data[20:22], 11)
	binary.BigEndian.PutUint16(data[22:24], 0)
	binary.BigEndian.PutUint16(data[24:26], 12)
	binary.BigEndian.PutUint16(data[26:28], 1)
	return data
}

func fakeCOLRv0Foreground() []byte {
	data := make([]byte, 14+6+4)
	binary.BigEndian.PutUint16(data[0:2], 0)
	binary.BigEndian.PutUint16(data[2:4], 1)
	binary.BigEndian.PutUint32(data[4:8], 14)
	binary.BigEndian.PutUint32(data[8:12], 20)
	binary.BigEndian.PutUint16(data[12:14], 1)
	binary.BigEndian.PutUint16(data[14:16], 10)
	binary.BigEndian.PutUint16(data[16:18], 0)
	binary.BigEndian.PutUint16(data[18:20], 1)
	binary.BigEndian.PutUint16(data[20:22], 11)
	binary.BigEndian.PutUint16(data[22:24], foregroundPaletteIndex)
	return data
}

func fakeCOLRv1PaintGlyphSolid() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 55)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)

	binary.BigEndian.PutUint32(data[34:38], 1)  // BaseGlyphList count
	binary.BigEndian.PutUint16(data[38:40], 10) // base glyph
	binary.BigEndian.PutUint32(data[40:44], 10) // paint offset from BaseGlyphList start

	data[44] = 10 // PaintGlyph
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11) // layer glyph
	data[50] = 2                                // PaintSolid
	binary.BigEndian.PutUint16(data[51:53], 0)  // red palette index
	binary.BigEndian.PutUint16(data[53:55], 0x4000)
	return data
}

func fakeCOLRv1PaintColrLayers() []byte {
	const (
		baseGlyphListOffset = 34
		basePaintOffset     = 44
		layerListOffset     = 50
		layer0Offset        = 62
		layer1Offset        = 73
	)
	data := make([]byte, 84)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[18:22], layerListOffset)

	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], basePaintOffset-baseGlyphListOffset)

	data[basePaintOffset] = 1 // PaintColrLayers
	data[basePaintOffset+1] = 2
	binary.BigEndian.PutUint32(data[basePaintOffset+2:basePaintOffset+6], 0)

	binary.BigEndian.PutUint32(data[layerListOffset:layerListOffset+4], 2)
	binary.BigEndian.PutUint32(data[layerListOffset+4:layerListOffset+8], layer0Offset-layerListOffset)
	binary.BigEndian.PutUint32(data[layerListOffset+8:layerListOffset+12], layer1Offset-layerListOffset)

	data[layer0Offset] = 10
	writeOffset24(data[layer0Offset+1:layer0Offset+4], 6)
	binary.BigEndian.PutUint16(data[layer0Offset+4:layer0Offset+6], 11)
	data[layer0Offset+6] = 2
	binary.BigEndian.PutUint16(data[layer0Offset+7:layer0Offset+9], 0)
	binary.BigEndian.PutUint16(data[layer0Offset+9:layer0Offset+11], 0x4000)

	data[layer1Offset] = 10
	writeOffset24(data[layer1Offset+1:layer1Offset+4], 6)
	binary.BigEndian.PutUint16(data[layer1Offset+4:layer1Offset+6], 12)
	data[layer1Offset+6] = 2
	binary.BigEndian.PutUint16(data[layer1Offset+7:layer1Offset+9], 1)
	binary.BigEndian.PutUint16(data[layer1Offset+9:layer1Offset+11], 0x2000)
	return data
}

func fakeCOLRv1PaintTranslate() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 63)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 14 // PaintTranslate
	writeOffset24(data[45:48], 8)
	putFWORD(data[48:50], 5)
	putFWORD(data[50:52], -3)

	data[52] = 10 // PaintGlyph
	writeOffset24(data[53:56], 6)
	binary.BigEndian.PutUint16(data[56:58], 11)
	data[58] = 2
	binary.BigEndian.PutUint16(data[59:61], 0)
	binary.BigEndian.PutUint16(data[61:63], 0x4000)
	return data
}

func fakeCOLRv1PaintTransform() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 91)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 12 // PaintTransform
	writeOffset24(data[45:48], 7)
	writeOffset24(data[48:51], 23)

	data[51] = 10 // PaintGlyph
	writeOffset24(data[52:55], 6)
	binary.BigEndian.PutUint16(data[55:57], 11)
	data[57] = 2
	binary.BigEndian.PutUint16(data[58:60], 0)
	binary.BigEndian.PutUint16(data[60:62], 0x4000)

	putFixed16Dot16(data[67:71], 2)
	putFixed16Dot16(data[71:75], 0)
	putFixed16Dot16(data[75:79], 0)
	putFixed16Dot16(data[79:83], 2)
	putFixed16Dot16(data[83:87], 5)
	putFixed16Dot16(data[87:91], -3)
	return data
}

func fakeCOLRv1PaintLinearGradient() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 81)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 10 // PaintGlyph
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)

	data[50] = 4 // PaintLinearGradient
	writeOffset24(data[51:54], 16)
	putFWORD(data[54:56], 0)
	putFWORD(data[56:58], 0)
	putFWORD(data[58:60], 10)
	putFWORD(data[60:62], 0)
	putFWORD(data[62:64], 10)
	putFWORD(data[64:66], 1)

	data[66] = 0 // Extend pad
	binary.BigEndian.PutUint16(data[67:69], 2)
	binary.BigEndian.PutUint16(data[69:71], 0)
	binary.BigEndian.PutUint16(data[71:73], 0)
	binary.BigEndian.PutUint16(data[73:75], 0x4000)
	binary.BigEndian.PutUint16(data[75:77], 0x4000)
	binary.BigEndian.PutUint16(data[77:79], 1)
	binary.BigEndian.PutUint16(data[79:81], 0x4000)
	return data
}

func fakeCOLRv1PaintRadialGradient() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 81)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 10 // PaintGlyph
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)

	data[50] = 6 // PaintRadialGradient
	writeOffset24(data[51:54], 16)
	putFWORD(data[54:56], 10)
	putFWORD(data[56:58], 0)
	binary.BigEndian.PutUint16(data[58:60], 0)
	putFWORD(data[60:62], 10)
	putFWORD(data[62:64], 0)
	binary.BigEndian.PutUint16(data[64:66], 10)

	data[66] = 0
	binary.BigEndian.PutUint16(data[67:69], 2)
	binary.BigEndian.PutUint16(data[69:71], 0)
	binary.BigEndian.PutUint16(data[71:73], 0)
	binary.BigEndian.PutUint16(data[73:75], 0x4000)
	binary.BigEndian.PutUint16(data[75:77], 0x4000)
	binary.BigEndian.PutUint16(data[77:79], 1)
	binary.BigEndian.PutUint16(data[79:81], 0x4000)
	return data
}

func fakeCOLRv1PaintSweepGradient() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 77)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)

	data[44] = 10 // PaintGlyph
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)

	data[50] = 8 // PaintSweepGradient
	writeOffset24(data[51:54], 12)
	putFWORD(data[54:56], 10)
	putFWORD(data[56:58], -6)
	binary.BigEndian.PutUint16(data[58:60], 0)
	binary.BigEndian.PutUint16(data[60:62], 0x4000)

	data[62] = 0
	binary.BigEndian.PutUint16(data[63:65], 2)
	binary.BigEndian.PutUint16(data[65:67], 0)
	binary.BigEndian.PutUint16(data[67:69], 0)
	binary.BigEndian.PutUint16(data[69:71], 0x4000)
	binary.BigEndian.PutUint16(data[71:73], 0x4000)
	binary.BigEndian.PutUint16(data[73:75], 1)
	binary.BigEndian.PutUint16(data[75:77], 0x4000)
	return data
}

func putFixed16Dot16(data []byte, value float64) {
	binary.BigEndian.PutUint32(data, uint32(int32(math.Round(value*65536))))
}

func putFWORD(data []byte, value int16) {
	binary.BigEndian.PutUint16(data, uint16(value))
}

func fakeCPAL() []byte {
	data := make([]byte, 12+2+8)
	binary.BigEndian.PutUint16(data[0:2], 0)   // version
	binary.BigEndian.PutUint16(data[2:4], 2)   // palette entries
	binary.BigEndian.PutUint16(data[4:6], 1)   // palettes
	binary.BigEndian.PutUint16(data[6:8], 2)   // color records
	binary.BigEndian.PutUint32(data[8:12], 14) // color offset
	binary.BigEndian.PutUint16(data[12:14], 0) // first color record
	data[14] = 0
	data[15] = 0
	data[16] = 255
	data[17] = 255 // red BGRA
	data[18] = 0
	data[19] = 255
	data[20] = 0
	data[21] = 255 // green BGRA
	return data
}
