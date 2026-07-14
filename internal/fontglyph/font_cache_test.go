package fontglyph

import (
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

// TestFontParseCacheReusesParse verifies the expensive parse happens once per
// font identity and is reused across point sizes — the property that keeps
// zooming cheap (no font re-read/re-parse per step).
func TestFontParseCacheReusesParse(t *testing.T) {
	calls := 0
	load := func() ([]byte, error) {
		calls++
		return gomono.TTF, nil
	}
	const key = "test:cache-reuse-fixture"

	f1, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 12, DPI: 96}, load)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	f2, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 24, DPI: 96}, load)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if calls != 1 {
		t.Fatalf("font should be read/parsed once and reused across sizes, got %d loads", calls)
	}
	if f1.sfnt == nil || f1.sfnt != f2.sfnt {
		t.Fatalf("expected both faces to share the cached parsed sfnt")
	}
	if f1.face == f2.face {
		t.Fatalf("expected a distinct face per point size")
	}
}
