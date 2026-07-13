package fontglyph

import (
	"image"
	"testing"
)

func TestApplySubpixelFIR(t *testing.T) {
	tests := []struct {
		name    string
		samples []uint8
		index   int
		want    uint8
	}{
		{name: "normalization", samples: []uint8{255, 255, 255, 255, 255}, index: 2, want: 255},
		{name: "left edge clamp", samples: []uint8{90, 0, 0, 0, 0}, index: 0, want: 60},
		{name: "right edge clamp", samples: []uint8{0, 0, 0, 0, 90}, index: 4, want: 60},
		{name: "symmetric left", samples: []uint8{0, 0, 90, 0, 0}, index: 1, want: 20},
		{name: "symmetric right", samples: []uint8{0, 0, 90, 0, 0}, index: 3, want: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applySubpixelFIR(tt.samples, tt.index); got != tt.want {
				t.Fatalf("applySubpixelFIR() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSubpixelRasterEvidence(t *testing.T) {
	b := newTestBackend(t, "subpixel")
	glyph, ok := b.Rasterize('d', 1)
	if !ok || !glyph.Subpixel {
		t.Fatalf("Rasterize('d') ok=%t Subpixel=%t", ok, glyph.Subpixel)
	}
	w, h, _ := b.CellMetrics()
	if got := glyph.Image.Bounds().Size(); got != image.Pt(w, h) {
		t.Fatalf("image size = %v, want %v", got, image.Pt(w, h))
	}
	var visible, fringe, interior bool
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := glyph.Image.RGBAAt(x, y)
			visible = visible || p.A != 0
			fringe = fringe || p.R != p.B
			interior = interior || (p.R == p.G && p.G == p.B && p.B >= 250)
		}
	}
	if !visible || !fringe || !interior {
		t.Fatalf("coverage evidence visible=%t fringe=%t interior=%t", visible, fringe, interior)
	}
}

func TestSubpixelPlacementParity(t *testing.T) {
	goBackend := newTestBackend(t, "go")
	subpixelBackend := newTestBackend(t, "subpixel")
	goGlyph, goOK := goBackend.Rasterize('d', 1)
	subpixelGlyph, subpixelOK := subpixelBackend.Rasterize('d', 1)
	if !goOK || !subpixelOK {
		t.Fatalf("raster results: go=%t subpixel=%t", goOK, subpixelOK)
	}
	goBounds, subpixelBounds := alphaBounds(goGlyph.Image), alphaBounds(subpixelGlyph.Image)
	for _, delta := range []int{
		absoluteDelta(goBounds.Min.X - subpixelBounds.Min.X), absoluteDelta(goBounds.Min.Y - subpixelBounds.Min.Y),
		absoluteDelta(goBounds.Max.X - subpixelBounds.Max.X), absoluteDelta(goBounds.Max.Y - subpixelBounds.Max.Y),
	} {
		if delta > 1 {
			t.Fatalf("placement differs by %dpx: go=%v subpixel=%v", delta, goBounds, subpixelBounds)
		}
	}
}

func TestSubpixelEmptyGlyph(t *testing.T) {
	b := newTestBackend(t, "subpixel")
	glyph, ok := b.Rasterize(' ', 1)
	if !ok || !glyph.Subpixel {
		t.Fatalf("Rasterize(space) ok=%t Subpixel=%t", ok, glyph.Subpixel)
	}
	if bounds := alphaBounds(glyph.Image); !bounds.Empty() {
		t.Fatalf("space coverage bounds = %v, want empty", bounds)
	}
}

func TestBackendSubpixelGating(t *testing.T) {
	for _, tt := range []struct {
		mode         string
		wantSubpixel bool
		wantEngine   string
	}{
		{mode: "go", wantEngine: "go"},
		{mode: "auto"},
		{mode: "subpixel", wantSubpixel: true, wantEngine: "subpixel"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			b := newTestBackend(t, tt.mode)
			glyph, ok := b.Rasterize('d', 1)
			if !ok || glyph.Subpixel != tt.wantSubpixel {
				t.Fatalf("Rasterize('d') ok=%t Subpixel=%t, want %t", ok, glyph.Subpixel, tt.wantSubpixel)
			}
			if tt.mode == "subpixel" && b.dwRaster != nil {
				t.Fatal("subpixel mode initialized DirectWrite rasterizer")
			}
			if tt.wantEngine != "" && b.TextRasterEngine() != tt.wantEngine {
				t.Fatalf("TextRasterEngine() = %q, want %q", b.TextRasterEngine(), tt.wantEngine)
			}
		})
	}
}

func newTestBackend(t *testing.T, mode string) *OpenTypeBackend {
	t.Helper()
	b, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: mode})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func alphaBounds(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	var result image.Rectangle
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			if !found {
				result = image.Rect(x, y, x+1, y+1)
				found = true
				continue
			}
			result.Min.X, result.Min.Y = min(result.Min.X, x), min(result.Min.Y, y)
			result.Max.X, result.Max.Y = max(result.Max.X, x+1), max(result.Max.Y, y+1)
		}
	}
	return result
}

func absoluteDelta(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
