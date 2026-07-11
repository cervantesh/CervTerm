package fontglyph

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestRedistributableGoRegularRealFontFixture(t *testing.T) {
	face, metrics, err := loadOpenTypeFace(goregular.TTF, Spec{Family: "Go", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("loadOpenTypeFace(goregular): %v", err)
	}
	if metrics.Ascent <= 0 || metrics.Descent <= 0 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
	if face.sfnt == nil || face.face == nil {
		t.Fatalf("expected parsed sfnt and font face")
	}
	if face.tables.HasAnyColor() {
		t.Fatalf("Go regular should be a monochrome real-font fixture, got color tables %#v", face.tables)
	}
}

func TestRedistributableGoBoldRealFontRasterAndShape(t *testing.T) {
	face, _, err := loadOpenTypeFace(gobold.TTF, Spec{Family: "Go", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("loadOpenTypeFace(gobold): %v", err)
	}
	glyphs, ok := (SimpleShaper{}).Shape("B", face, 19)
	if !ok || len(glyphs) != 1 || glyphs[0].GlyphID == 0 || glyphs[0].XAdvance <= 0 {
		t.Fatalf("unexpected shaped glyphs from real font: %#v ok=%v", glyphs, ok)
	}
}
