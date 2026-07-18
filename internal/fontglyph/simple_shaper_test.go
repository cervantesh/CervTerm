package fontglyph

import (
	"testing"

	"cervterm/internal/fontdesc"
)

func TestSimpleShaperShapesSingleRuneToGlyphID(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	face := backend.faces[0]
	glyphs, ok := (SimpleShaper{}).Shape("A", face, backend.ppem)
	if !ok {
		t.Fatalf("expected SimpleShaper to shape ASCII rune")
	}
	if len(glyphs) != 1 || glyphs[0].GlyphID == 0 || glyphs[0].XAdvance <= 0 {
		t.Fatalf("unexpected shaped glyphs: %#v", glyphs)
	}
}

func TestSimpleShaperShapesNFCClusterToSingleGlyph(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	glyphs, ok := (SimpleShaper{}).Shape("e\u0301", backend.faces[0], backend.ppem)
	if !ok {
		t.Fatalf("expected SimpleShaper to shape NFC-composable cluster")
	}
	if len(glyphs) != 1 || glyphs[0].GlyphID == 0 {
		t.Fatalf("expected one composed glyph, got %#v", glyphs)
	}
}

func TestSimpleShaperRejectsComplexClusters(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if glyphs, ok := (SimpleShaper{}).Shape("👨\u200d👩\u200d👧", backend.faces[0], backend.ppem); ok {
		t.Fatalf("expected ZWJ emoji cluster to fall back, got %#v", glyphs)
	}
	if glyphs, ok := (SimpleShaper{}).Shape("م", backend.faces[0], backend.ppem); ok {
		t.Fatalf("expected complex-script rune to fall back, got %#v", glyphs)
	}
}

func TestOpenTypeBackendHasDefaultShaper(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if backend.shaper == nil {
		t.Fatalf("default shaper is nil")
	}
	if glyphs, ok := backend.shaper.Shape("A", backend.faces[0], backend.ppem); !ok || len(glyphs) == 0 {
		t.Fatalf("default shaper failed to shape simple glyph: %#v ok=%v", glyphs, ok)
	}
}

func TestSimpleShaperReportsDeterministicFeatureCapability(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	backend.SetShaper(SimpleShaper{})
	features, err := fontdesc.NewFeatureSet(false, map[string]int{"ss01": 1})
	if err != nil {
		t.Fatal(err)
	}
	ConfigureBackendFeatures(backend, features)
	if got := BackendFeatureCapability(backend); got != "portable-unsupported" {
		t.Fatalf("capability=%q", got)
	}
	plain, plainOK := (SimpleShaper{}).Shape("A", backend.faces[0], backend.ppem)
	featured, featureOK := (SimpleShaper{}).ShapeFeatures("A", backend.faces[0], backend.ppem, features)
	if !plainOK || !featureOK || len(plain) != 1 || len(featured) != 1 || plain[0].GlyphID != featured[0].GlyphID {
		t.Fatalf("portable feature fallback changed glyphs: %#v/%#v", plain, featured)
	}
}
