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
	rasterMiss   bool
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
	if b.rasterMiss {
		return fontglyph.RasterizedGlyph{}, false
	}
	size := 1
	if b.oversized {
		size = atlasPageSize + 1
	}
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, size, 1)), Width: size, Height: 1, CellSpan: 1}, true
}
func (b *styledAtlasBackend) RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, _ string, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.clusterCalls[request]++
	if b.rasterMiss {
		return fontglyph.RasterizedGlyph{}, false
	}
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Width: 1, Height: 1, CellSpan: 1}, true
}
func (b *styledAtlasBackend) RasterizeRunStyle(request fontdesc.RequestedFaceStyle, _ string, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.runCalls[request]++
	return fontglyph.RasterizedGlyph{}, false
}

type metricPathBackend struct{ *styledAtlasBackend }

func (b *metricPathBackend) RasterizeStyle(request fontdesc.RequestedFaceStyle, _ rune, _ int) (fontglyph.RasterizedGlyph, bool) {
	b.runeCalls[request]++
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, 8, 16)), Width: 8, Height: 16, CellSpan: 1, HasColor: true}, true
}

func (b *metricPathBackend) RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, _ string, span int) (fontglyph.RasterizedGlyph, bool) {
	b.clusterCalls[request]++
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, 8*span, 16)), Width: 8 * span, Height: 16, CellSpan: span, Subpixel: true}, true
}

func (b *metricPathBackend) RasterizeRunStyle(request fontdesc.RequestedFaceStyle, _ string, span int) (fontglyph.RasterizedGlyph, bool) {
	b.runCalls[request]++
	return fontglyph.RasterizedGlyph{Image: image.NewRGBA(image.Rect(0, 0, 8*span, 16)), Width: 8 * span, Height: 16, CellSpan: span}, true
}

func TestMetricProjectionCoversRuneClusterRunAndColorPaths(t *testing.T) {
	backend := &metricPathBackend{styledAtlasBackend: newStyledAtlasBackend()}
	projection, err := fontdesc.NewMetricProjection(1.5, 1.25, 2, 3, -4)
	if err != nil {
		t.Fatal(err)
	}
	spec := fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}
	ctx, err := makeAtlasFontContextFromBackendWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: spec.Family}}, metrics: projection}, backend, fontInstallationMetrics{cellW: 8, cellH: 16, baseline: 12})
	if err != nil {
		t.Fatal(err)
	}
	atlas := newGlyphAtlasWithPreparedContext(&atlasTestRenderer{}, ctx, func(fontglyph.Spec) (fontglyph.Backend, error) { return backend, nil })
	defer atlas.close()
	clear(atlas.entries)
	runeEntry, runeOK := atlas.cachedRuneStyle(fontdesc.RequestedFaceStyleNormal, 'A')
	clusterEntry, clusterOK := atlas.cachedClusterStyle(fontdesc.RequestedFaceStyleNormal, "ab", 2)
	runEntry, runOK := atlas.cachedRunStyle(fontdesc.RequestedFaceStyleNormal, "->", 2)
	if !runeOK || !clusterOK || !runOK {
		t.Fatalf("projected path results rune/cluster/run=%v/%v/%v", runeOK, clusterOK, runOK)
	}
	if runeEntry.cellW != 10 || runeEntry.cellH != 24 || !runeEntry.colored || clusterEntry.cellSpan != 2 || !clusterEntry.subpixel || runEntry.cellSpan != 2 {
		t.Fatalf("projected entries rune=%#v cluster=%#v run=%#v", runeEntry, clusterEntry, runEntry)
	}
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
	if len(atlas.entries) != 8 || atlasNegativeReasonCount(atlas.activeContext, negativeRun) != 4 {
		t.Fatalf("entries/run negatives = %d/%d, want 8/4", len(atlas.entries), atlasNegativeReasonCount(atlas.activeContext, negativeRun))
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
	atlas.activeContext.negatives = atlasNegativeCache{}
	for request := fontdesc.RequestedFaceStyleNormal; request <= fontdesc.RequestedFaceStyleBoldItalic; request++ {
		_, _ = atlas.cachedRuneStyle(request, 'Z')
	}
	if count := atlasNegativeReasonCount(atlas.activeContext, negativeInsertion); count != 4 {
		t.Fatalf("insertion negatives=%d, want 4", count)
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

type contentStyledAtlasBackend struct {
	*styledAtlasBackend
	runeKey    fontdesc.ResolvedFaceKey
	clusterKey fontdesc.ResolvedFaceKey
}

func (b *contentStyledAtlasBackend) RuneResolution(request fontdesc.RequestedFaceStyle, _ rune) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	return b.runeKey, fontdesc.SyntheticNone, request <= fontdesc.RequestedFaceStyleBoldItalic
}

func (b *contentStyledAtlasBackend) ClusterResolution(request fontdesc.RequestedFaceStyle, _ string) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	return b.clusterKey, fontdesc.SyntheticItalic, request <= fontdesc.RequestedFaceStyleBoldItalic
}

func TestAtlasUsesContentResolvedFaceKeys(t *testing.T) {
	backend := &contentStyledAtlasBackend{
		styledAtlasBackend: newStyledAtlasBackend(),
		runeKey:            fontdesc.ResolvedFaceKey(fontdesc.CanonicalFaceIDFromBytes([]byte("fallback-rune"))),
		clusterKey:         fontdesc.ResolvedFaceKey(fontdesc.CanonicalFaceIDFromBytes([]byte("rule-cluster"))),
	}
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}, 1, 0, func(fontglyph.Spec) (fontglyph.Backend, error) { return backend, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	clear(atlas.entries)
	if _, ok := atlas.cachedRuneStyle(fontdesc.RequestedFaceStyleNormal, 'A'); !ok {
		t.Fatal("content-resolved rune failed")
	}
	if _, ok := atlas.cachedClusterStyle(fontdesc.RequestedFaceStyleNormal, "A", 1); !ok {
		t.Fatal("content-resolved cluster failed")
	}
	runeKey := atlasKey{spec: atlas.activeContext.key, face: backend.runeKey, kind: 'r', r: 'A'}
	clusterKey := atlasKey{spec: atlas.activeContext.key, face: backend.clusterKey, kind: 'c', text: "A", span: 1}
	if _, ok := atlas.entries[runeKey]; !ok {
		t.Fatalf("rune cache missing content face key: %#v", atlas.entries)
	}
	if _, ok := atlas.entries[clusterKey]; !ok {
		t.Fatalf("cluster cache missing content face key: %#v", atlas.entries)
	}
	if _, synthetic := atlas.resolveClusterStyle(fontdesc.RequestedFaceStyleNormal, "A"); synthetic != fontdesc.SyntheticItalic {
		t.Fatalf("cluster synthetic mode = %d, want italic", synthetic)
	}
}

func TestAtlasBoundsAndCachesRasterNegatives(t *testing.T) {
	backend := newStyledAtlasBackend()
	backend.rasterMiss = true
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}, 1, 0, func(fontglyph.Spec) (fontglyph.Backend, error) { return backend, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	backend.runeCalls = [4]int{}
	atlas.activeContext.negatives = atlasNegativeCache{}
	for attempt := 0; attempt < 2; attempt++ {
		if _, ok := atlas.cachedRuneStyle(fontdesc.RequestedFaceStyleNormal, '漢'); ok {
			t.Fatal("raster miss produced rune entry")
		}
		if _, ok := atlas.cachedClusterStyle(fontdesc.RequestedFaceStyleNormal, "漢", 2); ok {
			t.Fatal("raster miss produced cluster entry")
		}
	}
	if backend.runeCalls[0] != 1 || backend.clusterCalls[0] != 1 || atlasNegativeReasonCount(atlas.activeContext, negativeRaster) != 2 {
		t.Fatalf("negative cache calls/entries = %d/%d/%d", backend.runeCalls[0], backend.clusterCalls[0], atlasNegativeReasonCount(atlas.activeContext, negativeRaster))
	}
	for index := 0; index <= fontdesc.MaxNegativeEntries; index++ {
		atlas.activeContext.negatives.record(atlas.generation, negativeRun, atlasKey{kind: 'l', text: string(rune(index + 1))})
		atlas.recordInsertionFailure(atlas.activeContext, atlasKey{kind: 'r', r: rune(index + 1)})
	}
	if len(atlas.activeContext.negatives.entries) != fontdesc.MaxNegativeEntries {
		t.Fatalf("bounded per-context negatives = %d, want %d", len(atlas.activeContext.negatives.entries), fontdesc.MaxNegativeEntries)
	}
}
