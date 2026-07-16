//go:build windows

package fontglyph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestOpenTypeBackendRasterizesPiSpinnerWithSymbolFallback(t *testing.T) {
	fontDir := filepath.Join(os.Getenv("SystemRoot"), "Fonts")
	if fontDir == "Fonts" {
		fontDir = `C:\Windows\Fonts`
	}
	symbolPath := filepath.Join(fontDir, "seguisym.ttf")
	if _, err := os.Stat(symbolPath); err != nil {
		t.Fatalf("Pi spinner fallback font is unavailable at %s: %v", symbolPath, err)
	}

	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cellW, cellH, _ := backend.CellMetrics()
	for _, r := range []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		r := r
		t.Run(string(r), func(t *testing.T) {
			face, _, _, ok := backend.faceForRune(r)
			if !ok {
				t.Fatalf("no fallback face covers %q (U+%04X)", r, r)
			}
			var buf sfnt.Buffer
			glyphID, err := face.sfnt.GlyphIndex(&buf, r)
			if err != nil || glyphID == 0 {
				t.Fatalf("fallback face %s has no glyph for %q (U+%04X): id=%d err=%v", face.sourcePath, r, r, glyphID, err)
			}
			if !strings.EqualFold(filepath.Base(face.sourcePath), "seguisym.ttf") {
				t.Fatalf("spinner frame selected %s, want Segoe UI Symbol", face.sourcePath)
			}

			glyph, ok := backend.Rasterize(r, 1)
			if !ok || glyph.Image == nil {
				t.Fatalf("spinner frame %q (U+%04X) did not rasterize", r, r)
			}
			if glyph.CellSpan != 1 {
				t.Fatalf("spinner frame %q cell span = %d, want 1", r, glyph.CellSpan)
			}
			if got := glyph.Image.Bounds().Dx(); got != cellW {
				t.Fatalf("spinner frame %q canvas width = %d, want %d", r, got, cellW)
			}
			if got := glyph.Image.Bounds().Dy(); got != cellH {
				t.Fatalf("spinner frame %q canvas height = %d, want %d", r, got, cellH)
			}
			if !hasOpaquePixel(glyph.Image) {
				t.Fatalf("spinner frame %q rasterized without visible pixels", r)
			}
		})
	}
}
