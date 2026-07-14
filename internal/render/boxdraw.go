package render

import "math"

type CellRect struct {
	X, Y, W, H float32
	Alpha      float32
}

type arms struct{ up, down, left, right uint8 }

var armGlyphs = map[rune]arms{
	'─': {0, 0, 1, 1}, '━': {0, 0, 2, 2}, '│': {1, 1, 0, 0}, '┃': {2, 2, 0, 0},
	'┌': {0, 1, 0, 1}, '┍': {0, 1, 0, 2}, '┎': {0, 2, 0, 1}, '┏': {0, 2, 0, 2},
	'┐': {0, 1, 1, 0}, '┑': {0, 1, 2, 0}, '┒': {0, 2, 1, 0}, '┓': {0, 2, 2, 0},
	'└': {1, 0, 0, 1}, '┕': {1, 0, 0, 2}, '┖': {2, 0, 0, 1}, '┗': {2, 0, 0, 2},
	'┘': {1, 0, 1, 0}, '┙': {1, 0, 2, 0}, '┚': {2, 0, 1, 0}, '┛': {2, 0, 2, 0},
	'├': {1, 1, 0, 1}, '┝': {1, 1, 0, 2}, '┞': {2, 1, 0, 1}, '┟': {1, 2, 0, 1},
	'┠': {2, 2, 0, 1}, '┡': {2, 1, 0, 2}, '┢': {1, 2, 0, 2}, '┣': {2, 2, 0, 2},
	'┤': {1, 1, 1, 0}, '┥': {1, 1, 2, 0}, '┦': {2, 1, 1, 0}, '┧': {1, 2, 1, 0},
	'┨': {2, 2, 1, 0}, '┩': {2, 1, 2, 0}, '┪': {1, 2, 2, 0}, '┫': {2, 2, 2, 0},
	'┬': {0, 1, 1, 1}, '┭': {0, 1, 2, 1}, '┮': {0, 1, 1, 2}, '┯': {0, 1, 2, 2},
	'┰': {0, 2, 1, 1}, '┱': {0, 2, 2, 1}, '┲': {0, 2, 1, 2}, '┳': {0, 2, 2, 2},
	'┴': {1, 0, 1, 1}, '┵': {1, 0, 2, 1}, '┶': {1, 0, 1, 2}, '┷': {1, 0, 2, 2},
	'┸': {2, 0, 1, 1}, '┹': {2, 0, 2, 1}, '┺': {2, 0, 1, 2}, '┻': {2, 0, 2, 2},
	'┼': {1, 1, 1, 1}, '┽': {1, 1, 2, 1}, '┾': {1, 1, 1, 2}, '┿': {1, 1, 2, 2},
	'╀': {2, 1, 1, 1}, '╁': {1, 2, 1, 1}, '╂': {2, 2, 1, 1}, '╃': {2, 1, 2, 1},
	'╄': {2, 1, 1, 2}, '╅': {1, 2, 2, 1}, '╆': {1, 2, 1, 2}, '╇': {2, 2, 2, 1},
	'╈': {2, 2, 1, 2}, '╉': {2, 1, 2, 2}, '╊': {1, 2, 2, 2}, '╋': {2, 2, 2, 2},
	'╭': {0, 1, 0, 1}, '╮': {0, 1, 1, 0}, '╯': {1, 0, 1, 0}, '╰': {1, 0, 0, 1},
	'╴': {0, 0, 1, 0}, '╵': {1, 0, 0, 0}, '╶': {0, 0, 0, 1}, '╷': {0, 1, 0, 0},
	'╸': {0, 0, 2, 0}, '╹': {2, 0, 0, 0}, '╺': {0, 0, 0, 2}, '╻': {0, 2, 0, 0},
	'╼': {0, 0, 1, 2}, '╽': {1, 2, 0, 0}, '╾': {0, 0, 2, 1}, '╿': {2, 1, 0, 0},
}

var doubleGlyphs = map[rune]arms{
	'═': {0, 0, 2, 2}, '║': {2, 2, 0, 0}, '╒': {0, 1, 0, 2}, '╓': {0, 2, 0, 1}, '╔': {0, 2, 0, 2},
	'╕': {0, 1, 2, 0}, '╖': {0, 2, 1, 0}, '╗': {0, 2, 2, 0}, '╘': {1, 0, 0, 2}, '╙': {2, 0, 0, 1},
	'╚': {2, 0, 0, 2}, '╛': {1, 0, 2, 0}, '╜': {2, 0, 1, 0}, '╝': {2, 0, 2, 0}, '╞': {1, 1, 0, 2},
	'╟': {2, 2, 0, 1}, '╠': {2, 2, 0, 2}, '╡': {1, 1, 2, 0}, '╢': {2, 2, 1, 0}, '╣': {2, 2, 2, 0},
	'╤': {0, 1, 2, 2}, '╥': {0, 2, 1, 1}, '╦': {0, 2, 2, 2}, '╧': {1, 0, 2, 2}, '╨': {2, 0, 1, 1},
	'╩': {2, 0, 2, 2}, '╪': {1, 1, 2, 2}, '╫': {2, 2, 1, 1}, '╬': {2, 2, 2, 2},
}

func BoxGlyph(r rune, cellW, cellH float32) ([]CellRect, bool) {
	light := max(float32(1), float32(math.Round(float64(cellH/8))))
	heavy := max(light+1, float32(math.Round(float64(cellH/4))))
	if a, ok := armGlyphs[r]; ok {
		return drawArms(a, cellW, cellH, light, heavy), true
	}
	if r >= '\u2504' && r <= '\u250b' || r >= '\u254c' && r <= '\u254f' {
		return drawDashed(r, cellW, cellH, light, heavy), true
	}
	if a, ok := doubleGlyphs[r]; ok {
		return drawDoubleArms(a, cellW, cellH, light), true
	}
	return blockGlyph(r, cellW, cellH)
}

func drawArms(a arms, w, h, light, heavy float32) []CellRect {
	t := func(weight uint8) float32 {
		if weight == 2 {
			return heavy
		}
		return light
	}
	mx, my := w/2, h/2
	maxT := float32(0)
	for _, weight := range []uint8{a.up, a.down, a.left, a.right} {
		if weight > 0 {
			maxT = max(maxT, t(weight))
		}
	}
	r := make([]CellRect, 0, 5)
	if a.left > 0 && a.left == a.right {
		th := t(a.left)
		r = append(r, CellRect{0, my - th/2, w, th, 1})
	} else if a.left > 0 {
		th := t(a.left)
		r = append(r, CellRect{0, my - th/2, mx + th/2, th, 1})
	}
	if a.right > 0 && a.left != a.right {
		th := t(a.right)
		r = append(r, CellRect{mx - th/2, my - th/2, w - mx + th/2, th, 1})
	}
	if a.up > 0 && a.up == a.down {
		th := t(a.up)
		r = append(r, CellRect{mx - th/2, 0, th, h, 1})
	} else if a.up > 0 {
		th := t(a.up)
		r = append(r, CellRect{mx - th/2, 0, th, my + th/2, 1})
	}
	if a.down > 0 && a.up != a.down {
		th := t(a.down)
		r = append(r, CellRect{mx - th/2, my - th/2, th, h - my + th/2, 1})
	}
	if maxT > 0 && (a.left != a.right || a.up != a.down) {
		r = append(r, CellRect{mx - maxT/2, my - maxT/2, maxT, maxT, 1})
	}
	return r
}

func drawDashed(r rune, w, h, light, heavy float32) []CellRect {
	vertical := r == '\u2506' || r == '\u2507' || r == '\u250a' || r == '\u250b' || r == '\u254e' || r == '\u254f'
	weight := light
	if r == '\u2505' || r == '\u2507' || r == '\u2509' || r == '\u250b' || r == '\u254d' || r == '\u254f' {
		weight = heavy
	}
	count := 2
	if r >= '\u2504' && r <= '\u2507' {
		count = 3
	} else if r >= '\u2508' && r <= '\u250b' {
		count = 4
	}
	length := w
	if vertical {
		length = h
	}
	unit := length / float32(count*2-1)
	out := make([]CellRect, count)
	for i := range out {
		if vertical {
			out[i] = CellRect{w/2 - weight/2, float32(i*2) * unit, weight, unit, 1}
		} else {
			out[i] = CellRect{float32(i*2) * unit, h/2 - weight/2, unit, weight, 1}
		}
	}
	return out
}

func drawDoubleArms(a arms, w, h, light float32) []CellRect {
	mx, my, offset := w/2, h/2, light
	out := make([]CellRect, 0, 8)
	if a.left > 0 {
		ys := []float32{my}
		if a.left == 2 {
			ys = []float32{my - offset, my + offset}
		}
		for _, y := range ys {
			out = append(out, CellRect{0, y - light/2, mx + offset + light/2, light, 1})
		}
	}
	if a.right > 0 {
		ys := []float32{my}
		if a.right == 2 {
			ys = []float32{my - offset, my + offset}
		}
		for _, y := range ys {
			out = append(out, CellRect{mx - offset - light/2, y - light/2, w - mx + offset + light/2, light, 1})
		}
	}
	if a.up > 0 {
		xs := []float32{mx}
		if a.up == 2 {
			xs = []float32{mx - offset, mx + offset}
		}
		for _, x := range xs {
			out = append(out, CellRect{x - light/2, 0, light, my + offset + light/2, 1})
		}
	}
	if a.down > 0 {
		xs := []float32{mx}
		if a.down == 2 {
			xs = []float32{mx - offset, mx + offset}
		}
		for _, x := range xs {
			out = append(out, CellRect{x - light/2, my - offset - light/2, light, h - my + offset + light/2, 1})
		}
	}
	return out
}

// blockQuadrants maps the quadrant block runes (U+2596..U+259F) to a 4-bit
// region mask. Package-level so blockGlyph — called for every rendered cell —
// does not allocate a fresh map per call.
var blockQuadrants = map[rune]uint8{'▖': 4, '▗': 8, '▘': 1, '▙': 13, '▚': 9, '▛': 7, '▜': 11, '▝': 2, '▞': 6, '▟': 14}

func blockGlyph(r rune, w, h float32) ([]CellRect, bool) {
	if r == '\u2580' {
		return []CellRect{{0, 0, w, roundedFraction(h, 4, 8), 1}}, true
	}
	if r == '\u2584' { // lower half: split at the same point as the quadrants
		hh := roundedFraction(h, 4, 8)
		return []CellRect{{0, hh, w, h - hh, 1}}, true
	}
	if r >= '\u2581' && r <= '\u2588' {
		n := int(r - '\u2580')
		height := roundedFraction(h, n, 8)
		return []CellRect{{0, h - height, w, height, 1}}, true
	}
	if r >= '\u2589' && r <= '\u258f' {
		n := 8 - int(r-'\u2588')
		width := roundedFraction(w, n, 8)
		return []CellRect{{0, 0, width, h, 1}}, true
	}
	if r == '\u2590' { // right half: split at the same point as the quadrants
		hw := roundedFraction(w, 4, 8)
		return []CellRect{{hw, 0, w - hw, h, 1}}, true
	}
	if r >= '\u2591' && r <= '\u2593' {
		return []CellRect{{0, 0, w, h, float32(r-'\u2590') * .25}}, true
	}
	if r == '\u2594' {
		return []CellRect{{0, 0, w, roundedFraction(h, 1, 8), 1}}, true
	}
	if r == '\u2595' {
		width := roundedFraction(w, 1, 8)
		return []CellRect{{w - width, 0, width, h, 1}}, true
	}
	mask, ok := blockQuadrants[r]
	if !ok {
		return nil, false
	}
	hw, hh := roundedFraction(w, 4, 8), roundedFraction(h, 4, 8)
	regions := []CellRect{{0, 0, hw, hh, 1}, {hw, 0, w - hw, hh, 1}, {0, hh, hw, h - hh, 1}, {hw, hh, w - hw, h - hh, 1}}
	out := make([]CellRect, 0, 3)
	for i, rc := range regions {
		if mask&(1<<i) != 0 {
			out = append(out, rc)
		}
	}
	return out, true
}

func roundedFraction(v float32, n, d int) float32 {
	return float32(math.Round(float64(v) * float64(n) / float64(d)))
}
