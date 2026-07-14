package fontglyph

import (
	"image"
	"testing"
)

// TestNarrowGlyphIsCentered guards against the regression where every glyph's
// ink was jammed to the cell's left edge (dotX = 1 - bounds.Min.X), discarding
// the font's designed centering. For narrow glyphs like the period that left a
// near-full-cell gap on the right that read as an extra space (e.g. "1.2"
// rendering as "1. 2"). The ink centroid of a period must sit near the middle
// of its cell, not against the left edge.
func TestNarrowGlyphIsCentered(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cellW, _, _ := backend.CellMetrics()

	glyph, ok := backend.Rasterize('.', 1)
	if !ok || glyph.Image == nil {
		t.Fatalf("expected period to rasterize")
	}
	cx, n := inkHorizontalCentroid(glyph.Image)
	if n == 0 {
		t.Fatalf("period rasterized with no visible ink")
	}
	// The dot should be roughly centered: within the middle 60% of the cell.
	lo, hi := float64(cellW)*0.2, float64(cellW)*0.8
	if cx < lo || cx > hi {
		t.Fatalf("period ink centroid x=%.1f outside centered band [%.1f, %.1f] of cell width %d (glyph jammed to one edge)", cx, lo, hi, cellW)
	}
}

// inkHorizontalCentroid returns the alpha-weighted mean x of an image's opaque
// pixels and the count of contributing pixels.
func inkHorizontalCentroid(img *image.RGBA) (float64, int) {
	bounds := img.Bounds()
	var sumX, weight float64
	n := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			w := float64(a)
			sumX += float64(x-bounds.Min.X) * w
			weight += w
			n++
		}
	}
	if weight == 0 {
		return 0, 0
	}
	return sumX / weight, n
}
