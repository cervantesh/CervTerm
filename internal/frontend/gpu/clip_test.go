package gpu

import "testing"

func TestClipRectIntersectClampAndScissor(t *testing.T) {
	outer := ClipRect{X: 10, Y: 20, Width: 100, Height: 80}
	inner := ClipRect{X: 50, Y: 0, Width: 100, Height: 60}
	if got, want := outer.Intersect(inner), (ClipRect{X: 50, Y: 20, Width: 60, Height: 40}); got != want {
		t.Fatalf("intersection = %#v, want %#v", got, want)
	}
	if got, want := (ClipRect{X: -5, Y: 90, Width: 20, Height: 20}).Clamp(100, 100), (ClipRect{X: 0, Y: 90, Width: 15, Height: 10}); got != want {
		t.Fatalf("clamp = %#v, want %#v", got, want)
	}
	x, y, width, height := (ClipRect{X: 7, Y: 11, Width: 13, Height: 17}).Scissor(100)
	if x != 7 || y != 72 || width != 13 || height != 17 {
		t.Fatalf("scissor = (%d,%d %dx%d), want (7,72 13x17)", x, y, width, height)
	}
}

func TestClipRectDisjointIsEmptyAtBoundary(t *testing.T) {
	got := (ClipRect{Width: 10, Height: 10}).Intersect(ClipRect{X: 10, Width: 5, Height: 5})
	if got.Width != 0 || got.Height != 0 || got.X != 10 {
		t.Fatalf("disjoint intersection = %#v", got)
	}
}
