package fontglyph

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func TestSVGExtractorReturnsDocumentForGlyphRange(t *testing.T) {
	extractor, err := newSVGExtractor(fakeSVGTable())
	if err != nil {
		t.Fatalf("newSVGExtractor: %v", err)
	}
	doc, ok := extractor.document(11)
	if !ok {
		t.Fatalf("expected SVG document for glyph 11")
	}
	if string(doc) != `<svg><text>emoji</text></svg>` {
		t.Fatalf("document = %q", string(doc))
	}
	if _, ok := extractor.document(20); ok {
		t.Fatalf("unexpected SVG document for glyph outside range")
	}
}

func TestSVGExtractorRejectsInvalidTable(t *testing.T) {
	if _, err := newSVGExtractor([]byte{0, 0, 0}); err != ErrInvalidSVGTable {
		t.Fatalf("expected ErrInvalidSVGTable, got %v", err)
	}
}

func TestRasterizeSVGDocumentRect(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 10 10"><rect x="2" y="2" width="6" height="6" fill="#ff0000"/></svg>`), 20, 20)
	if !ok {
		t.Fatalf("expected simple SVG rect to rasterize")
	}
	inside := img.RGBAAt(10, 10)
	if inside.R == 0 || inside.A == 0 {
		t.Fatalf("expected red pixel inside rect, got %#v", inside)
	}
	outside := img.RGBAAt(1, 1)
	if outside.A != 0 {
		t.Fatalf("expected transparent outside rect, got %#v", outside)
	}
}

func TestRasterizeSVGDocumentCircle(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="blue" opacity="0.5"/></svg>`), 20, 20)
	if !ok {
		t.Fatalf("expected simple SVG circle to rasterize")
	}
	center := img.RGBAAt(10, 10)
	if center.B == 0 || center.A == 0 || center.A >= 255 {
		t.Fatalf("expected semi-transparent blue pixel at center, got %#v", center)
	}
}

func TestRasterizeSVGDocumentPath(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 10 10"><path d="M 1 9 L 5 1 L 9 9 Z" fill="lime"/></svg>`), 20, 20)
	if !ok {
		t.Fatalf("expected simple SVG path to rasterize")
	}
	inside := img.RGBAAt(10, 10)
	if inside.G == 0 || inside.A == 0 {
		t.Fatalf("expected green pixel inside path, got %#v", inside)
	}
	outside := img.RGBAAt(1, 1)
	if outside.A != 0 {
		t.Fatalf("expected transparent outside path, got %#v", outside)
	}
}

func TestRasterizeSVGDocumentQuadraticCurvePath(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 10 10"><path d="M 1 9 Q 5 1 9 9 Z" fill="lime"/></svg>`), 20, 20)
	if !ok {
		t.Fatalf("expected quadratic curve SVG path to rasterize")
	}
	inside := img.RGBAAt(10, 10)
	if inside.G == 0 || inside.A == 0 {
		t.Fatalf("expected green pixel inside quadratic path, got %#v", inside)
	}
}

func TestRasterizeSVGDocumentCubicCurvePath(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 10 10"><path d="M 1 9 C 3 1 7 1 9 9 Z" fill="blue"/></svg>`), 20, 20)
	if !ok {
		t.Fatalf("expected cubic curve SVG path to rasterize")
	}
	inside := img.RGBAAt(10, 10)
	if inside.B == 0 || inside.A == 0 {
		t.Fatalf("expected blue pixel inside cubic path, got %#v", inside)
	}
}

func TestRasterizeSVGDocumentLinearGradient(t *testing.T) {
	doc := []byte(`<svg viewBox="0 0 10 10"><defs><linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="0%"><stop offset="0%" stop-color="red"/><stop offset="100%" stop-color="blue"/></linearGradient></defs><rect x="0" y="0" width="10" height="10" fill="url(#g)"/></svg>`)
	img, ok := rasterizeSVGDocument(doc, 20, 20)
	if !ok {
		t.Fatalf("expected SVG linear gradient to rasterize")
	}
	left := img.RGBAAt(1, 10)
	right := img.RGBAAt(18, 10)
	if left.R <= left.B || right.B <= right.R {
		t.Fatalf("expected red-to-blue gradient, left=%#v right=%#v", left, right)
	}
}

func TestRasterizeSVGDocumentText(t *testing.T) {
	img, ok := rasterizeSVGDocument([]byte(`<svg viewBox="0 0 40 20"><text x="2" y="14" fill="red">Hi</text></svg>`), 40, 20)
	if !ok {
		t.Fatalf("expected simple SVG text to rasterize")
	}
	found := false
	for y := 0; y < 20 && !found; y++ {
		for x := 0; x < 40; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 0 && c.A > 0 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected red text pixels")
	}
}

func TestRasterizeSVGDocumentTextLayout(t *testing.T) {
	doc := []byte(`<svg viewBox="0 0 100 40"><text x="50" y="20" font-size="20" text-anchor="middle" dominant-baseline="middle" fill="blue"><tspan>Hi</tspan> <tspan>Go</tspan></text></svg>`)
	img, ok := rasterizeSVGDocument(doc, 100, 40)
	if !ok {
		t.Fatalf("expected laid-out SVG text to rasterize")
	}
	left := countColoredPixels(img, 0, 20)
	right := countColoredPixels(img, 80, 100)
	if left != 0 || right != 0 {
		t.Fatalf("middle-anchored text should stay near center, left=%d right=%d", left, right)
	}
	center := countColoredPixels(img, 30, 70)
	if center == 0 {
		t.Fatalf("expected centered text pixels")
	}
}

func countColoredPixels(img interface{ RGBAAt(x, y int) color.RGBA }, minX, maxX int) int {
	count := 0
	for y := 0; y < 40; y++ {
		for x := minX; x < maxX; x++ {
			if img.RGBAAt(x, y).A != 0 {
				count++
			}
		}
	}
	return count
}

func TestRasterizeSVGDocumentUnsupportedReturnsFalse(t *testing.T) {
	if _, ok := rasterizeSVGDocument([]byte(`<svg><ellipse cx="5" cy="5" rx="3" ry="2" fill="red"/></svg>`), 20, 20); ok {
		t.Fatalf("unsupported ellipse-only SVG should not report rasterized pixels")
	}
}

func fakeSVGTable() []byte {
	doc := []byte(`<svg><text>emoji</text></svg>`)
	const documentListOffset = 10
	const documentOffsetFromList = 14
	data := make([]byte, documentListOffset+documentOffsetFromList+len(doc))
	binary.BigEndian.PutUint16(data[0:2], 0)
	binary.BigEndian.PutUint32(data[2:6], documentListOffset)
	binary.BigEndian.PutUint16(data[documentListOffset:documentListOffset+2], 1)
	entry := data[documentListOffset+2 : documentListOffset+14]
	binary.BigEndian.PutUint16(entry[0:2], 10)
	binary.BigEndian.PutUint16(entry[2:4], 12)
	binary.BigEndian.PutUint32(entry[4:8], documentOffsetFromList)
	binary.BigEndian.PutUint32(entry[8:12], uint32(len(doc)))
	copy(data[documentListOffset+documentOffsetFromList:], doc)
	return data
}
