package fontglyph

import "image/color"

const (
	colrCompositeClear         = 0
	colrCompositeSrc           = 1
	colrCompositeDest          = 2
	colrCompositeSrcOver       = 3
	colrCompositeDestOver      = 4
	colrCompositeSrcIn         = 5
	colrCompositeDestIn        = 6
	colrCompositeSrcOut        = 7
	colrCompositeDestOut       = 8
	colrCompositeSrcAtop       = 9
	colrCompositeDestAtop      = 10
	colrCompositeXor           = 11
	colrCompositePlus          = 12
	colrCompositeScreen        = 13
	colrCompositeOverlay       = 14
	colrCompositeDarken        = 15
	colrCompositeLighten       = 16
	colrCompositeColorDodge    = 17
	colrCompositeColorBurn     = 18
	colrCompositeHardLight     = 19
	colrCompositeSoftLight     = 20
	colrCompositeDifference    = 21
	colrCompositeExclusion     = 22
	colrCompositeMultiply      = 23
	colrCompositeHSLHue        = 24
	colrCompositeHSLSaturation = 25
	colrCompositeHSLColor      = 26
	colrCompositeHSLLuminosity = 27
	colrCompositeLast          = colrCompositeHSLLuminosity
)

func (p *colrParser) parseV1Composite(paintOffset int, palette []color.RGBA, depth int, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	if paintOffset+8 > len(p.data) {
		return nil, ErrInvalidCOLRTable
	}
	sourceOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
	compositeMode := int(p.data[paintOffset+4])
	backdropOffset := paintOffset + int(readOffset24(p.data[paintOffset+5:paintOffset+8]))

	var source []COLRLayer
	var backdrop []COLRLayer
	var err error
	if compositeMode != colrCompositeClear && compositeMode != colrCompositeDest {
		source, err = p.parseV1Paint(sourceOffset, palette, depth+1, hasGlyph, glyphID, transform)
		if err != nil {
			return nil, err
		}
	}
	if compositeMode != colrCompositeClear && compositeMode != colrCompositeSrc {
		backdrop, err = p.parseV1Paint(backdropOffset, palette, depth+1, hasGlyph, glyphID, transform)
		if err != nil {
			return nil, err
		}
	}
	switch compositeMode {
	case colrCompositeClear:
		return nil, nil
	case colrCompositeSrc:
		return source, nil
	case colrCompositeDest:
		return backdrop, nil
	case colrCompositeSrcOver:
		return append(backdrop, source...), nil
	case colrCompositeDestOver:
		return append(source, backdrop...), nil
	default:
		if compositeMode >= colrCompositeSrcIn && compositeMode <= colrCompositeLast {
			return []COLRLayer{{Fill: COLRFillComposite, CompositeMode: compositeMode, Source: source, Backdrop: backdrop}}, nil
		}
		return nil, ErrUnsupportedCOLR
	}
}
