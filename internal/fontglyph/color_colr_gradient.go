package fontglyph

import "image/color"

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
