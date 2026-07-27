//go:build windows

package platform

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/gofont/gomono"
)

func TestDirectWriteCreatesNonzeroCollectionFace(t *testing.T) {
	if !directWriteAvailable() {
		t.Skip("DirectWrite unavailable")
	}
	path := filepath.Join(t.TempDir(), "two-face.ttc")
	if err := os.WriteFile(path, makePlatformTestTTC(t, gomono.TTF, gomono.TTF), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Skipf("DirectWrite factory unavailable: %v", err)
	}
	defer factory.release()
	face, err := factory.createFontFaceFromPathIndex(path, 1)
	if err != nil {
		t.Fatalf("create collection face 1: %v", err)
	}
	face.release()
	rasterizer, err := newDWriteRasterizer(path, 1, 14, 96)
	if err != nil {
		t.Fatalf("create raster collection face 1: %v", err)
	}
	rasterizer.Close()
	if rasterizer, err := newDWriteRasterizer(path, 2, 14, 96); err == nil {
		rasterizer.Close()
		t.Fatal("out-of-range raster collection face unexpectedly resolved")
	}
	if _, err := factory.createFontFaceFromPathIndex(path, 2); err == nil {
		t.Fatal("out-of-range collection face unexpectedly resolved")
	}
	if _, err := factory.createFontFaceFromPathIndex(path, -1); err == nil {
		t.Fatal("negative collection index accepted")
	}
}

func TestDirectWriteRasterConstructorValidatesBeforeNativeCalls(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		faceIndex int
		size      float64
		dpi       float64
	}{
		{name: "empty path", path: "", faceIndex: 0, size: 14, dpi: 96},
		{name: "negative face", path: "fixture.ttf", faceIndex: -1, size: 14, dpi: 96},
		{name: "excessive face", path: "fixture.ttf", faceIndex: fontdesc.MaxFacesPerFile, size: 14, dpi: 96},
		{name: "zero size", path: "fixture.ttf", faceIndex: 0, size: 0, dpi: 96},
		{name: "negative size", path: "fixture.ttf", faceIndex: 0, size: -1, dpi: 96},
		{name: "NaN size", path: "fixture.ttf", faceIndex: 0, size: math.NaN(), dpi: 96},
		{name: "infinite size", path: "fixture.ttf", faceIndex: 0, size: math.Inf(1), dpi: 96},
		{name: "zero DPI", path: "fixture.ttf", faceIndex: 0, size: 14, dpi: 0},
		{name: "negative DPI", path: "fixture.ttf", faceIndex: 0, size: 14, dpi: -1},
		{name: "NaN DPI", path: "fixture.ttf", faceIndex: 0, size: 14, dpi: math.NaN()},
		{name: "infinite DPI", path: "fixture.ttf", faceIndex: 0, size: 14, dpi: math.Inf(1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nativeCalls := 0
			createFactory := func(uintptr) (*iWriteFactory, error) {
				nativeCalls++
				return nil, fmt.Errorf("unexpected native call")
			}
			rasterizer, err := newDWriteRasterizerWithFactory(test.path, test.faceIndex, test.size, test.dpi, createFactory)
			if rasterizer != nil {
				rasterizer.Close()
				t.Fatal("invalid request returned a rasterizer")
			}
			if err == nil {
				t.Fatal("invalid request was accepted")
			}
			if nativeCalls != 0 {
				t.Fatalf("native factory calls = %d, want 0", nativeCalls)
			}
		})
	}
}

func TestDirectWriteRasterFaceIndexCollectionBounds(t *testing.T) {
	for _, test := range []struct {
		index int
		count uint32
		valid bool
	}{{0, 1, true}, {1, 2, true}, {-1, 2, false}, {2, 2, false}, {fontdesc.MaxFacesPerFile, 300, false}} {
		err := validateDWriteFaceIndex(test.index, test.count)
		if (err == nil) != test.valid {
			t.Fatalf("validate face index %d count %d: err=%v valid=%t", test.index, test.count, err, test.valid)
		}
	}
}

func TestDirectWriteBridgeCreatesAndShapesFontFace(t *testing.T) {
	if !directWriteAvailable() {
		t.Skip("DirectWrite unavailable")
	}
	path := filepath.Join(t.TempDir(), "gomono.ttf")
	if err := os.WriteFile(path, gomono.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		t.Fatalf("newDirectWriteFactory: %v", err)
	}
	defer factory.release()
	fontFace, err := factory.createFontFaceFromPath(path)
	if err != nil {
		t.Fatalf("createFontFaceFromPath: %v", err)
	}
	defer fontFace.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		t.Fatalf("createTextAnalyzer: %v", err)
	}
	defer analyzer.release()
	glyphs, ok, err := analyzer.shapeText("ABC", fontFace, 18, fontdesc.FeatureSet{})
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

func makePlatformTestTTC(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	headerLen := (12 + 4*len(fonts) + 3) &^ 3
	total := headerLen
	for _, fontData := range fonts {
		total = (total + len(fontData) + 3) &^ 3
	}
	out := make([]byte, total)
	copy(out[:4], "ttcf")
	binary.BigEndian.PutUint32(out[4:8], 0x00010000)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(fonts)))
	offset := headerLen
	for i, fontData := range fonts {
		binary.BigEndian.PutUint32(out[12+i*4:16+i*4], uint32(offset))
		copy(out[offset:], fontData)
		fontCopy := out[offset : offset+len(fontData)]
		if len(fontCopy) < 12 {
			t.Fatal("invalid sfnt fixture")
		}
		numTables := int(binary.BigEndian.Uint16(fontCopy[4:6]))
		if 12+numTables*16 > len(fontCopy) {
			t.Fatal("invalid sfnt table directory")
		}
		for table := 0; table < numTables; table++ {
			field := 12 + table*16 + 8
			tableOffset := binary.BigEndian.Uint32(fontCopy[field : field+4])
			binary.BigEndian.PutUint32(fontCopy[field:field+4], tableOffset+uint32(offset))
		}
		offset = (offset + len(fontData) + 3) &^ 3
	}
	return out
}
