package raster

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/math/fixed"
)

func overRGBA(dst *image.RGBA, x int, y int, src color.RGBA) {
	if src.A == 0 {
		return
	}
	d := dst.RGBAAt(x, y)
	sa := uint32(src.A)
	inv := 255 - sa
	outA := sa + uint32(d.A)*inv/255
	if outA == 0 {
		dst.SetRGBA(x, y, color.RGBA{})
		return
	}
	dst.SetRGBA(x, y, color.RGBA{
		R: uint8((uint32(src.R)*sa + uint32(d.R)*uint32(d.A)*inv/255) / outA),
		G: uint8((uint32(src.G)*sa + uint32(d.G)*uint32(d.A)*inv/255) / outA),
		B: uint8((uint32(src.B)*sa + uint32(d.B)*uint32(d.A)*inv/255) / outA),
		A: uint8(outA),
	})
}

func compositeCOLRPixel(src, dst color.RGBA, mode int) color.RGBA {
	s := premulRGBA(src)
	d := premulRGBA(dst)
	switch mode {
	case colrCompositeClear:
		return color.RGBA{}
	case colrCompositeSrc:
		return src
	case colrCompositeDest:
		return dst
	case colrCompositeSrcOver:
		return unpremulRGBA(addPremul(s, scalePremul(d, 1-s.a)))
	case colrCompositeDestOver:
		return unpremulRGBA(addPremul(d, scalePremul(s, 1-d.a)))
	case colrCompositeSrcIn:
		return unpremulRGBA(scalePremul(s, d.a))
	case colrCompositeDestIn:
		return unpremulRGBA(scalePremul(d, s.a))
	case colrCompositeSrcOut:
		return unpremulRGBA(scalePremul(s, 1-d.a))
	case colrCompositeDestOut:
		return unpremulRGBA(scalePremul(d, 1-s.a))
	case colrCompositeSrcAtop:
		return unpremulRGBA(addPremul(scalePremul(s, d.a), scalePremul(d, 1-s.a)))
	case colrCompositeDestAtop:
		return unpremulRGBA(addPremul(scalePremul(d, s.a), scalePremul(s, 1-d.a)))
	case colrCompositeXor:
		return unpremulRGBA(addPremul(scalePremul(s, 1-d.a), scalePremul(d, 1-s.a)))
	case colrCompositePlus:
		return unpremulRGBA(premul{r: math.Min(1, s.r+d.r), g: math.Min(1, s.g+d.g), b: math.Min(1, s.b+d.b), a: math.Min(1, s.a+d.a)})
	case colrCompositeScreen, colrCompositeOverlay, colrCompositeDarken, colrCompositeLighten, colrCompositeColorDodge, colrCompositeColorBurn, colrCompositeHardLight, colrCompositeSoftLight, colrCompositeDifference, colrCompositeExclusion, colrCompositeMultiply, colrCompositeHSLHue, colrCompositeHSLSaturation, colrCompositeHSLColor, colrCompositeHSLLuminosity:
		return blendCOLRPixel(src, dst, mode)
	default:
		return color.RGBA{}
	}
}

type premul struct{ r, g, b, a float64 }

func premulRGBA(c color.RGBA) premul {
	a := float64(c.A) / 255
	return premul{r: float64(c.R) / 255 * a, g: float64(c.G) / 255 * a, b: float64(c.B) / 255 * a, a: a}
}

func unpremulRGBA(p premul) color.RGBA {
	p.a = clampUnit(p.a)
	if p.a == 0 {
		return color.RGBA{}
	}
	return color.RGBA{R: byte255(p.r / p.a), G: byte255(p.g / p.a), B: byte255(p.b / p.a), A: byte255(p.a)}
}

func addPremul(a, b premul) premul {
	return premul{r: a.r + b.r, g: a.g + b.g, b: a.b + b.b, a: a.a + b.a}
}
func scalePremul(p premul, f float64) premul {
	return premul{r: p.r * f, g: p.g * f, b: p.b * f, a: p.a * f}
}
func byte255(v float64) uint8     { return uint8(math.Round(clampUnit(v) * 255)) }
func clampUnit(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func blendCOLRPixel(src, dst color.RGBA, mode int) color.RGBA {
	as := float64(src.A) / 255
	ab := float64(dst.A) / 255
	a := as + ab - as*ab
	if a == 0 {
		return color.RGBA{}
	}
	cs := [3]float64{float64(src.R) / 255, float64(src.G) / 255, float64(src.B) / 255}
	cb := [3]float64{float64(dst.R) / 255, float64(dst.G) / 255, float64(dst.B) / 255}
	blendedColor := blendColor(cs, cb, mode)
	var out [3]float64
	for i := 0; i < 3; i++ {
		premul := (1-ab)*as*cs[i] + (1-as)*ab*cb[i] + as*ab*blendedColor[i]
		out[i] = premul / a
	}
	return color.RGBA{R: byte255(out[0]), G: byte255(out[1]), B: byte255(out[2]), A: byte255(a)}
}

func blendChannel(s, b float64, mode int) float64 {
	switch mode {
	case colrCompositeScreen:
		return b + s - b*s
	case colrCompositeOverlay:
		return hardLightChannel(b, s)
	case colrCompositeDarken:
		return math.Min(b, s)
	case colrCompositeLighten:
		return math.Max(b, s)
	case colrCompositeColorDodge:
		if s >= 1 {
			return 1
		}
		return math.Min(1, b/(1-s))
	case colrCompositeColorBurn:
		if s <= 0 {
			return 0
		}
		return 1 - math.Min(1, (1-b)/s)
	case colrCompositeHardLight:
		return hardLightChannel(s, b)
	case colrCompositeSoftLight:
		if s <= 0.5 {
			return b - (1-2*s)*b*(1-b)
		}
		d := math.Sqrt(b)
		return b + (2*s-1)*(d-b)
	case colrCompositeDifference:
		return math.Abs(b - s)
	case colrCompositeExclusion:
		return b + s - 2*b*s
	case colrCompositeMultiply:
		return b * s
	default:
		return s
	}
}

func blendColor(s, b [3]float64, mode int) [3]float64 {
	switch mode {
	case colrCompositeHSLHue:
		return setLum(setSat(s, sat(b)), lum(b))
	case colrCompositeHSLSaturation:
		return setLum(setSat(b, sat(s)), lum(b))
	case colrCompositeHSLColor:
		return setLum(s, lum(b))
	case colrCompositeHSLLuminosity:
		return setLum(b, lum(s))
	default:
		return [3]float64{blendChannel(s[0], b[0], mode), blendChannel(s[1], b[1], mode), blendChannel(s[2], b[2], mode)}
	}
}

func lum(c [3]float64) float64 { return 0.3*c[0] + 0.59*c[1] + 0.11*c[2] }

func sat(c [3]float64) float64 {
	return math.Max(c[0], math.Max(c[1], c[2])) - math.Min(c[0], math.Min(c[1], c[2]))
}

func setLum(c [3]float64, target float64) [3]float64 {
	delta := target - lum(c)
	return clipColor([3]float64{c[0] + delta, c[1] + delta, c[2] + delta})
}

func clipColor(c [3]float64) [3]float64 {
	l := lum(c)
	n := math.Min(c[0], math.Min(c[1], c[2]))
	x := math.Max(c[0], math.Max(c[1], c[2]))
	if n < 0 {
		for i := 0; i < 3; i++ {
			c[i] = l + ((c[i]-l)*l)/(l-n)
		}
	}
	if x > 1 {
		for i := 0; i < 3; i++ {
			c[i] = l + ((c[i]-l)*(1-l))/(x-l)
		}
	}
	return c
}

func setSat(c [3]float64, target float64) [3]float64 {
	minI, midI, maxI := sortedColorIndexes(c)
	if c[maxI] > c[minI] {
		c[midI] = ((c[midI] - c[minI]) * target) / (c[maxI] - c[minI])
		c[maxI] = target
	} else {
		c[midI] = 0
		c[maxI] = 0
	}
	c[minI] = 0
	return c
}

func sortedColorIndexes(c [3]float64) (int, int, int) {
	idx := [3]int{0, 1, 2}
	for i := 0; i < len(idx); i++ {
		for j := i + 1; j < len(idx); j++ {
			if c[idx[j]] < c[idx[i]] {
				idx[i], idx[j] = idx[j], idx[i]
			}
		}
	}
	return idx[0], idx[1], idx[2]
}

func hardLightChannel(s, b float64) float64 {
	if s <= 0.5 {
		return 2 * s * b
	}
	return 1 - 2*(1-s)*(1-b)
}

func pointXY(p fixed.Point26_6, offset int, baseline int, transform COLRTransform) (float32, float32) {
	x, y := transform.Apply(float64(p.X)/64, float64(p.Y)/64)
	return float32(offset) + float32(x), float32(baseline) + float32(y)
}
