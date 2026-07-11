package fontglyph

import (
	"encoding/binary"
	"errors"
	"image/color"
	"math"
)

var (
	ErrNoCOLRTable         = errors.New("fontglyph: font has no COLR table")
	ErrNoCPALTable         = errors.New("fontglyph: font has no CPAL table")
	ErrInvalidCOLRTable    = errors.New("fontglyph: invalid COLR table")
	ErrInvalidCPALTable    = errors.New("fontglyph: invalid CPAL table")
	ErrUnsupportedCOLR     = errors.New("fontglyph: unsupported COLR version")
	ErrCOLRGlyphNotFound   = errors.New("fontglyph: COLR glyph not found")
	foregroundPaletteIndex = uint16(0xFFFF)
)

func newCOLRParser(colrData, cpalData []byte) (*colrParser, error) {
	if len(colrData) == 0 {
		return nil, ErrNoCOLRTable
	}
	if len(cpalData) == 0 {
		return nil, ErrNoCPALTable
	}
	palettes, err := parseCPAL(cpalData)
	if err != nil {
		return nil, err
	}
	version, baseGlyphs, layers, basePaints, layerPaints, err := parseCOLR(colrData)
	if err != nil {
		return nil, err
	}
	var variationStore *colrVariationStore
	if version == 1 && len(colrData) >= 34 {
		variationStoreOffset := int(binary.BigEndian.Uint32(colrData[30:34]))
		variationStore, err = parseCOLRVariationStore(colrData, variationStoreOffset)
		if err != nil {
			return nil, err
		}
	}
	return &colrParser{
		data:           colrData,
		version:        version,
		baseGlyphs:     baseGlyphs,
		layers:         layers,
		basePaints:     basePaints,
		layerPaints:    layerPaints,
		palettes:       palettes,
		variationStore: variationStore,
	}, nil
}

func (p *colrParser) glyph(glyphID uint16, paletteIndex int) (COLRGlyph, error) {
	if paletteIndex < 0 || paletteIndex >= len(p.palettes) {
		paletteIndex = 0
	}
	palette := p.palettes[paletteIndex]
	if p.version == 1 {
		if glyph, err := p.glyphV1(glyphID, palette); err == nil {
			return glyph, nil
		} else if !errors.Is(err, ErrCOLRGlyphNotFound) {
			return COLRGlyph{}, err
		}
	}
	return p.glyphV0(glyphID, palette)
}

func (p *colrParser) glyphV0(glyphID uint16, palette []color.RGBA) (COLRGlyph, error) {
	for _, base := range p.baseGlyphs {
		if base.glyphID != glyphID {
			continue
		}
		end := int(base.firstLayer + base.numLayers)
		if int(base.firstLayer) > len(p.layers) || end > len(p.layers) {
			return COLRGlyph{}, ErrInvalidCOLRTable
		}
		glyph := COLRGlyph{GlyphID: glyphID, Layers: make([]COLRLayer, 0, base.numLayers)}
		for _, layer := range p.layers[base.firstLayer:end] {
			out, err := p.layerFromPalette(layer.glyphID, layer.paletteIndex, palette)
			if err != nil {
				return COLRGlyph{}, err
			}
			glyph.Layers = append(glyph.Layers, out)
		}
		return glyph, nil
	}
	return COLRGlyph{}, ErrCOLRGlyphNotFound
}

func (p *colrParser) glyphV1(glyphID uint16, palette []color.RGBA) (COLRGlyph, error) {
	for _, base := range p.basePaints {
		if base.glyphID != glyphID {
			continue
		}
		layers, err := p.parseV1Paint(int(base.paintOffset), palette, 0, false, 0, identityCOLRTransform())
		if err != nil {
			return COLRGlyph{}, err
		}
		return COLRGlyph{GlyphID: glyphID, Layers: layers}, nil
	}
	return COLRGlyph{}, ErrCOLRGlyphNotFound
}

func (p *colrParser) parseV1Paint(paintOffset int, palette []color.RGBA, depth int, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	if depth > 32 || paintOffset < 0 || paintOffset >= len(p.data) {
		return nil, ErrInvalidCOLRTable
	}
	switch p.data[paintOffset] {
	case 1: // PaintColrLayers: format, numLayers, firstLayerIndex.
		if paintOffset+6 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		numLayers := int(p.data[paintOffset+1])
		firstLayerIndex := int(binary.BigEndian.Uint32(p.data[paintOffset+2 : paintOffset+6]))
		if firstLayerIndex < 0 || firstLayerIndex+numLayers > len(p.layerPaints) {
			return nil, ErrInvalidCOLRTable
		}
		var layers []COLRLayer
		for _, layerPaint := range p.layerPaints[firstLayerIndex : firstLayerIndex+numLayers] {
			childLayers, err := p.parseV1Paint(int(layerPaint), palette, depth+1, hasGlyph, glyphID, transform)
			if err != nil {
				return nil, err
			}
			layers = append(layers, childLayers...)
		}
		return layers, nil
	case 2: // PaintSolid: format, paletteIndex, alpha.
		if paintOffset+5 > len(p.data) || !hasGlyph {
			return nil, ErrInvalidCOLRTable
		}
		paletteIndex := binary.BigEndian.Uint16(p.data[paintOffset+1 : paintOffset+3])
		layer, err := p.layerFromPalette(glyphID, paletteIndex, palette)
		if err != nil {
			return nil, err
		}
		layer.Transform = transform
		if !layer.Foreground {
			layer.Color.A = scaleAlpha(layer.Color.A, binary.BigEndian.Uint16(p.data[paintOffset+3:paintOffset+5]))
		}
		return []COLRLayer{layer}, nil
	case 4: // PaintLinearGradient.
		return p.parseV1LinearGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 6: // PaintRadialGradient.
		return p.parseV1RadialGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 3, 5, 7, 9, 13, 15, 17, 19, 21, 23, 25, 27, 29, 31: // PaintVar* values with ItemVariationStore deltas when variation coords are available.
		return p.parseV1VariablePaint(paintOffset, palette, depth, hasGlyph, glyphID, transform)
	case 8: // PaintSweepGradient.
		return p.parseV1SweepGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 10: // PaintGlyph: format, paintOffset, glyphID.
		if paintOffset+6 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		childGlyphID := binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6])
		return p.parseV1Paint(childOffset, palette, depth+1, true, childGlyphID, transform)
	case 11: // PaintColrGlyph: format, glyphID.
		if paintOffset+3 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childGlyphID := binary.BigEndian.Uint16(p.data[paintOffset+1 : paintOffset+3])
		for _, base := range p.basePaints {
			if base.glyphID == childGlyphID {
				return p.parseV1Paint(int(base.paintOffset), palette, depth+1, false, 0, transform)
			}
		}
		return nil, ErrCOLRGlyphNotFound
	case 12: // PaintTransform: format, paintOffset, transformOffset.
		if paintOffset+7 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		matrixOffset := paintOffset + int(readOffset24(p.data[paintOffset+4:paintOffset+7]))
		matrix, ok := p.readAffine2x3(matrixOffset)
		if !ok {
			return nil, ErrInvalidCOLRTable
		}
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(matrix))
	case 14: // PaintTranslate: format, paintOffset, dx, dy.
		if paintOffset+8 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		dx := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6])))
		dy := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8])))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(translateCOLR(dx, dy)))
	case 16: // PaintScale: format, paintOffset, scaleX, scaleY.
		if paintOffset+8 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		sx := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))
		sy := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8]))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(scaleCOLR(sx, sy)))
	case 18: // PaintScaleAroundCenter: format, paintOffset, scaleX, scaleY, centerX, centerY.
		if paintOffset+12 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		sx := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))
		sy := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8]))
		cx := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])))
		cy := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10 : paintOffset+12])))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(scaleCOLR(sx, sy), cx, cy)))
	case 20: // PaintScaleUniform: format, paintOffset, scale.
		if paintOffset+6 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		scale := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(scaleCOLR(scale, scale)))
	case 22: // PaintScaleUniformAroundCenter: format, paintOffset, scale, centerX, centerY.
		if paintOffset+10 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		scale := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))
		cx := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8])))
		cy := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(scaleCOLR(scale, scale), cx, cy)))
	case 24: // PaintRotate: format, paintOffset, angle.
		if paintOffset+6 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		angle := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(rotateCOLR(angle)))
	case 26: // PaintRotateAroundCenter: format, paintOffset, angle, centerX, centerY.
		if paintOffset+10 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		angle := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		cx := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8])))
		cy := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(rotateCOLR(angle), cx, cy)))
	case 28: // PaintSkew: format, paintOffset, xSkewAngle, ySkewAngle.
		if paintOffset+8 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		xSkew := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		ySkew := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8])) * math.Pi
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(skewCOLR(xSkew, ySkew)))
	case 30: // PaintSkewAroundCenter: format, paintOffset, xSkewAngle, ySkewAngle, centerX, centerY.
		if paintOffset+12 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		xSkew := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		ySkew := f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8])) * math.Pi
		cx := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])))
		cy := float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10 : paintOffset+12])))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(skewCOLR(xSkew, ySkew), cx, cy)))
	case 32: // PaintComposite.
		return p.parseV1Composite(paintOffset, palette, depth, hasGlyph, glyphID, transform)
	default:
		return nil, ErrUnsupportedCOLR
	}
}

func (p *colrParser) layerFromPalette(glyphID, paletteIndex uint16, palette []color.RGBA) (COLRLayer, error) {
	out := COLRLayer{GlyphID: glyphID, PaletteIndex: paletteIndex, Transform: identityCOLRTransform()}
	if paletteIndex == foregroundPaletteIndex {
		out.Foreground = true
		return out, nil
	}
	if int(paletteIndex) >= len(palette) {
		return COLRLayer{}, ErrInvalidCPALTable
	}
	out.Color = palette[paletteIndex]
	return out, nil
}

func (p *colrParser) readAffine2x3(offset int) (COLRTransform, bool) {
	if offset < 0 || offset+24 > len(p.data) {
		return COLRTransform{}, false
	}
	return COLRTransform{
		XX: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset : offset+4])),
		YX: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset+4 : offset+8])),
		XY: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset+8 : offset+12])),
		YY: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset+12 : offset+16])),
		DX: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset+16 : offset+20])),
		DY: fixed16Dot16(binary.BigEndian.Uint32(p.data[offset+20 : offset+24])),
	}, true
}

func parseCOLR(data []byte) (version uint16, baseGlyphs []colrBaseGlyph, layers []colrLayerRecord, basePaints []colrBaseGlyphPaint, layerPaints []uint32, err error) {
	if len(data) < 14 {
		return 0, nil, nil, nil, nil, ErrInvalidCOLRTable
	}
	version = binary.BigEndian.Uint16(data[0:2])
	if version > 1 {
		return 0, nil, nil, nil, nil, ErrUnsupportedCOLR
	}
	baseGlyphs, layers, err = parseCOLRv0Records(data)
	if err != nil {
		return 0, nil, nil, nil, nil, err
	}
	if version == 1 {
		basePaints, layerPaints, err = parseCOLRv1Records(data)
		if err != nil {
			return 0, nil, nil, nil, nil, err
		}
	}
	return version, baseGlyphs, layers, basePaints, layerPaints, nil
}

func parseCOLRv0(data []byte) ([]colrBaseGlyph, []colrLayerRecord, error) {
	if len(data) >= 2 && binary.BigEndian.Uint16(data[0:2]) != 0 {
		return nil, nil, ErrUnsupportedCOLR
	}
	return parseCOLRv0Records(data)
}

func parseCOLRv0Records(data []byte) ([]colrBaseGlyph, []colrLayerRecord, error) {
	if len(data) < 14 {
		return nil, nil, ErrInvalidCOLRTable
	}
	numBase := int(binary.BigEndian.Uint16(data[2:4]))
	baseOffset := int(binary.BigEndian.Uint32(data[4:8]))
	layerOffset := int(binary.BigEndian.Uint32(data[8:12]))
	numLayers := int(binary.BigEndian.Uint16(data[12:14]))
	if (numBase > 0 && (baseOffset <= 0 || baseOffset+numBase*6 > len(data))) || (numLayers > 0 && (layerOffset <= 0 || layerOffset+numLayers*4 > len(data))) {
		return nil, nil, ErrInvalidCOLRTable
	}
	baseGlyphs := make([]colrBaseGlyph, numBase)
	for i := 0; i < numBase; i++ {
		o := baseOffset + i*6
		baseGlyphs[i] = colrBaseGlyph{
			glyphID:    binary.BigEndian.Uint16(data[o : o+2]),
			firstLayer: binary.BigEndian.Uint16(data[o+2 : o+4]),
			numLayers:  binary.BigEndian.Uint16(data[o+4 : o+6]),
		}
	}
	layers := make([]colrLayerRecord, numLayers)
	for i := 0; i < numLayers; i++ {
		o := layerOffset + i*4
		layers[i] = colrLayerRecord{
			glyphID:      binary.BigEndian.Uint16(data[o : o+2]),
			paletteIndex: binary.BigEndian.Uint16(data[o+2 : o+4]),
		}
	}
	return baseGlyphs, layers, nil
}

func parseCOLRv1Records(data []byte) ([]colrBaseGlyphPaint, []uint32, error) {
	if len(data) < 34 {
		return nil, nil, ErrInvalidCOLRTable
	}
	baseGlyphListOffset := int(binary.BigEndian.Uint32(data[14:18]))
	layerListOffset := int(binary.BigEndian.Uint32(data[18:22]))
	basePaints, err := parseBaseGlyphList(data, baseGlyphListOffset)
	if err != nil {
		return nil, nil, err
	}
	layerPaints, err := parseLayerList(data, layerListOffset)
	if err != nil {
		return nil, nil, err
	}
	return basePaints, layerPaints, nil
}

func parseBaseGlyphList(data []byte, offset int) ([]colrBaseGlyphPaint, error) {
	if offset == 0 {
		return nil, nil
	}
	if offset < 0 || offset+4 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	count := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	if offset+4+count*6 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	paints := make([]colrBaseGlyphPaint, count)
	for i := 0; i < count; i++ {
		o := offset + 4 + i*6
		paintOffset := offset + int(binary.BigEndian.Uint32(data[o+2:o+6]))
		if paintOffset < offset || paintOffset >= len(data) {
			return nil, ErrInvalidCOLRTable
		}
		paints[i] = colrBaseGlyphPaint{glyphID: binary.BigEndian.Uint16(data[o : o+2]), paintOffset: uint32(paintOffset)}
	}
	return paints, nil
}

func parseLayerList(data []byte, offset int) ([]uint32, error) {
	if offset == 0 {
		return nil, nil
	}
	if offset < 0 || offset+4 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	count := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	if offset+4+count*4 > len(data) {
		return nil, ErrInvalidCOLRTable
	}
	paints := make([]uint32, count)
	for i := 0; i < count; i++ {
		paintOffset := offset + int(binary.BigEndian.Uint32(data[offset+4+i*4:offset+8+i*4]))
		if paintOffset < offset || paintOffset >= len(data) {
			return nil, ErrInvalidCOLRTable
		}
		paints[i] = uint32(paintOffset)
	}
	return paints, nil
}

func parseCPAL(data []byte) ([][]color.RGBA, error) {
	if len(data) < 12 {
		return nil, ErrInvalidCPALTable
	}
	version := binary.BigEndian.Uint16(data[0:2])
	if version > 1 {
		return nil, ErrInvalidCPALTable
	}
	numPaletteEntries := int(binary.BigEndian.Uint16(data[2:4]))
	numPalettes := int(binary.BigEndian.Uint16(data[4:6]))
	numColorRecords := int(binary.BigEndian.Uint16(data[6:8]))
	colorRecordsOffset := int(binary.BigEndian.Uint32(data[8:12]))
	if 12+numPalettes*2 > len(data) || colorRecordsOffset+numColorRecords*4 > len(data) {
		return nil, ErrInvalidCPALTable
	}
	palettes := make([][]color.RGBA, numPalettes)
	for p := 0; p < numPalettes; p++ {
		first := int(binary.BigEndian.Uint16(data[12+p*2 : 14+p*2]))
		if first+numPaletteEntries > numColorRecords {
			return nil, ErrInvalidCPALTable
		}
		palette := make([]color.RGBA, numPaletteEntries)
		for i := 0; i < numPaletteEntries; i++ {
			o := colorRecordsOffset + (first+i)*4
			palette[i] = color.RGBA{B: data[o], G: data[o+1], R: data[o+2], A: data[o+3]}
		}
		palettes[p] = palette
	}
	return palettes, nil
}

func readOffset24(data []byte) uint32 {
	return uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
}

func writeOffset24(data []byte, value uint32) {
	data[0] = byte(value >> 16)
	data[1] = byte(value >> 8)
	data[2] = byte(value)
}

func scaleAlpha(base uint8, alphaF2Dot14 uint16) uint8 {
	alpha := math.Max(0, math.Min(1, float64(int16(alphaF2Dot14))/16384.0))
	return uint8(math.Round(float64(base) * alpha))
}
