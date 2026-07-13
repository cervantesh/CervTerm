package fontglyph

import "testing"

// ligatureRunShaper collapses any multi-rune cluster into a single real glyph
// (the first rune's glyph) so the whole-run output has fewer glyphs than the
// input — the unambiguous ligature case. Single runes shape to themselves.
type ligatureRunShaper struct{}

func (ligatureRunShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if face.sfnt == nil {
		return nil, false
	}
	runes := []rune(cluster)
	if len(runes) == 0 {
		return nil, false
	}
	g, ok := shapeOneRune(face.sfnt, runes[0], ppem)
	if !ok {
		return nil, false
	}
	return g, true
}

// kerningRunShaper mimics a GPOS-only font: it returns one glyph per rune with
// the real glyph IDs (identical to per-rune shaping) but nudges the advance for
// multi-rune clusters. Advance-only changes must not count as a ligature.
type kerningRunShaper struct{}

func (kerningRunShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if face.sfnt == nil {
		return nil, false
	}
	var out []ShapedGlyph
	for _, r := range cluster {
		g, ok := shapeOneRune(face.sfnt, r, ppem)
		if !ok {
			return nil, false
		}
		out = append(out, g...)
	}
	if len(out) == 0 {
		return nil, false
	}
	if len([]rune(cluster)) > 1 {
		out[0].XAdvance -= 1 // kern the pair tighter; glyph IDs unchanged
	}
	return out, true
}

func TestRasterizeRunDetectsLigatureSubstitution(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.SetShaper(ligatureRunShaper{})
	glyph, ligated := backend.RasterizeRun("->", 2)
	if !ligated {
		t.Fatalf("expected substitution to be reported as a ligature")
	}
	if glyph.Image == nil || !hasOpaquePixel(glyph.Image) {
		t.Fatalf("ligature glyph must be a visible bitmap, got %#v", glyph)
	}
	if glyph.CellSpan != 2 {
		t.Fatalf("ligature CellSpan = %d, want 2 (grid wins)", glyph.CellSpan)
	}
	if want := float64(backend.cellW * 2); glyph.AdvanceX != want {
		t.Fatalf("ligature AdvanceX = %v, want %v (grid wins)", glyph.AdvanceX, want)
	}
}

func TestRasterizeRunRejectsNoSubstitution(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	// SimpleShaper maps one glyph per rune with unchanged IDs -> no ligature.
	backend.SetShaper(SimpleShaper{})
	if _, ligated := backend.RasterizeRun("->", 2); ligated {
		t.Fatalf("per-rune shaping must not be reported as a ligature")
	}
}

func TestRasterizeRunTreatsKerningAsNoLigature(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.SetShaper(kerningRunShaper{})
	if _, ligated := backend.RasterizeRun("->", 2); ligated {
		t.Fatalf("advance-only (GPOS) changes must not count as a ligature")
	}
}

func TestSupportsLigaturesGatesOnShaperKind(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	backend.SetShaper(SimpleShaper{})
	if backend.SupportsLigatures() {
		t.Fatalf("SimpleShaper must not support ligatures")
	}
	backend.SetShaper(ligatureRunShaper{})
	if !backend.SupportsLigatures() {
		t.Fatalf("an advanced shaper must support ligatures")
	}
}
