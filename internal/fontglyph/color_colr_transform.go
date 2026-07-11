package fontglyph

import "math"

type COLRTransform struct {
	XX float64
	YX float64
	XY float64
	YY float64
	DX float64
	DY float64
}

func identityCOLRTransform() COLRTransform {
	return COLRTransform{XX: 1, YY: 1}
}

func translateCOLR(dx, dy float64) COLRTransform {
	return COLRTransform{XX: 1, YY: 1, DX: dx, DY: dy}
}

func scaleCOLR(sx, sy float64) COLRTransform {
	return COLRTransform{XX: sx, YY: sy}
}

func rotateCOLR(angle float64) COLRTransform {
	sin, cos := math.Sin(angle), math.Cos(angle)
	return COLRTransform{XX: cos, YX: sin, XY: -sin, YY: cos}
}

func skewCOLR(xAngle, yAngle float64) COLRTransform {
	return COLRTransform{XX: 1, YX: math.Tan(yAngle), XY: math.Tan(xAngle), YY: 1}
}

func aroundCOLR(transform COLRTransform, cx, cy float64) COLRTransform {
	return translateCOLR(cx, cy).Mul(transform).Mul(translateCOLR(-cx, -cy))
}

func (a COLRTransform) Mul(b COLRTransform) COLRTransform {
	return COLRTransform{
		XX: a.XX*b.XX + a.XY*b.YX,
		YX: a.YX*b.XX + a.YY*b.YX,
		XY: a.XX*b.XY + a.XY*b.YY,
		YY: a.YX*b.XY + a.YY*b.YY,
		DX: a.XX*b.DX + a.XY*b.DY + a.DX,
		DY: a.YX*b.DX + a.YY*b.DY + a.DY,
	}
}

func (t COLRTransform) Apply(x, y float64) (float64, float64) {
	return t.XX*x + t.XY*y + t.DX, t.YX*x + t.YY*y + t.DY
}

func f2dot14(value uint16) float64 {
	return float64(int16(value)) / 16384.0
}

func fixed16Dot16(value uint32) float64 {
	return float64(int32(value)) / 65536.0
}
