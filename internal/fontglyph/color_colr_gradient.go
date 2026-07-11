package fontglyph

import (
	"encoding/binary"
	"image/color"
	"math"
)

type COLRFillKind uint8

const (
	COLRFillSolid COLRFillKind = iota
	COLRFillLinearGradient
	COLRFillRadialGradient
	COLRFillSweepGradient
	COLRFillComposite
)

type COLRColorStop struct {
	Offset float64
	Color  color.RGBA
}

type COLRLinearGradient struct {
	X0    float64
	Y0    float64
	X1    float64
	Y1    float64
	X2    float64
	Y2    float64
	Stops []COLRColorStop
}

type COLRRadialGradient struct {
	X0      float64
	Y0      float64
	Radius0 float64
	X1      float64
	Y1      float64
	Radius1 float64
	Stops   []COLRColorStop
}

type COLRSweepGradient struct {
	CenterX    float64
	CenterY    float64
	StartAngle float64
	EndAngle   float64
	Stops      []COLRColorStop
}

func (p *colrParser) parseV1LinearGradient(paintOffset int, palette []color.RGBA, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	if paintOffset+16 > len(p.data) || !hasGlyph {
		return nil, ErrInvalidCOLRTable
	}
	colorLineOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
	gradient, err := p.parseColorLine(colorLineOffset, palette)
	if err != nil {
		return nil, err
	}
	if p.data[paintOffset] == 5 {
		if varied, ok, err := p.parseVarColorLine(colorLineOffset, palette); err != nil {
			return nil, err
		} else if ok {
			gradient = varied
		}
	}
	gradient.X0 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6])))
	gradient.Y0 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8])))
	gradient.X1 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])))
	gradient.Y1 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10 : paintOffset+12])))
	gradient.X2 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+12 : paintOffset+14])))
	gradient.Y2 = float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+14 : paintOffset+16])))
	if p.data[paintOffset] == 5 {
		if paintOffset+20 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+16 : paintOffset+20])
		gradient.X0 = p.applyVarFloat(varIdxBase, 0, gradient.X0)
		gradient.Y0 = p.applyVarFloat(varIdxBase, 1, gradient.Y0)
		gradient.X1 = p.applyVarFloat(varIdxBase, 2, gradient.X1)
		gradient.Y1 = p.applyVarFloat(varIdxBase, 3, gradient.Y1)
		gradient.X2 = p.applyVarFloat(varIdxBase, 4, gradient.X2)
		gradient.Y2 = p.applyVarFloat(varIdxBase, 5, gradient.Y2)
	}
	return []COLRLayer{{GlyphID: glyphID, Transform: transform, Fill: COLRFillLinearGradient, LinearGradient: gradient}}, nil
}

func (p *colrParser) parseV1RadialGradient(paintOffset int, palette []color.RGBA, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	if paintOffset+16 > len(p.data) || !hasGlyph {
		return nil, ErrInvalidCOLRTable
	}
	colorLineOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
	line, err := p.parseColorLine(colorLineOffset, palette)
	if err != nil {
		return nil, err
	}
	if p.data[paintOffset] == 7 {
		if varied, ok, err := p.parseVarColorLine(colorLineOffset, palette); err != nil {
			return nil, err
		} else if ok {
			line = varied
		}
	}
	gradient := COLRRadialGradient{
		X0:      float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))),
		Y0:      float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8]))),
		Radius0: float64(binary.BigEndian.Uint16(p.data[paintOffset+8 : paintOffset+10])),
		X1:      float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+10 : paintOffset+12]))),
		Y1:      float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+12 : paintOffset+14]))),
		Radius1: float64(binary.BigEndian.Uint16(p.data[paintOffset+14 : paintOffset+16])),
		Stops:   line.Stops,
	}
	if p.data[paintOffset] == 7 {
		if paintOffset+20 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+16 : paintOffset+20])
		gradient.X0 = p.applyVarFloat(varIdxBase, 0, gradient.X0)
		gradient.Y0 = p.applyVarFloat(varIdxBase, 1, gradient.Y0)
		gradient.Radius0 = math.Max(0, p.applyVarFloat(varIdxBase, 2, gradient.Radius0))
		gradient.X1 = p.applyVarFloat(varIdxBase, 3, gradient.X1)
		gradient.Y1 = p.applyVarFloat(varIdxBase, 4, gradient.Y1)
		gradient.Radius1 = math.Max(0, p.applyVarFloat(varIdxBase, 5, gradient.Radius1))
	}
	return []COLRLayer{{GlyphID: glyphID, Transform: transform, Fill: COLRFillRadialGradient, RadialGradient: gradient}}, nil
}

func (p *colrParser) parseV1SweepGradient(paintOffset int, palette []color.RGBA, hasGlyph bool, glyphID uint16, transform COLRTransform) ([]COLRLayer, error) {
	if paintOffset+12 > len(p.data) || !hasGlyph {
		return nil, ErrInvalidCOLRTable
	}
	colorLineOffset := paintOffset + int(readOffset24(p.data[paintOffset+1:paintOffset+4]))
	line, err := p.parseColorLine(colorLineOffset, palette)
	if err != nil {
		return nil, err
	}
	if p.data[paintOffset] == 9 {
		if varied, ok, err := p.parseVarColorLine(colorLineOffset, palette); err != nil {
			return nil, err
		} else if ok {
			line = varied
		}
	}
	gradient := COLRSweepGradient{
		CenterX:    float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+4 : paintOffset+6]))),
		CenterY:    float64(int16(binary.BigEndian.Uint16(p.data[paintOffset+6 : paintOffset+8]))),
		StartAngle: f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10])) * math.Pi,
		EndAngle:   f2dot14(binary.BigEndian.Uint16(p.data[paintOffset+10:paintOffset+12])) * math.Pi,
		Stops:      line.Stops,
	}
	if p.data[paintOffset] == 9 {
		if paintOffset+16 > len(p.data) {
			return nil, ErrInvalidCOLRTable
		}
		varIdxBase := binary.BigEndian.Uint32(p.data[paintOffset+12 : paintOffset+16])
		gradient.CenterX = p.applyVarFloat(varIdxBase, 0, gradient.CenterX)
		gradient.CenterY = p.applyVarFloat(varIdxBase, 1, gradient.CenterY)
		gradient.StartAngle = p.applyVarF2Dot14(varIdxBase, 2, binary.BigEndian.Uint16(p.data[paintOffset+8:paintOffset+10])) * math.Pi
		gradient.EndAngle = p.applyVarF2Dot14(varIdxBase, 3, binary.BigEndian.Uint16(p.data[paintOffset+10:paintOffset+12])) * math.Pi
	}
	return []COLRLayer{{GlyphID: glyphID, Transform: transform, Fill: COLRFillSweepGradient, SweepGradient: gradient}}, nil
}

func (p *colrParser) parseColorLine(offset int, palette []color.RGBA) (COLRLinearGradient, error) {
	if offset < 0 || offset+3 > len(p.data) {
		return COLRLinearGradient{}, ErrInvalidCOLRTable
	}
	_ = p.data[offset] // Extend mode; the initial rasterizer clamps to pad behavior.
	stopCount := int(binary.BigEndian.Uint16(p.data[offset+1 : offset+3]))
	stopsOffset := offset + 3
	if stopCount <= 0 || stopsOffset+stopCount*6 > len(p.data) {
		return COLRLinearGradient{}, ErrInvalidCOLRTable
	}
	gradient := COLRLinearGradient{Stops: make([]COLRColorStop, 0, stopCount)}
	for i := 0; i < stopCount; i++ {
		o := stopsOffset + i*6
		paletteIndex := binary.BigEndian.Uint16(p.data[o+2 : o+4])
		var c color.RGBA
		if paletteIndex == foregroundPaletteIndex {
			c = color.RGBA{255, 255, 255, 255}
		} else if int(paletteIndex) < len(palette) {
			c = palette[paletteIndex]
		} else {
			return COLRLinearGradient{}, ErrInvalidCPALTable
		}
		c.A = scaleAlpha(c.A, binary.BigEndian.Uint16(p.data[o+4:o+6]))
		gradient.Stops = append(gradient.Stops, COLRColorStop{
			Offset: f2dot14(binary.BigEndian.Uint16(p.data[o : o+2])),
			Color:  c,
		})
	}
	return gradient, nil
}

func (p *colrParser) parseVarColorLine(offset int, palette []color.RGBA) (COLRLinearGradient, bool, error) {
	if offset < 0 || offset+3 > len(p.data) {
		return COLRLinearGradient{}, false, ErrInvalidCOLRTable
	}
	stopCount := int(binary.BigEndian.Uint16(p.data[offset+1 : offset+3]))
	stopsOffset := offset + 3
	if stopCount <= 0 {
		return COLRLinearGradient{}, false, ErrInvalidCOLRTable
	}
	if stopsOffset+stopCount*10 > len(p.data) {
		return COLRLinearGradient{}, false, nil
	}
	gradient := COLRLinearGradient{Stops: make([]COLRColorStop, 0, stopCount)}
	for i := 0; i < stopCount; i++ {
		o := stopsOffset + i*10
		paletteIndex := binary.BigEndian.Uint16(p.data[o+2 : o+4])
		var c color.RGBA
		if paletteIndex == foregroundPaletteIndex {
			c = color.RGBA{255, 255, 255, 255}
		} else if int(paletteIndex) < len(palette) {
			c = palette[paletteIndex]
		} else {
			return COLRLinearGradient{}, false, ErrInvalidCPALTable
		}
		varIdxBase := binary.BigEndian.Uint32(p.data[o+6 : o+10])
		offsetValue := p.applyVarF2Dot14(varIdxBase, 0, binary.BigEndian.Uint16(p.data[o:o+2]))
		alpha := p.applyVarUint16(varIdxBase, 1, binary.BigEndian.Uint16(p.data[o+4:o+6]), 0, 0x4000)
		c.A = scaleAlpha(c.A, alpha)
		gradient.Stops = append(gradient.Stops, COLRColorStop{Offset: offsetValue, Color: c})
	}
	return gradient, true, nil
}

func (g COLRLinearGradient) colorAt(x, y float64) color.RGBA {
	if len(g.Stops) == 0 {
		return color.RGBA{}
	}
	if len(g.Stops) == 1 {
		return g.Stops[0].Color
	}
	dx := g.X1 - g.X0
	dy := g.Y1 - g.Y0
	denom := dx*dx + dy*dy
	var t float64
	if denom > 0 {
		t = ((x-g.X0)*dx + (y-g.Y0)*dy) / denom
	}
	return colorFromStops(g.Stops, t)
}

func (g COLRRadialGradient) colorAt(x, y float64) color.RGBA {
	if len(g.Stops) == 0 {
		return color.RGBA{}
	}
	if len(g.Stops) == 1 {
		return g.Stops[0].Color
	}
	dx := x - g.X0
	dy := y - g.Y0
	dist := math.Sqrt(dx*dx + dy*dy)
	denom := g.Radius1 - g.Radius0
	t := 0.0
	if denom != 0 {
		t = (dist - g.Radius0) / denom
	}
	return colorFromStops(g.Stops, t)
}

func (g COLRSweepGradient) colorAt(x, y float64) color.RGBA {
	if len(g.Stops) == 0 {
		return color.RGBA{}
	}
	if len(g.Stops) == 1 {
		return g.Stops[0].Color
	}
	angle := math.Atan2(y-g.CenterY, x-g.CenterX)
	span := g.EndAngle - g.StartAngle
	if span == 0 {
		return colorFromStops(g.Stops, 0)
	}
	for span < 0 {
		span += 2 * math.Pi
	}
	for angle < g.StartAngle {
		angle += 2 * math.Pi
	}
	t := (angle - g.StartAngle) / span
	return colorFromStops(g.Stops, t)
}

func colorFromStops(stops []COLRColorStop, t float64) color.RGBA {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	if len(stops) == 1 || t <= stops[0].Offset {
		return stops[0].Color
	}
	last := stops[len(stops)-1]
	if t >= last.Offset {
		return last.Color
	}
	for i := 1; i < len(stops); i++ {
		right := stops[i]
		if t > right.Offset {
			continue
		}
		left := stops[i-1]
		span := right.Offset - left.Offset
		local := 0.0
		if span > 0 {
			local = (t - left.Offset) / span
		}
		return lerpRGBA(left.Color, right.Color, local)
	}
	return last.Color
}

func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t + 0.5),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t + 0.5),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t + 0.5),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t + 0.5),
	}
}
