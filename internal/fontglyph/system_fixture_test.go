package fontglyph

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSystemColorEmojiFixtureDetection(t *testing.T) {
	path := firstExistingSystemColorEmojiFont()
	if path == "" {
		t.Skip("no known system color emoji font fixture found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read system color emoji fixture %s: %v", path, err)
	}
	tables, err := DetectColorTables(data)
	if err != nil {
		t.Fatalf("DetectColorTables(%s): %v", path, err)
	}
	if !tables.HasAnyColor() {
		t.Fatalf("%s was expected to contain a color glyph table, got %#v", path, tables)
	}
	if runtime.GOOS == "windows" && filepath.Base(path) == "seguiemj.ttf" && (!tables.HasCOLRVersion || tables.COLRVersion == 0) {
		t.Fatalf("Segoe UI Emoji should expose COLRv1 on current Windows systems, got %#v", tables)
	}
}

func TestSystemColorEmojiRasterizesKnownGlyph(t *testing.T) {
	path := firstExistingSystemColorEmojiFont()
	if path == "" {
		t.Skip("no known system color emoji font fixture found")
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	glyph, ok := backend.Rasterize('😀', 2)
	if !ok {
		t.Fatalf("expected known emoji glyph to rasterize using system color font fixture %s", path)
	}
	if !glyph.HasColor {
		t.Fatalf("expected emoji glyph to use color path, got %#v", glyph)
	}
	if !hasOpaquePixel(glyph.Image) {
		t.Fatalf("expected rasterized emoji glyph to contain visible pixels")
	}
}

func TestSystemColorEmojiRasterizesRepresentativeGlyphs(t *testing.T) {
	path := firstExistingSystemColorEmojiFont()
	if path == "" {
		t.Skip("no known system color emoji font fixture found")
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	samples := []rune{'😀', '🚀', '❤', '☀', '⭐'}
	colorCount := 0
	for _, sample := range samples {
		glyph, ok := backend.Rasterize(sample, 2)
		if ok && glyph.HasColor && hasOpaquePixel(glyph.Image) {
			colorCount++
		}
	}
	if colorCount < 4 {
		t.Fatalf("expected at least 4 representative emoji glyphs to rasterize through color paths using %s, got %d/%d", path, colorCount, len(samples))
	}
}

func firstExistingSystemColorEmojiFont() string {
	for _, path := range systemColorEmojiFontCandidates() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func systemColorEmojiFontCandidates() []string {
	var paths []string
	if runtime.GOOS == "windows" {
		fontDir := filepath.Join(os.Getenv("SystemRoot"), "Fonts")
		if fontDir == "Fonts" {
			fontDir = `C:\Windows\Fonts`
		}
		paths = append(paths, filepath.Join(fontDir, "seguiemj.ttf"))
	}
	paths = append(paths,
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/google-noto-emoji/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	)
	return paths
}
