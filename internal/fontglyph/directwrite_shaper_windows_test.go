//go:build windows

package fontglyph

import "testing"

func TestDefaultShaperUsesDirectWriteWhenAvailable(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if !directWriteTextAnalyzerAvailable() {
		if _, ok := backend.shaper.(SimpleShaper); !ok {
			t.Fatalf("default shaper = %T, want SimpleShaper when DirectWrite TextAnalyzer is unavailable", backend.shaper)
		}
		return
	}
	shaper, ok := backend.shaper.(DirectWriteShaper)
	if !ok {
		t.Fatalf("default shaper = %T, want DirectWriteShaper when DirectWrite TextAnalyzer is available", backend.shaper)
	}
	if !shaper.Available() {
		t.Fatalf("DirectWriteShaper reports unavailable despite TextAnalyzer availability")
	}
	if glyphs, ok := shaper.Shape("A", backend.faces[0], backend.ppem); !ok || len(glyphs) == 0 {
		t.Fatalf("DirectWriteShaper fallback failed to shape simple glyph: %#v ok=%v", glyphs, ok)
	}
}

func TestDirectWriteShaperDefersComplexClustersUntilTextAnalyzer(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	shaper := DirectWriteShaper{Fallback: SimpleShaper{}}
	if glyphs, ok := shaper.Shape("م", backend.faces[0], backend.ppem); ok {
		t.Fatalf("complex script should defer until TextAnalyzer integration, got %#v", glyphs)
	}
}

func TestDirectWriteShaperShapesComplexFallbackFace(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	face, ok := backend.faceForCluster("م")
	if !ok || face.sourcePath == "" {
		t.Skip("no source-backed fallback font for Arabic sample")
	}
	shaper := DirectWriteShaper{Fallback: SimpleShaper{}}
	glyphs, ok := shaper.Shape("م", face, backend.ppem)
	if !ok || len(glyphs) == 0 {
		t.Fatalf("expected DirectWrite to shape Arabic sample using %s, got %#v ok=%v", face.sourcePath, glyphs, ok)
	}
	for _, glyph := range glyphs {
		if glyph.GlyphID == 0 || glyph.XAdvance < 0 {
			t.Fatalf("invalid DirectWrite glyph: %#v", glyph)
		}
	}
}

func TestDirectWriteShaperShapesEmojiZWJFallbackFace(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cluster := "👩\u200d💻"
	face, ok := backend.faceForCluster(cluster)
	if !ok || face.sourcePath == "" {
		t.Skip("no source-backed fallback font for emoji ZWJ sample")
	}
	shaper := DirectWriteShaper{Fallback: SimpleShaper{}}
	glyphs, ok := shaper.Shape(cluster, face, backend.ppem)
	if !ok || len(glyphs) == 0 {
		t.Fatalf("expected DirectWrite to shape emoji ZWJ sample using %s, got %#v ok=%v", face.sourcePath, glyphs, ok)
	}
	for _, glyph := range glyphs {
		if glyph.GlyphID == 0 || glyph.XAdvance < 0 {
			t.Fatalf("invalid DirectWrite glyph: %#v", glyph)
		}
	}
}

func TestDirectWriteShaperShapesIndicFallbackFace(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	cluster := "क"
	face, ok := backend.faceForCluster(cluster)
	if !ok || face.sourcePath == "" {
		t.Skip("no source-backed fallback font for Indic sample")
	}
	shaper := DirectWriteShaper{Fallback: SimpleShaper{}}
	glyphs, ok := shaper.Shape(cluster, face, backend.ppem)
	if !ok || len(glyphs) == 0 {
		t.Fatalf("expected DirectWrite to shape Indic sample using %s, got %#v ok=%v", face.sourcePath, glyphs, ok)
	}
	for _, glyph := range glyphs {
		if glyph.GlyphID == 0 || glyph.XAdvance < 0 {
			t.Fatalf("invalid DirectWrite glyph: %#v", glyph)
		}
	}
}

func TestDirectWriteShaperShapesScriptClusterSmokeCases(t *testing.T) {
	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 18, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	shaper := DirectWriteShaper{Fallback: SimpleShaper{}}
	for _, sample := range []string{"لا", "क्ष"} {
		t.Run(sample, func(t *testing.T) {
			face, ok := backend.faceForCluster(sample)
			if !ok || face.sourcePath == "" {
				t.Skipf("no source-backed fallback font for %q", sample)
			}
			glyphs, ok := shaper.Shape(sample, face, backend.ppem)
			if !ok || len(glyphs) == 0 {
				t.Fatalf("expected DirectWrite to shape %q using %s, got %#v ok=%v", sample, face.sourcePath, glyphs, ok)
			}
			for _, glyph := range glyphs {
				if glyph.GlyphID == 0 || glyph.XAdvance < 0 {
					t.Fatalf("invalid DirectWrite glyph for %q: %#v", sample, glyph)
				}
			}
		})
	}
}
