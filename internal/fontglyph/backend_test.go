package fontglyph

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestOpenTypeBackendRasterizesGlyphWithCellMetrics(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cellW, cellH, baseline := backend.CellMetrics()
	if cellW <= 0 || cellH <= 0 || baseline <= 0 || baseline >= cellH {
		t.Fatalf("invalid cell metrics: width=%d height=%d baseline=%d", cellW, cellH, baseline)
	}

	glyph, ok := backend.Rasterize('é', 1)
	if !ok {
		t.Fatalf("expected accented glyph to rasterize")
	}
	if glyph.Image == nil {
		t.Fatalf("rasterized glyph image is nil")
	}
	if got := glyph.Image.Bounds().Dx(); got != cellW {
		t.Fatalf("glyph canvas width = %d, want %d", got, cellW)
	}
	if got := glyph.Image.Bounds().Dy(); got != cellH {
		t.Fatalf("glyph canvas height = %d, want %d", got, cellH)
	}
	if glyph.CellSpan != 1 {
		t.Fatalf("accented glyph cell span = %d, want 1", glyph.CellSpan)
	}
	if glyph.Width <= 0 || glyph.Height <= 0 || glyph.AdvanceX <= 0 {
		t.Fatalf("expected positive glyph metrics, got width=%d height=%d advance=%f", glyph.Width, glyph.Height, glyph.AdvanceX)
	}
	if !hasOpaquePixel(glyph.Image) {
		t.Fatalf("rasterized glyph image has no visible pixels")
	}
}

func TestOpenTypeBackendWideGlyphCanvasUsesRequestedTwoCells(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cellW, _, _ := backend.CellMetrics()
	glyph, ok := backend.Rasterize('安', 2)
	if !ok {
		t.Skip("no CJK fallback font available on this system")
	}
	if glyph.CellSpan != 2 {
		t.Fatalf("CJK glyph cell span = %d, want 2", glyph.CellSpan)
	}
	if got, want := glyph.Image.Bounds().Dx(), cellW*2; got != want {
		t.Fatalf("CJK glyph canvas width = %d, want %d", got, want)
	}
}

func TestOpenTypeBackendRasterizesCombiningCluster(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	glyph, ok := backend.RasterizeCluster("e\u0301", 1)
	if !ok {
		t.Fatalf("expected combining cluster to rasterize")
	}
	if glyph.Image == nil || glyph.CellSpan != 1 || glyph.AdvanceX <= 0 {
		t.Fatalf("invalid cluster glyph: %#v", glyph)
	}
	if !hasOpaquePixel(glyph.Image) {
		t.Fatalf("rasterized cluster image has no visible pixels")
	}
}

func TestNormalizeClusterToSingleRuneComposesCombiningMark(t *testing.T) {
	r, ok := normalizeClusterToSingleRune("e\u0301")
	if !ok || r != 'é' {
		t.Fatalf("normalizeClusterToSingleRune = %q, %v; want é, true", r, ok)
	}
	if _, ok := normalizeClusterToSingleRune("ab"); ok {
		t.Fatalf("multi-rune cluster should not normalize to one rune")
	}
}

func TestOpenTypeBackendRasterizesEmojiZWJClusterSafely(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	glyph, ok := backend.RasterizeCluster("👩\u200d💻", 2)
	if !ok {
		t.Skip("no fallback font path could rasterize emoji ZWJ cluster on this host")
	}
	if glyph.Image == nil || glyph.CellSpan != 2 || glyph.AdvanceX <= 0 {
		t.Fatalf("invalid ZWJ cluster glyph: %#v", glyph)
	}
	if !glyph.HasColor {
		t.Fatalf("expected shaped ZWJ emoji cluster to use color glyph path, got %#v", glyph)
	}
	if !hasOpaquePixel(glyph.Image) {
		t.Fatalf("ZWJ cluster rasterized without visible pixels")
	}
}

func TestOpenTypeBackendPrefersBestEmojiFaceForEmojiClusters(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	for _, cluster := range []string{"😀", "❤️", "👩\u200d💻"} {
		t.Run(cluster, func(t *testing.T) {
			face, ok := backend.faceForCluster(cluster)
			if !ok {
				t.Fatalf("faceForCluster(%q) failed", cluster)
			}
			if seg, ok := backend.segoeEmojiFace(); ok && !strings.EqualFold(filepath.Clean(face.sourcePath), filepath.Clean(seg.sourcePath)) {
				t.Fatalf("faceForCluster(%q) = %q, want Segoe UI Emoji for non-flag emoji", cluster, face.sourcePath)
			}
		})
	}
	if noto, ok := backend.notoColorEmojiFace(); ok {
		face, ok := backend.faceForCluster("🇦🇷")
		if !ok {
			t.Fatalf("faceForCluster(flag) failed")
		}
		if !strings.EqualFold(filepath.Clean(face.sourcePath), filepath.Clean(noto.sourcePath)) {
			t.Fatalf("faceForCluster(flag) = %q, want Noto Color Emoji %q", face.sourcePath, noto.sourcePath)
		}
	}
}

func TestOpenTypeBackendRasterizesSegoeEmojiAsFullColorGlyph(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if _, ok := backend.segoeEmojiFace(); !ok {
		t.Skip("Segoe UI Emoji is not available on this host")
	}
	glyph, ok := backend.RasterizeCluster("😀", 2)
	if !ok {
		t.Fatalf("expected Segoe emoji cluster to rasterize")
	}
	if !glyph.HasColor {
		t.Fatalf("expected color emoji glyph, got %#v", glyph)
	}
	if opaque := countOpaquePixels(glyph.Image); opaque < 100 {
		t.Fatalf("emoji glyph has too few opaque pixels (%d), likely partial COLR render", opaque)
	}
}

func TestOpenTypeBackendFitsSegoeEmojiToCellCanvas(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if _, ok := backend.segoeEmojiFace(); !ok {
		t.Skip("Segoe UI Emoji is not available on this host")
	}
	cellW, cellH, _ := backend.CellMetrics()
	for _, sample := range []string{"🍣", "✈️", "🚲", "⚽", "🎸"} {
		t.Run(sample, func(t *testing.T) {
			glyph, ok := backend.RasterizeCluster(sample, 2)
			if !ok {
				t.Fatalf("RasterizeCluster(%q) failed", sample)
			}
			bounds, ok := visibleRGBABounds(glyph.Image)
			if !ok {
				t.Fatalf("RasterizeCluster(%q) produced no visible pixels", sample)
			}
			if bounds.Dx() < cellW*2-3 && bounds.Dy() < cellH-3 {
				t.Fatalf("RasterizeCluster(%q) visible bounds = %dx%d, want width near %d or height near %d", sample, bounds.Dx(), bounds.Dy(), cellW*2, cellH)
			}
		})
	}
}

func TestOpenTypeBackendUsesConfiguredShaper(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	shaper := &testShaper{}
	backend.SetShaper(shaper)
	glyph, ok := backend.RasterizeCluster("A", 1)
	if !ok {
		t.Fatalf("expected shaped cluster to rasterize")
	}
	if !shaper.called {
		t.Fatalf("expected configured shaper to be called")
	}
	if glyph.Image == nil || glyph.AdvanceX <= 0 || !hasOpaquePixel(glyph.Image) {
		t.Fatalf("invalid shaped glyph: %#v", glyph)
	}
}

func TestDiagnoseEmojiFontsReportsInstalledFallbacks(t *testing.T) {
	diag := DiagnoseEmojiFonts()
	if diag.NotoColorEmojiPath == "" && diag.SegoeEmojiPath == "" && len(diag.Warnings) == 0 {
		t.Fatalf("expected emoji font diagnostic path or warning, got %#v", diag)
	}
	if diag.NotoColorEmojiPath == "" && len(diag.Warnings) == 0 {
		t.Fatalf("missing Noto should produce a warning")
	}
}

type testShaper struct {
	called bool
}

func (s *testShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	s.called = true
	if face.sfnt == nil {
		return nil, false
	}
	var buf sfnt.Buffer
	glyphID, err := face.sfnt.GlyphIndex(&buf, 'A')
	if err != nil || glyphID == 0 {
		return nil, false
	}
	return []ShapedGlyph{{GlyphID: uint16(glyphID), XAdvance: 10}}, true
}

func hasOpaquePixel(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0 {
				return true
			}
		}
	}
	return false
}

func countOpaquePixels(img *image.RGBA) int {
	if img == nil {
		return 0
	}
	count := 0
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0 {
				count++
			}
		}
	}
	return count
}
