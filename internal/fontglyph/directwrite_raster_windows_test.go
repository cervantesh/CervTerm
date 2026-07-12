//go:build windows

package fontglyph

import (
	"bytes"
	"image"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestDWriteRasterizer(t *testing.T) {
	t.Setenv("LocalAppData", t.TempDir())
	fontPath, err := cachedGoMonoPath()
	if err != nil {
		t.Fatalf("cachedGoMonoPath: %v", err)
	}
	raster, err := newDWriteRasterizer(fontPath, 0, 14, 96)
	if err != nil {
		t.Fatalf("newDWriteRasterizer: %v", err)
	}
	defer raster.Close()
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "go"})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	defer backend.Close()
	var directWriteLetter *image.RGBA
	for _, tt := range []struct {
		name        string
		r           rune
		wantVisible bool
	}{{"letter", 'd', true}, {"space", ' ', false}} {
		t.Run(tt.name, func(t *testing.T) {
			var buf sfnt.Buffer
			id, idErr := backend.faces[0].sfnt.GlyphIndex(&buf, tt.r)
			if idErr != nil {
				t.Fatalf("GlyphIndex(%q): %v", tt.r, idErr)
			}
			img, ok := raster.RasterizeGlyph(uint16(id), backend.cellW, backend.cellH, backend.baseline, 1)
			if !ok || img.Bounds() != image.Rect(0, 0, backend.cellW, backend.cellH) {
				t.Fatalf("RasterizeGlyph(%q) ok=%t bounds=%v", tt.r, ok, img.Bounds())
			}
			visible := false
			for i := 0; i < len(img.Pix); i += 4 {
				r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
				if r != g || r != b || r != a {
					t.Fatalf("pixel %d is not premultiplied white: %v", i/4, img.Pix[i:i+4])
				}
				visible = visible || a != 0
			}
			if visible != tt.wantVisible {
				t.Fatalf("visible=%t, want %t", visible, tt.wantVisible)
			}
			if tt.r == 'd' {
				directWriteLetter = img
			}
		})
	}
	goGlyph, ok := backend.Rasterize('x', 1)
	if !ok || directWriteLetter == nil || bytes.Equal(directWriteLetter.Pix, goGlyph.Image.Pix) {
		t.Fatal("DirectWrite 'd' coverage unexpectedly matches Go-path 'x' coverage")
	}
}

func TestNewDWriteRasterizerRejectsMissingFile(t *testing.T) {
	if raster, err := newDWriteRasterizer(filepath.Join(t.TempDir(), "missing.ttf"), 0, 14, 96); err == nil {
		raster.Close()
		t.Fatal("newDWriteRasterizer succeeded for missing file")
	}
}

func TestBackendTextRasterModes(t *testing.T) {
	t.Setenv("LocalAppData", t.TempDir())
	for _, mode := range []string{"auto", "go"} {
		t.Run(mode, func(t *testing.T) {
			backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: mode})
			if err != nil {
				t.Fatalf("NewOpenTypeBackend: %v", err)
			}
			defer backend.Close()
			if got := backend.dwRaster != nil; got != (mode == "auto") {
				t.Fatalf("dwRaster present=%t for mode %q", got, mode)
			}
			glyph, ok := backend.Rasterize('d', 1)
			if !ok || glyph.Image == nil {
				t.Fatalf("Rasterize('d') ok=%t image=%v", ok, glyph.Image)
			}
		})
	}
}
