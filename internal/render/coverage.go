package render

import "math"

// CoverageLUT returns a glyph coverage lookup table using stem darkening then inverse gamma.
func CoverageLUT(gamma, darken float64) [256]uint8 {
	var lut [256]uint8
	for i := range lut {
		coverage := float64(i) / 255
		coverage = math.Min(1, coverage*(1+darken))
		coverage = math.Pow(coverage, 1/gamma)
		lut[i] = uint8(math.Round(coverage * 255))
	}
	lut[0] = 0
	lut[255] = 255
	return lut
}

// ApplyCoverageLUT remaps premultiplied white RGBA pixels in place.
func ApplyCoverageLUT(pix []uint8, lut *[256]uint8) {
	for i, value := range pix {
		pix[i] = lut[value]
	}
}
