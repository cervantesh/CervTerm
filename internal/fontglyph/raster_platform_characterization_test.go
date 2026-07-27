package fontglyph

import (
	"crypto/sha256"
	"fmt"
	"image"
	"os"
	"reflect"
	"runtime"
	"testing"

	rasterpkg "cervterm/internal/fontglyph/raster"
)

func TestL402RasterColorFixtureCharacterization(t *testing.T) {
	data := readFixture(t, "noto-color-emoji-smoke.ttf")
	face, _, err := loadOpenTypeFace(data, Spec{Family: "Noto Color Emoji", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("loadOpenTypeFace: %v", err)
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96, TextRaster: "go"})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.faces = append([]loadedFace{backend.faces[0], face}, backend.faces[1:]...)
	defer backend.Close()

	glyph, ok := backend.Rasterize('😀', 2)
	if !ok || glyph.Image == nil || !glyph.HasColor {
		t.Fatalf("Rasterize emoji = %#v, %t", glyph, ok)
	}
	got := rasterCharacterization(glyph)
	const want = "b829c7ebfef8b9d6b21ee81c9961f78fd54c4b049038a7b68ebea041731d6b3a"
	if got != want {
		t.Fatalf("emoji characterization = %s, want %s", got, want)
	}
}

func TestL402SVGGradientFixtureCharacterization(t *testing.T) {
	img, ok := rasterpkg.RasterizeSVGTableGlyph(readFixture(t, "svg-gradient-table.bin"), 10, 48, 32)
	if !ok {
		t.Fatal("SVG fixture did not rasterize")
	}
	got := rgbaCharacterization(img)
	const want = "ffd73d9d6502421a5552bac40b57f8ba7cd10a5ff0c50dd0e2d9e6a4721f4e0f"
	if got != want {
		t.Fatalf("SVG characterization = %s, want %s", got, want)
	}
}

func TestL402ConcreteCompatibilitySurface(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "go"})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	defer backend.Close()
	checks := map[string]string{
		"backend": fmt.Sprintf("%T", backend),
		"shaper":  fmt.Sprintf("%T", backend.shaper),
		"glyph":   reflect.TypeOf(RasterizedGlyph{}).PkgPath() + "." + reflect.TypeOf(RasterizedGlyph{}).Name(),
		"tables":  reflect.TypeOf(ColorTables{}).PkgPath() + "." + reflect.TypeOf(ColorTables{}).Name(),
	}
	wantShaper := "fontglyph.shapeToRootShaper"
	if runtime.GOOS == "windows" {
		wantShaper = "fontglyph.DirectWriteShaper"
	}
	want := map[string]string{
		"backend": "*fontglyph.OpenTypeBackend",
		"shaper":  wantShaper,
		"glyph":   "cervterm/internal/fontglyph.RasterizedGlyph",
		"tables":  "cervterm/internal/fontglyph.ColorTables",
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("compatibility surface = %#v, want %#v", checks, want)
	}
}

func BenchmarkL402RasterColorGlyph(b *testing.B) {
	data, err := os.ReadFile("testdata/noto-color-emoji-smoke.ttf")
	if err != nil {
		b.Fatal(err)
	}
	face, _, err := loadOpenTypeFace(data, Spec{Family: "Noto Color Emoji", Size: 18, DPI: 96})
	if err != nil {
		b.Fatal(err)
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96, TextRaster: "go"})
	if err != nil {
		b.Fatal(err)
	}
	backend.faces = append([]loadedFace{backend.faces[0], face}, backend.faces[1:]...)
	defer backend.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := backend.Rasterize('😀', 2); !ok {
			b.Fatal("emoji did not rasterize")
		}
	}
}

func rasterCharacterization(glyph RasterizedGlyph) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d,%d,%d,%d,%.9f,%d,%t,%t|", glyph.Width, glyph.Height, glyph.BearingX, glyph.BearingY, glyph.AdvanceX, glyph.CellSpan, glyph.HasColor, glyph.Subpixel)
	writeRGBACharacterization(h, glyph.Image)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func rgbaCharacterization(img *image.RGBA) string {
	h := sha256.New()
	writeRGBACharacterization(h, img)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func writeRGBACharacterization(h interface{ Write([]byte) (int, error) }, img *image.RGBA) {
	if img == nil {
		_, _ = h.Write([]byte("nil"))
		return
	}
	_, _ = h.Write([]byte(fmt.Sprintf("%v,%d|", img.Rect, img.Stride)))
	_, _ = h.Write(img.Pix)
}
