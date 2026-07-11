//go:build windows

package fontglyph

import "testing"

func TestDirectWriteBridgeShapesSimpleText(t *testing.T) {
	// Enabled while stabilizing DirectWrite GetGlyphs/GetGlyphPlacements.
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	var path string
	for _, face := range backend.faces[1:] {
		if face.sourcePath != "" {
			path = face.sourcePath
			break
		}
	}
	if path == "" {
		t.Skip("no fallback system font with source path loaded on this host")
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Fatalf("newDirectWriteFactory: %v", err)
	}
	defer factory.release()
	fontFace, err := factory.createFontFaceFromPath(path)
	if err != nil {
		t.Fatalf("createFontFaceFromPath(%s): %v", path, err)
	}
	defer fontFace.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		t.Fatalf("createTextAnalyzer: %v", err)
	}
	defer analyzer.release()
	glyphs, ok, err := analyzer.shapeText("ABC", fontFace, backend.ppem)
	if err != nil {
		t.Fatalf("shapeText: %v", err)
	}
	if !ok || len(glyphs) == 0 {
		t.Fatalf("expected shaped glyphs, got %#v ok=%v", glyphs, ok)
	}
	for _, glyph := range glyphs {
		if glyph.GlyphID == 0 || glyph.XAdvance <= 0 {
			t.Fatalf("invalid shaped glyph: %#v", glyph)
		}
	}
}
