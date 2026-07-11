package fontglyph

import (
	"bytes"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestNotoColorEmojiSubsetFixtureHasLicenseProvenance(t *testing.T) {
	license := readFixture(t, "NotoEmoji-LICENSE.txt")
	if !bytes.Contains(license, []byte("SIL Open Font License")) {
		t.Fatalf("expected OFL license text, got %q", string(license))
	}
	provenance := readFixture(t, "noto-color-emoji-smoke.provenance.txt")
	for _, marker := range [][]byte{[]byte("source=https://github.com/googlefonts/noto-emoji"), []byte("subset_tool=pyftsubset"), []byte("subset_text=")} {
		if !bytes.Contains(provenance, marker) {
			t.Fatalf("provenance missing %q: %q", marker, string(provenance))
		}
	}
}

func TestNotoColorEmojiSubsetFixtureRasterizesColorGlyph(t *testing.T) {
	data := readFixture(t, "noto-color-emoji-smoke.ttf")
	tables, err := DetectColorTables(data)
	if err != nil {
		t.Fatalf("DetectColorTables: %v", err)
	}
	if !tables.HasAnyColor() {
		t.Fatalf("expected color tables in Noto subset, got %#v", tables)
	}
	face, _, err := loadOpenTypeFace(data, Spec{Family: "Noto Color Emoji", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("loadOpenTypeFace subset: %v", err)
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.faces = append([]loadedFace{backend.faces[0], face}, backend.faces[1:]...)
	glyph, ok := backend.Rasterize('😀', 2)
	if !ok {
		t.Fatalf("expected emoji glyph to rasterize from Noto subset")
	}
	if !glyph.HasColor || !hasOpaquePixel(glyph.Image) {
		t.Fatalf("expected color glyph with visible pixels, got %#v", glyph)
	}
}

func TestNotoColorEmojiSubsetRasterizesMultiGlyphShapedColorCluster(t *testing.T) {
	data := readFixture(t, "noto-color-emoji-smoke.ttf")
	face, _, err := loadOpenTypeFace(data, Spec{Family: "Noto Color Emoji", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("loadOpenTypeFace subset: %v", err)
	}
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.faces = append([]loadedFace{backend.faces[0], face}, backend.faces[1:]...)
	backend.SetShaper(colorPairShaper{first: '😀', second: '🚀'})
	glyph, ok := backend.RasterizeCluster("😀🚀", 4)
	if !ok {
		t.Fatalf("expected shaped emoji pair to rasterize from Noto subset")
	}
	if !glyph.HasColor || !hasOpaquePixel(glyph.Image) {
		t.Fatalf("expected visible color multi-glyph cluster, got %#v", glyph)
	}
}

type colorPairShaper struct {
	first  rune
	second rune
}

func (s colorPairShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if face.sfnt == nil {
		return nil, false
	}
	var buf sfnt.Buffer
	first, err := face.sfnt.GlyphIndex(&buf, s.first)
	if err != nil || first == 0 {
		return nil, false
	}
	second, err := face.sfnt.GlyphIndex(&buf, s.second)
	if err != nil || second == 0 {
		return nil, false
	}
	return []ShapedGlyph{
		{GlyphID: uint16(first), XAdvance: 18},
		{GlyphID: uint16(second), XAdvance: 18},
	}, true
}
