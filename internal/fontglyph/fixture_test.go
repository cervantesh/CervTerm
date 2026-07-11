package fontglyph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRedistributableSVGGradientFixture(t *testing.T) {
	data := readFixture(t, "svg-gradient-table.bin")
	extractor, err := newSVGExtractor(data)
	if err != nil {
		t.Fatalf("newSVGExtractor: %v", err)
	}
	doc, ok := extractor.document(10)
	if !ok {
		t.Fatalf("expected SVG document for fixture glyph")
	}
	img, ok := rasterizeSVGDocument(doc, 20, 20)
	if !ok {
		t.Fatalf("expected fixture SVG document to rasterize")
	}
	left := img.RGBAAt(1, 10)
	right := img.RGBAAt(18, 10)
	if left.R <= left.B || right.B <= right.R {
		t.Fatalf("expected red-to-blue fixture gradient, left=%#v right=%#v", left, right)
	}
}

func TestRedistributableSVGTextFixture(t *testing.T) {
	data := readFixture(t, "svg-text-table.bin")
	extractor, err := newSVGExtractor(data)
	if err != nil {
		t.Fatalf("newSVGExtractor: %v", err)
	}
	doc, ok := extractor.document(20)
	if !ok {
		t.Fatalf("expected SVG text document for fixture glyph")
	}
	img, ok := rasterizeSVGDocument(doc, 100, 40)
	if !ok {
		t.Fatalf("expected fixture SVG text document to rasterize")
	}
	if countColoredPixels(img, 30, 70) == 0 {
		t.Fatalf("expected centered text pixels in fixture")
	}
}

func TestRedistributableCOLRVarScaleFixture(t *testing.T) {
	colr := readFixture(t, "colr-var-scale-table.bin")
	cpal := readFixture(t, "cpal-red-green.bin")
	parser, err := newCOLRParser(colr, cpal)
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	parser.variationCoords = []float64{0.5}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	want := COLRTransform{XX: 1.5, YY: 0.75}
	assertTransformClose(t, glyph.Layers[0].Transform, want)
}

func TestRedistributableCOLRCompositeFixture(t *testing.T) {
	colr := readFixture(t, "colr-composite-multiply-table.bin")
	cpal := readFixture(t, "cpal-red-green.bin")
	parser, err := newCOLRParser(colr, cpal)
	if err != nil {
		t.Fatalf("newCOLRParser: %v", err)
	}
	glyph, err := parser.glyph(10, 0)
	if err != nil {
		t.Fatalf("glyph: %v", err)
	}
	if len(glyph.Layers) != 1 || glyph.Layers[0].Fill != COLRFillComposite || glyph.Layers[0].CompositeMode != colrCompositeMultiply {
		t.Fatalf("unexpected composite fixture glyph: %#v", glyph.Layers)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
