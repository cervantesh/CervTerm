package platform

import (
	"image"
	"testing"
)

type closeProbe struct {
	closes int
}

func (p *closeProbe) RasterizeGlyph(uint16, int, int, int, int, float32) (*image.RGBA, bool) {
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), true
}

func (p *closeProbe) Close() {
	if p.closes == 0 {
		p.closes++
	}
}

func TestNativeHooksKeepCrossPackageNativeStateInjected(t *testing.T) {
	probe := &closeProbe{}
	hooks := NativeHooks{
		Shape: func(request ShapeRequest) ([]ShapeGlyph, bool) {
			return []ShapeGlyph{{GlyphID: uint16(request.Face.Index + 1), XAdvance: float64(request.PPEM)}}, true
		},
		Raster: func(TextRasterSpec) GlyphRasterizer { return probe },
	}
	shaped, ok := hooks.Shape(ShapeRequest{Face: FaceSource{Path: "fixture.ttf", Index: 2}, PPEM: 16})
	if !ok || len(shaped) != 1 || shaped[0].GlyphID != 3 || shaped[0].XAdvance != 16 {
		t.Fatalf("injected shape = %#v, %t", shaped, ok)
	}
	raster := hooks.Raster(TextRasterSpec{})
	raster.Close()
	raster.Close()
	if probe.closes != 1 {
		t.Fatalf("idempotent injected close count = %d", probe.closes)
	}
}
