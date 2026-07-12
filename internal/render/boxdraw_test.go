package render

import (
	"math"
	"testing"
)

func TestBoxGlyphLines(t *testing.T) {
	const w, h = float32(10), float32(16)
	tests := []struct {
		name  rune
		check func(*testing.T, []CellRect)
	}{
		{'─', func(t *testing.T, r []CellRect) { assertSpans(t, r, true, w) }},
		{'│', func(t *testing.T, r []CellRect) { assertSpans(t, r, false, h) }},
		{'┼', func(t *testing.T, r []CellRect) { assertSpans(t, r, true, w); assertSpans(t, r, false, h) }},
		{'┏', func(t *testing.T, r []CellRect) {
			if !hasRect(t, r, func(rc CellRect) bool { return near(rc.X+rc.W, w) && rc.W > w/2 }) ||
				!hasRect(t, r, func(rc CellRect) bool { return near(rc.Y+rc.H, h) && rc.H > h/2 }) {
				t.Fatal("heavy corner does not have right and down arms")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(string(tt.name), func(t *testing.T) {
			r, ok := BoxGlyph(tt.name, w, h)
			if !ok {
				t.Fatal("BoxGlyph returned ok=false")
			}
			tt.check(t, r)
		})
	}
}

func TestBoxGlyphBlocks(t *testing.T) {
	const w, h = float32(16), float32(24)
	tests := []struct {
		r    rune
		want []CellRect
	}{
		{'█', []CellRect{{0, 0, w, h, 1}}},
		{'▀', []CellRect{{0, 0, w, 12, 1}}},
		{'▄', []CellRect{{0, 12, w, 12, 1}}},
		{'▌', []CellRect{{0, 0, 8, h, 1}}},
		{'▐', []CellRect{{8, 0, 8, h, 1}}},
		{'▃', []CellRect{{0, 15, w, 9, 1}}},
		{'░', []CellRect{{0, 0, w, h, .25}}},
		{'▖', []CellRect{{0, 12, 8, 12, 1}}},
	}
	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got, ok := BoxGlyph(tt.r, w, h)
			if !ok {
				t.Fatal("BoxGlyph returned ok=false")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rects, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range got {
				if !rectNear(got[i], tt.want[i]) {
					t.Fatalf("rect %d = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBoxGlyphFallback(t *testing.T) {
	for _, r := range []rune{'A', '你', '😀', '╱', '╲', '╳'} {
		rects, ok := BoxGlyph(r, 10, 16)
		if ok || rects != nil {
			t.Errorf("%q: got (%#v, %v), want (nil, false)", r, rects, ok)
		}
	}
}

func TestBoxGlyphHorizontalAdjacency(t *testing.T) {
	const w = float32(11)
	r, ok := BoxGlyph('─', w, 17)
	if !ok {
		t.Fatal("BoxGlyph returned ok=false")
	}
	assertSpans(t, r, true, w)
}

func TestBoxGlyphHalvesAlignWithQuadrants(t *testing.T) {
	// At an odd cell size the lower/right halves must split at the same pixel
	// boundary as the quadrants, so ▄ beside ▟ (or ▐ beside ▛) has no 1px seam.
	const w, h = float32(11), float32(25)
	lower, _ := BoxGlyph('▄', w, h)
	quad, _ := BoxGlyph('▟', w, h) // bottom-left + bottom-right + ... bottom row
	if lower[0].Y != quadBottomTop(quad) {
		t.Fatalf("lower half top %v != quadrant bottom top %v", lower[0].Y, quadBottomTop(quad))
	}
	right, _ := BoxGlyph('▐', w, h)
	rq, _ := BoxGlyph('▗', w, h) // bottom-right quadrant
	if right[0].X != rq[0].X {
		t.Fatalf("right half left %v != quadrant right left %v", right[0].X, rq[0].X)
	}
}

func quadBottomTop(rects []CellRect) float32 {
	// The bottom-row quadrants start at the split Y — the largest Y among regions.
	max := float32(0)
	for _, rc := range rects {
		if rc.Y > max {
			max = rc.Y
		}
	}
	return max
}

func assertSpans(t *testing.T, rects []CellRect, horizontal bool, extent float32) {
	t.Helper()
	if !hasRect(t, rects, func(rc CellRect) bool {
		if horizontal {
			return near(rc.X, 0) && near(rc.X+rc.W, extent)
		}
		return near(rc.Y, 0) && near(rc.Y+rc.H, extent)
	}) {
		t.Fatalf("no rectangle spans 0..%v: %#v", extent, rects)
	}
}

func hasRect(t *testing.T, rects []CellRect, predicate func(CellRect) bool) bool {
	t.Helper()
	for _, rc := range rects {
		if predicate(rc) {
			return true
		}
	}
	return false
}

func rectNear(a, b CellRect) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H) && near(a.Alpha, b.Alpha)
}
func near(a, b float32) bool { return math.Abs(float64(a-b)) < .001 }
