package fontglyph

import "testing"

// TestBackendLoadsFallbacksLazily verifies the ~24 MB fallback fonts are not
// loaded until a glyph the primary font can't cover is requested — the property
// that keeps a plain (no-emoji) session's heap small.
func TestBackendLoadsFallbacksLazily(t *testing.T) {
	b, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if b.fallbacksLoaded {
		t.Fatalf("fallbacks must not load eagerly")
	}
	if len(b.faces) != 1 {
		t.Fatalf("expected only the primary face before any fallback, got %d", len(b.faces))
	}

	// ASCII is covered by the primary; it must not trigger a fallback load.
	if _, _, _, ok := b.faceForRune('A'); !ok {
		t.Fatalf("primary font should cover 'A'")
	}
	if b.fallbacksLoaded {
		t.Fatalf("resolving an ASCII glyph must not load fallbacks")
	}

	// A glyph outside the primary (an emoji) triggers the lazy load.
	b.faceForRune('😀')
	if !b.fallbacksLoaded {
		t.Fatalf("resolving an uncovered glyph should have loaded the fallbacks")
	}
}
