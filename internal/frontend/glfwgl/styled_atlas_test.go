//go:build glfw

package glfwgl

import (
	"image"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
)

type styledAtlasBackend struct {
	keys         [4]fontdesc.ResolvedFaceKey
	synthetic    [4]fontdesc.SyntheticMode
	runeCalls    [4]int
	clusterCalls [4]int
	runCalls     [4]int
	oversized    bool
}

func newStyledAtlasBackend() *styledAtlasBackend {
	b := &styledAtlasBackend{}
	for i := range b.keys {
		b.keys[i] = fontdesc.ResolvedFaceKey(fontdesc.CanonicalFaceIDFromBytes([]byte{byte(i + 1)}))
	}
	return b
}

func (b *styledAtlasBackend) CellMetrics() (int, int, int) { return 8, 16, 12 }
func (b *styledAtlasBackend) Close()                       {}
func (b *styledAtlasBackend) Rasterize(r rune, span int) (fontglyph.RasterizedGlyph, bool) {
	return b.RasterizeStyle(fontdesc.RequestedFaceStyleNormal, r, span)
}
func (b *styledAtlasBackend) RasterizeCluster(text string, span int) (fontglyph.RasterizedGlyph, bool) {
	return b.RasterizeClusterStyle(fontdesc.RequestedFaceStyleNormal, text, span)
}
func (b *styledAtlasBackend) StyleResolution(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	if request > fontdesc.RequestedFaceStyleBoldItalic {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	return b.keys[request], b.synthetic[request], true
}
func (b *styledAtlasBackend) RasterizeStyle(request fontdesc.RequestedFaceStyle, _ rune, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.runeCalls[request]++
	size := 1
	if b.oversized {
		size = atlasPageSize + 1
	}
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, size, 1)), Width: size, Height: 1, CellSpan: 1}, true
}
func (b *styledAtlasBackend) RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, _ string, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.clusterCalls[request]++
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Width: 1, Height: 1, CellSpan: 1}, true
}
func (b *styledAtlasBackend) RasterizeRunStyle(request fontdesc.RequestedFaceStyle, _ string, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.runCalls[request]++
	return fontglyph.RasterizedGlyph{}, false
}

func TestStyledAtlasSeparatesPositiveAndNegativeKeys(t *testing.T) {
	backend := newStyledAtlasBackend()
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}, 1, 0, func(fontglyph.Spec) (fontglyph.Backend, error) { return backend, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	clear(atlas.entries)
	backend.runeCalls = [4]int{}

	for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		if _, ok := atlas.cachedRuneStyle(request, 'A'); !ok {
			t.Fatalf("style %d rune miss", request)
		}
		if _, ok := atlas.cachedClusterStyle(request, "a\u0301", 1); !ok {
			t.Fatalf("style %d cluster miss", request)
		}
		if _, ok := atlas.cachedRunStyle(request, "->", 2); ok {
			t.Fatalf("style %d unexpected ligature", request)
		}
	}
	if len(atlas.entries) != 8 || len(atlas.runNegative) != 4 {
		t.Fatalf("entries/run negatives = %d/%d, want 8/4", len(atlas.entries), len(atlas.runNegative))
	}
	for i := range backend.runeCalls {
		if backend.runeCalls[i] != 1 || backend.clusterCalls[i] != 1 || backend.runCalls[i] != 1 {
			t.Fatalf("style %d calls rune/cluster/run=%d/%d/%d", i, backend.runeCalls[i], backend.clusterCalls[i], backend.runCalls[i])
		}
	}
}

func TestStyledAtlasSeparatesInsertionNegatives(t *testing.T) {
	backend := newStyledAtlasBackend()
	backend.oversized = true
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}, 1, 0, func(fontglyph.Spec) (fontglyph.Backend, error) { return backend, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	clear(atlas.insertNegative)
	for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		_, _ = atlas.cachedRuneStyle(request, 'Z')
	}
	if len(atlas.insertNegative) != 4 {
		t.Fatalf("insertion negatives=%d, want 4", len(atlas.insertNegative))
	}
}

func TestStyleDrawEffectsAndLigatureBoundaries(t *testing.T) {
	bold, skew := styleDrawEffects(fontdesc.SyntheticBold|fontdesc.SyntheticItalic, 20)
	if !bold || skew != 4 {
		t.Fatalf("synthetic effects bold/skew=%v/%.1f", bold, skew)
	}
	bold, skew = styleDrawEffects(fontdesc.SyntheticNone, 20)
	if bold || skew != 0 {
		t.Fatalf("real effects bold/skew=%v/%.1f", bold, skew)
	}
	cells := []core.Cell{{Attr: core.Attr{Bold: true}}, {Attr: core.Attr{Bold: true}}, {Attr: core.Attr{Italic: true}}}
	if !renderSpanMatchesStyle(cells, 0, 2, fontdesc.RequestedFaceStyleBold) {
		t.Fatal("matching style span rejected")
	}
	if renderSpanMatchesStyle(cells, 0, 3, fontdesc.RequestedFaceStyleBold) {
		t.Fatal("cross-style span accepted")
	}
}
