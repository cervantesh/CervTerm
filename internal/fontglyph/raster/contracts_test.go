package raster

import (
	"image"
	"reflect"
	"testing"
)

func TestGlyphPreservesImageLayoutAndConcreteIdentity(t *testing.T) {
	img := &image.RGBA{Pix: make([]byte, 96), Stride: 24, Rect: image.Rect(2, 3, 8, 7)}
	glyph := Glyph{Image: img, Width: 6, Height: 4, BearingX: -1, BearingY: 3, AdvanceX: 7.5, CellSpan: 2, HasColor: true, Subpixel: true}
	if glyph.Image != img || glyph.Image.Stride != 24 || glyph.Image.Rect != image.Rect(2, 3, 8, 7) || len(glyph.Image.Pix) != 96 {
		t.Fatalf("glyph image layout changed: %#v", glyph)
	}
	if got := reflect.TypeOf(glyph).String(); got != "raster.Glyph" {
		t.Fatalf("concrete glyph type = %q", got)
	}
}

func TestShapeGlyphValuesAreDetached(t *testing.T) {
	input := []ShapeGlyph{{GlyphID: 9, XOffset: 1, YOffset: -2, XAdvance: 8}}
	copyOfInput := append([]ShapeGlyph(nil), input...)
	input[0].GlyphID = 10
	if copyOfInput[0].GlyphID != 9 {
		t.Fatalf("shape glyph copy retained caller mutation: %#v", copyOfInput)
	}
}
