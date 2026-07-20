//go:build windows

package fontglyph

import (
	"testing"

	"cervterm/internal/fontdesc"
)

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
	glyphs, ok, err := analyzer.shapeText("ABC", fontFace, backend.ppem, fontdesc.FeatureSet{})
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

func TestDirectWriteFeatureArgumentsPreserveSortedTagsAndValues(t *testing.T) {
	features, err := fontdesc.NewFeatureSet(true, map[string]int{"liga": 0, "ss01": 7})
	if err != nil {
		t.Fatal(err)
	}
	arguments := newDirectWriteFeatureArguments(features, 4)
	entries := features.Entries()
	if len(arguments.entries) != len(entries) || len(arguments.pointers) != 1 || arguments.rangeLengths[0] != 4 {
		t.Fatalf("arguments=%#v", arguments)
	}
	for index, feature := range entries {
		if arguments.entries[index].Name != directWriteFeatureTag(feature.Tag) || arguments.entries[index].Parameter != uint32(feature.Value) {
			t.Fatalf("entry %d=%#v want %#v", index, arguments.entries[index], feature)
		}
	}
	if directWriteFeatureTag("liga") != uint32('l')|uint32('i')<<8|uint32('g')<<16|uint32('a')<<24 {
		t.Fatal("DirectWrite OpenType tag byte order changed")
	}
	featurePointer, lengths, ranges := arguments.callPointers()
	if featurePointer == 0 || lengths == 0 || ranges != 1 {
		t.Fatalf("call pointers=%x/%x/%d", featurePointer, lengths, ranges)
	}
}
