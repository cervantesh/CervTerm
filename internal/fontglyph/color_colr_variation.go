package fontglyph

import (
	"encoding/binary"
	"image/color"
	"math"
)

func (p *colrParser) parseV1VariablePaint(paintOffset int, palette []color.RGBA, depth int, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	switch p.data[paintOffset] {
	case 3: // PaintVarSolid: same base fields as PaintSolid plus VarIdxBase.
		if paintOffset+9 > len(p.data) || !hasGlyph {
			return nil, ErrInvalidCOLRTable
		}
		paletteIndex := binary.BigEndian.Uint16(p.data[paintOffset+1 : paintOffset+3])
		layer, err := p.layerFromPalette(glyphID, paletteIndex, palette)
		if err != nil {
			return nil, err
		}
		layer.Transform = transform
		if !layer.Foreground {
			alpha := binary.BigEndian.Uint16(p.data[paintOffset+3 : paintOffset+5])
			varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+5 : paintOffset+9])
			alpha = p.applyVarUint16(varIdxBase, 0, alpha, 0, 0x4000)
			layer.Color.A = scaleAlpha(layer.Color.A, alpha)
		}
		return []COLRLayer{layer}, nil
	case 5: // PaintVarLinearGradient.
		if paintOffset+20 > len(p.data) || !hasGlyph {
			return nil, ErrInvalidCOLRTable
		}
		return p.parseV1LinearGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 7: // PaintVarRadialGradient.
		if paintOffset+20 > len(p.data) || !hasGlyph {
			return nil, ErrInvalidCOLRTable
		}
		return p.parseV1RadialGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 9: // PaintVarSweepGradient.
		if paintOffset+16 > len(p.data) || !hasGlyph {
			return nil, ErrInvalidCOLRTable
		}
		return p.parseV1SweepGradient(paintOffset, palette, hasGlyph, glyphID, transform)
	case 13: // PaintVarTransform.
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
	case 15: // PaintVarTranslate.
		if paintOffset+12 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+8 : paintOffset+12])
		dx := p.applyVarFloat(varIdxBase, 0, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6]))))
		dy := p.applyVarFloat(varIdxBase, 1, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8]))))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(translateCOLR(dx, dy)))
	case 17: // PaintVarScale.
		if paintOffset+12 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+8 : paintOffset+12])
		sx := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6]))
		sy := p.applyVarF2Dot14(varIdxBase, 1, binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8]))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(scaleCOLR(sx, sy)))
	case 19: // PaintVarScaleAroundCenter.
		if paintOffset+16 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+12 : paintOffset+16])
		sx := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6]))
		sy := p.applyVarF2Dot14(varIdxBase, 1, binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8]))
		cx := p.applyVarFloat(varIdxBase, 2, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10]))))
		cy := p.applyVarFloat(varIdxBase, 3, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10:paintOffset+12]))))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(scaleCOLR(sx, sy), cx, cy)))
	case 21: // PaintVarScaleUniform.
		if paintOffset+10 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+6 : paintOffset+10])
		scale := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6]))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(scaleCOLR(scale, scale)))
	case 23: // PaintVarScaleUniformAroundCenter.
		if paintOffset+14 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+10 : paintOffset+14])
		scale := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6]))
		cx := p.applyVarFloat(varIdxBase, 1, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8]))))
		cy := p.applyVarFloat(varIdxBase, 2, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10]))))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(scaleCOLR(scale, scale), cx, cy)))
	case 25: // PaintVarRotate.
		if paintOffset+10 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+6 : paintOffset+10])
		angle := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(rotateCOLR(angle)))
	case 27: // PaintVarRotateAroundCenter.
		if paintOffset+14 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+10 : paintOffset+14])
		angle := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		cx := p.applyVarFloat(varIdxBase, 1, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8]))))
		cy := p.applyVarFloat(varIdxBase, 2, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10]))))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(rotateCOLR(angle), cx, cy)))
	case 29: // PaintVarSkew.
		if paintOffset+12 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+8 : paintOffset+12])
		xSkew := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		ySkew := p.applyVarF2Dot14(varIdxBase, 1, binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8])) * math.Pi
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(skewCOLR(xSkew, ySkew)))
	case 31: // PaintVarSkewAroundCenter.
		if paintOffset+16 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		childOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+12 : paintOffset+16])
		xSkew := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[paintOffset+4:paintOffset+6])) * math.Pi
		ySkew := p.applyVarF2Dot14(varIdxBase, 1, binary.BigEndian.Uint16(p.data[paintOffset+6:paintOffset+8])) * math.Pi
		cx := p.applyVarFloat(varIdxBase, 2, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10]))))
		cy := p.applyVarFloat(varIdxBase, 3, float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10:paintOffset+12]))))
		return p.parseV1Paint(childOffset, palette, depth+1, hasGlyph, glyphID, transform.Mul(aroundCOLR(skewCOLR(xSkew, ySkew), cx, cy)))
	default:
		return nil, ErrUnsupportedCOLR
	}
}

func (p *colrParser) applyVarFloat(varIdxBase uint32, field int, value float64) float64 {
	if p == nil || p.variationStore == nil || varIdxBase == 0xFFFFFFFF {
		return value
	}
	delta, ok := p.variationStore.delta(varIdxBase+uint32(field), p.variationCoords)
	if !ok {
		return value
	}
	return value + delta
}

func (p *colrParser) applyVarUint16(varIdxBase uint32, field int, value uint16, minValue uint16, maxValue uint16) uint16 {
	if p == nil || p.variationStore == nil || varIdxBase == 0xFFFFFFFF {
		return value
	}
	delta, ok := p.variationStore.delta(varIdxBase+uint32(field), p.variationCoords)
	if !ok {
		return value
	}
	adjusted := int(value) + int(math.Round(delta))
	if adjusted < int(minValue) {
		return minValue
	}
	if adjusted > int(maxValue) {
		return maxValue
	}
	return uint16(adjusted)
}

func (p *colrParser) applyVarF2Dot14(varIdxBase uint32, field int, value uint16) float64 {
	if p == nil || p.variationStore == nil || varIdxBase == 0xFFFFFFFF {
		return f2dot14(value)
	}
	delta, ok := p.variationStore.delta(varIdxBase+uint32(field), p.variationCoords)
	if !ok {
		return f2dot14(value)
	}
	adjusted := int(int16(value)) + int(math.Round(delta))
	if adjusted < math.MinInt16 {
		adjusted = math.MinInt16
	}
	if adjusted > math.MaxInt16 {
		adjusted = math.MaxInt16
	}
	return float64(adjusted) / 16384.0
}
