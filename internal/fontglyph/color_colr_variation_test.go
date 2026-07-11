package fontglyph

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func TestCOLRParserExtractsV1PaintVarSolid(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarSolid(), fakeCPAL())
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
	if layer.GlyphID != 11 || layer.Color != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("unexpected var solid layer: %#v", layer)
	}
}

func TestCOLRParserExtractsV1PaintVarTranslate(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarTranslate(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	want := COLRTransform{XX: 1, YY: 1, DX: 5, DY: -3}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestCOLRParserAppliesV1PaintVarTranslateDeltas(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarTranslateWithStore(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	want := COLRTransform{XX: 1, YY: 1, DX: 12, DY: -5}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestCOLRParserAppliesV1PaintVarScaleDeltas(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarScaleWithStore(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	want := COLRTransform{XX: 1.5, YY: 0.75}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestCOLRParserAppliesV1PaintVarLinearGradientDeltas(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarLinearGradientWithStore(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	gradient := glyph.Layers[0].LinearGradient
	if gradient.X0 != 4 || gradient.Y0 != 1 || gradient.X1 != 12 || gradient.Y1 != -1 || gradient.X2 != 10 || gradient.Y2 != 2 {
		t.Fatalf("unexpected varied linear gradient coords: %#v", gradient)
	}
}

func TestCOLRParserAppliesV1PaintVarRadialGradientDeltas(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarRadialGradientWithStore(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	gradient := glyph.Layers[0].RadialGradient
	if gradient.X0 != 14 || gradient.Y0 != 1 || gradient.Radius0 != 2 || gradient.X1 != 11 || gradient.Y1 != -1 || gradient.Radius1 != 13 {
		t.Fatalf("unexpected varied radial gradient coords: %#v", gradient)
	}
}

func TestCOLRParserExtractsV1PaintVarLinearGradient(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarLinearGradient(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	layer := glyph.Layers[0]
	if layer.Fill != COLRFillLinearGradient || len(layer.LinearGradient.Stops) != 2 {
		t.Fatalf("unexpected var linear gradient layer: %#v", layer)
	}
}

func fakeCOLRv1PaintVarSolid() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 59)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 10
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)
	data[50] = 3 // PaintVarSolid
	binary.BigEndian.PutUint16(data[51:53], 0)
	binary.BigEndian.PutUint16(data[53:55], 0x4000)
	binary.BigEndian.PutUint32(data[55:59], 0xFFFFFFFF)
	return data
}

func fakeCOLRv1PaintVarTranslate() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 67)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 15 // PaintVarTranslate
	writeOffset24(data[45:48], 12)
	putFWORD(data[48:50], 5)
	putFWORD(data[50:52], -3)
	binary.BigEndian.PutUint32(data[52:56], 0xFFFFFFFF)
	data[56] = 10
	writeOffset24(data[57:60], 6)
	binary.BigEndian.PutUint16(data[60:62], 11)
	data[62] = 2
	binary.BigEndian.PutUint16(data[63:65], 0)
	binary.BigEndian.PutUint16(data[65:67], 0x4000)
	return data
}

func fakeCOLRv1PaintVarTranslateWithStore() []byte {
	data := fakeCOLRv1PaintVarTranslate()
	const varStoreOffset = 67
	data = append(data, make([]byte, 34)...)
	binary.BigEndian.PutUint32(data[30:34], varStoreOffset)
	binary.BigEndian.PutUint32(data[52:56], 0)

	binary.BigEndian.PutUint16(data[varStoreOffset:varStoreOffset+2], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+2:varStoreOffset+6], 12)
	binary.BigEndian.PutUint16(data[varStoreOffset+6:varStoreOffset+8], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+8:varStoreOffset+12], 22)

	regionOffset := varStoreOffset + 12
	binary.BigEndian.PutUint16(data[regionOffset:regionOffset+2], 1)
	binary.BigEndian.PutUint16(data[regionOffset+2:regionOffset+4], 1)
	binary.BigEndian.PutUint16(data[regionOffset+4:regionOffset+6], 0)
	binary.BigEndian.PutUint16(data[regionOffset+6:regionOffset+8], 0x2000)
	binary.BigEndian.PutUint16(data[regionOffset+8:regionOffset+10], 0x4000)

	dataOffset := varStoreOffset + 22
	binary.BigEndian.PutUint16(data[dataOffset:dataOffset+2], 2)
	binary.BigEndian.PutUint16(data[dataOffset+2:dataOffset+4], 1)
	binary.BigEndian.PutUint16(data[dataOffset+4:dataOffset+6], 1)
	binary.BigEndian.PutUint16(data[dataOffset+6:dataOffset+8], 0)
	putFWORD(data[dataOffset+8:dataOffset+10], 7)
	putFWORD(data[dataOffset+10:dataOffset+12], -2)
	return data
}

func fakeCOLRv1PaintVarScaleWithStore() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 67)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 17 // PaintVarScale
	writeOffset24(data[45:48], 12)
	binary.BigEndian.PutUint16(data[48:50], 0x4000)
	binary.BigEndian.PutUint16(data[50:52], 0x4000)
	binary.BigEndian.PutUint32(data[52:56], 0)
	data[56] = 10
	writeOffset24(data[57:60], 6)
	binary.BigEndian.PutUint16(data[60:62], 11)
	data[62] = 2
	binary.BigEndian.PutUint16(data[63:65], 0)
	binary.BigEndian.PutUint16(data[65:67], 0x4000)

	const varStoreOffset = 67
	data = append(data, make([]byte, 34)...)
	binary.BigEndian.PutUint32(data[30:34], varStoreOffset)
	binary.BigEndian.PutUint16(data[varStoreOffset:varStoreOffset+2], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+2:varStoreOffset+6], 12)
	binary.BigEndian.PutUint16(data[varStoreOffset+6:varStoreOffset+8], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+8:varStoreOffset+12], 22)

	regionOffset := varStoreOffset + 12
	binary.BigEndian.PutUint16(data[regionOffset:regionOffset+2], 1)
	binary.BigEndian.PutUint16(data[regionOffset+2:regionOffset+4], 1)
	binary.BigEndian.PutUint16(data[regionOffset+4:regionOffset+6], 0)
	binary.BigEndian.PutUint16(data[regionOffset+6:regionOffset+8], 0x2000)
	binary.BigEndian.PutUint16(data[regionOffset+8:regionOffset+10], 0x4000)

	dataOffset := varStoreOffset + 22
	binary.BigEndian.PutUint16(data[dataOffset:dataOffset+2], 2)
	binary.BigEndian.PutUint16(data[dataOffset+2:dataOffset+4], 1)
	binary.BigEndian.PutUint16(data[dataOffset+4:dataOffset+6], 1)
	binary.BigEndian.PutUint16(data[dataOffset+6:dataOffset+8], 0)
	putFWORD(data[dataOffset+8:dataOffset+10], 0x2000)
	putFWORD(data[dataOffset+10:dataOffset+12], -0x1000)
	return data
}

func fakeCOLRv1PaintVarLinearGradient() []byte {
	const baseGlyphListOffset = 34
	data := make([]byte, 85)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], baseGlyphListOffset)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 10
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)
	data[50] = 5 // PaintVarLinearGradient
	writeOffset24(data[51:54], 20)
	putFWORD(data[54:56], 0)
	putFWORD(data[56:58], 0)
	putFWORD(data[58:60], 10)
	putFWORD(data[60:62], 0)
	putFWORD(data[62:64], 10)
	putFWORD(data[64:66], 1)
	binary.BigEndian.PutUint32(data[66:70], 0xFFFFFFFF)
	data[70] = 0
	binary.BigEndian.PutUint16(data[71:73], 2)
	binary.BigEndian.PutUint16(data[73:75], 0)
	binary.BigEndian.PutUint16(data[75:77], 0)
	binary.BigEndian.PutUint16(data[77:79], 0x4000)
	binary.BigEndian.PutUint16(data[79:81], 0x4000)
	binary.BigEndian.PutUint16(data[81:83], 1)
	binary.BigEndian.PutUint16(data[83:85], 0x4000)
	return data
}

func fakeCOLRv1PaintVarLinearGradientWithStore() []byte {
	data := fakeCOLRv1PaintVarLinearGradient()
	const varStoreOffset = 85
	data = append(data, make([]byte, 42)...)
	binary.BigEndian.PutUint32(data[30:34], varStoreOffset)
	binary.BigEndian.PutUint32(data[66:70], 0)

	binary.BigEndian.PutUint16(data[varStoreOffset:varStoreOffset+2], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+2:varStoreOffset+6], 12)
	binary.BigEndian.PutUint16(data[varStoreOffset+6:varStoreOffset+8], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+8:varStoreOffset+12], 22)

	regionOffset := varStoreOffset + 12
	binary.BigEndian.PutUint16(data[regionOffset:regionOffset+2], 1)
	binary.BigEndian.PutUint16(data[regionOffset+2:regionOffset+4], 1)
	binary.BigEndian.PutUint16(data[regionOffset+4:regionOffset+6], 0)
	binary.BigEndian.PutUint16(data[regionOffset+6:regionOffset+8], 0x2000)
	binary.BigEndian.PutUint16(data[regionOffset+8:regionOffset+10], 0x4000)

	dataOffset := varStoreOffset + 22
	binary.BigEndian.PutUint16(data[dataOffset:dataOffset+2], 6)
	binary.BigEndian.PutUint16(data[dataOffset+2:dataOffset+4], 1)
	binary.BigEndian.PutUint16(data[dataOffset+4:dataOffset+6], 1)
	binary.BigEndian.PutUint16(data[dataOffset+6:dataOffset+8], 0)
	putFWORD(data[dataOffset+8:dataOffset+10], 4)
	putFWORD(data[dataOffset+10:dataOffset+12], 1)
	putFWORD(data[dataOffset+12:dataOffset+14], 2)
	putFWORD(data[dataOffset+14:dataOffset+16], -1)
	putFWORD(data[dataOffset+16:dataOffset+18], 0)
	putFWORD(data[dataOffset+18:dataOffset+20], 1)
	return data
}

func fakeCOLRv1PaintVarRadialGradientWithStore() []byte {
	data := make([]byte, 127)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], 34)
	binary.BigEndian.PutUint32(data[30:34], 85)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 10
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)
	data[50] = 7 // PaintVarRadialGradient
	writeOffset24(data[51:54], 20)
	putFWORD(data[54:56], 10)
	putFWORD(data[56:58], 0)
	binary.BigEndian.PutUint16(data[58:60], 0)
	putFWORD(data[60:62], 10)
	putFWORD(data[62:64], 0)
	binary.BigEndian.PutUint16(data[64:66], 10)
	binary.BigEndian.PutUint32(data[66:70], 0)
	data[70] = 0
	binary.BigEndian.PutUint16(data[71:73], 2)
	binary.BigEndian.PutUint16(data[73:75], 0)
	binary.BigEndian.PutUint16(data[75:77], 0)
	binary.BigEndian.PutUint16(data[77:79], 0x4000)
	binary.BigEndian.PutUint16(data[79:81], 0x4000)
	binary.BigEndian.PutUint16(data[81:83], 1)
	binary.BigEndian.PutUint16(data[83:85], 0x4000)

	const varStoreOffset = 85
	binary.BigEndian.PutUint16(data[varStoreOffset:varStoreOffset+2], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+2:varStoreOffset+6], 12)
	binary.BigEndian.PutUint16(data[varStoreOffset+6:varStoreOffset+8], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+8:varStoreOffset+12], 22)

	regionOffset := varStoreOffset + 12
	binary.BigEndian.PutUint16(data[regionOffset:regionOffset+2], 1)
	binary.BigEndian.PutUint16(data[regionOffset+2:regionOffset+4], 1)
	binary.BigEndian.PutUint16(data[regionOffset+4:regionOffset+6], 0)
	binary.BigEndian.PutUint16(data[regionOffset+6:regionOffset+8], 0x2000)
	binary.BigEndian.PutUint16(data[regionOffset+8:regionOffset+10], 0x4000)

	dataOffset := varStoreOffset + 22
	binary.BigEndian.PutUint16(data[dataOffset:dataOffset+2], 6)
	binary.BigEndian.PutUint16(data[dataOffset+2:dataOffset+4], 1)
	binary.BigEndian.PutUint16(data[dataOffset+4:dataOffset+6], 1)
	binary.BigEndian.PutUint16(data[dataOffset+6:dataOffset+8], 0)
	putFWORD(data[dataOffset+8:dataOffset+10], 4)
	putFWORD(data[dataOffset+10:dataOffset+12], 1)
	putFWORD(data[dataOffset+12:dataOffset+14], 2)
	putFWORD(data[dataOffset+14:dataOffset+16], 1)
	putFWORD(data[dataOffset+16:dataOffset+18], -1)
	putFWORD(data[dataOffset+18:dataOffset+20], 3)
	return data
}

func TestCOLRParserAppliesV1VarColorLineStopDeltas(t *testing.T) {
	parser, err := newCOLRParser(fakeCOLRv1PaintVarLinearGradientWithVarColorLine(), fakeCPAL())
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	stops := glyph.Layers[0].LinearGradient.Stops
	if len(stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(stops))
	}
	if stops[0].Offset != 0.25 || stops[0].Color.A >= 255 || stops[1].Offset != 0.75 {
		t.Fatalf("unexpected varied stops: %#v", stops)
	}
}

func fakeCOLRv1PaintVarLinearGradientWithVarColorLine() []byte {
	data := make([]byte, 135)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[14:18], 34)
	binary.BigEndian.PutUint32(data[30:34], 93)
	binary.BigEndian.PutUint32(data[34:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint32(data[40:44], 10)
	data[44] = 10
	writeOffset24(data[45:48], 6)
	binary.BigEndian.PutUint16(data[48:50], 11)
	data[50] = 5 // PaintVarLinearGradient
	writeOffset24(data[51:54], 20)
	putFWORD(data[54:56], 0)
	putFWORD(data[56:58], 0)
	putFWORD(data[58:60], 10)
	putFWORD(data[60:62], 0)
	putFWORD(data[62:64], 10)
	putFWORD(data[64:66], 1)
	binary.BigEndian.PutUint32(data[66:70], 0xFFFFFFFF)

	data[70] = 0
	binary.BigEndian.PutUint16(data[71:73], 2)
	binary.BigEndian.PutUint16(data[73:75], 0)
	binary.BigEndian.PutUint16(data[75:77], 0)
	binary.BigEndian.PutUint16(data[77:79], 0x4000)
	binary.BigEndian.PutUint32(data[79:83], 0)
	binary.BigEndian.PutUint16(data[83:85], 0x4000)
	binary.BigEndian.PutUint16(data[85:87], 1)
	binary.BigEndian.PutUint16(data[87:89], 0x4000)
	binary.BigEndian.PutUint32(data[89:93], 2)

	const varStoreOffset = 93
	binary.BigEndian.PutUint16(data[varStoreOffset:varStoreOffset+2], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+2:varStoreOffset+6], 12)
	binary.BigEndian.PutUint16(data[varStoreOffset+6:varStoreOffset+8], 1)
	binary.BigEndian.PutUint32(data[varStoreOffset+8:varStoreOffset+12], 22)

	regionOffset := varStoreOffset + 12
	binary.BigEndian.PutUint16(data[regionOffset:regionOffset+2], 1)
	binary.BigEndian.PutUint16(data[regionOffset+2:regionOffset+4], 1)
	binary.BigEndian.PutUint16(data[regionOffset+4:regionOffset+6], 0)
	binary.BigEndian.PutUint16(data[regionOffset+6:regionOffset+8], 0x2000)
	binary.BigEndian.PutUint16(data[regionOffset+8:regionOffset+10], 0x4000)

	dataOffset := varStoreOffset + 22
	binary.BigEndian.PutUint16(data[dataOffset:dataOffset+2], 4)
	binary.BigEndian.PutUint16(data[dataOffset+2:dataOffset+4], 1)
	binary.BigEndian.PutUint16(data[dataOffset+4:dataOffset+6], 1)
	binary.BigEndian.PutUint16(data[dataOffset+6:dataOffset+8], 0)
	putFWORD(data[dataOffset+8:dataOffset+10], 0x1000)
	putFWORD(data[dataOffset+10:dataOffset+12], -0x1000)
	putFWORD(data[dataOffset+12:dataOffset+14], -0x1000)
	putFWORD(data[dataOffset+14:dataOffset+16], 0)
	return data
}
