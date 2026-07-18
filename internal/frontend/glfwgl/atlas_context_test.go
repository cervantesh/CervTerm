//go:build glfw

package glfwgl

import (
	"image"
	"image/color"
	"testing"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
)

func atlasNegativeReasonCount(ctx *atlasFontContext, reason byte) int {
	count := 0
	for entry := range ctx.negatives.entries {
		if entry.reason == reason {
			count++
		}
	}
	return count
}

type atlasTestRenderer struct {
	configureCalls  int
	configuredPages int
	configuredSize  int
	uploadCalls     int
	clearCalls      int
	destroyCalls    int
	draws           []atlasTestDraw
}

type atlasTestDraw struct {
	width  float32
	height float32
}

func (*atlasTestRenderer) Resize(int, int)                                            {}
func (*atlasTestRenderer) BeginFrame(int, int)                                        {}
func (*atlasTestRenderer) PushClip(gpu.ClipRect)                                      {}
func (*atlasTestRenderer) PopClip()                                                   {}
func (*atlasTestRenderer) Clear(color.RGBA)                                           {}
func (*atlasTestRenderer) FillRect(float32, float32, float32, float32, color.RGBA)    {}
func (*atlasTestRenderer) ReplaceRect(float32, float32, float32, float32, color.RGBA) {}
func (r *atlasTestRenderer) DrawGlyph(_ int, _ gpu.GlyphMode, _, _, width, height, _ float32, _, _, _, _ float32, _ color.RGBA) {
	r.draws = append(r.draws, atlasTestDraw{width: width, height: height})
}
func (r *atlasTestRenderer) ConfigureAtlas(pageCount, sizePx int) {
	r.configureCalls++
	r.configuredPages = pageCount
	r.configuredSize = sizePx
}
func (r *atlasTestRenderer) UploadAtlasRegion(_, _, _, _, _ int, _ []byte) { r.uploadCalls++ }
func (r *atlasTestRenderer) ClearAtlasPage(int)                            { r.clearCalls++ }
func (*atlasTestRenderer) EndFrame()                                       {}
func (r *atlasTestRenderer) Destroy()                                      { r.destroyCalls++ }

type atlasTestBackend struct {
	cellW, cellH int
	baseline     int
	closeCalls   int
	runCalls     map[string]int
}

func (b *atlasTestBackend) CellMetrics() (int, int, int) {
	return b.cellW, b.cellH, b.baseline
}

func (b *atlasTestBackend) Rasterize(_ rune, cellSpan int) (fontglyph.RasterizedGlyph, bool) {
	return b.raster(cellSpan), true
}

func (b *atlasTestBackend) RasterizeCluster(_ string, cellSpan int) (fontglyph.RasterizedGlyph, bool) {
	return b.raster(cellSpan), true
}

func (b *atlasTestBackend) RasterizeRun(run string, _ int) (fontglyph.RasterizedGlyph, bool) {
	if b.runCalls == nil {
		b.runCalls = make(map[string]int)
	}
	b.runCalls[run]++
	return fontglyph.RasterizedGlyph{}, false
}

func (*atlasTestBackend) SupportsLigatures() bool { return true }
func (b *atlasTestBackend) Close()                { b.closeCalls++ }

func (b *atlasTestBackend) raster(cellSpan int) fontglyph.RasterizedGlyph {
	cellSpan = max(1, cellSpan)
	img := image.NewRGBA(image.Rect(0, 0, b.cellW*cellSpan, b.cellH))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
	return fontglyph.RasterizedGlyph{Image: img, CellSpan: cellSpan}
}

type atlasStyledTestBackend struct {
	*atlasTestBackend
	environment fontdesc.FontEnvironmentKey
	faceSalt    string
}

func (b *atlasStyledTestBackend) StyleResolution(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	if request > fontdesc.RequestedFaceStyleBoldItalic {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	key, err := fontdesc.NewResolvedFaceKey(fontdesc.ResolvedFaceInput{
		Environment: b.environment,
		Face:        fontdesc.CanonicalFaceIDFromBytes([]byte(b.faceSalt)),
		Tier:        fontdesc.SourceTierPrimary,
		SourceIndex: uint32(request),
		Target:      fontdesc.FaceTarget{Weight: fontdesc.DefaultWeight, Style: fontdesc.StyleNormal, Stretch: fontdesc.DefaultStretch},
	})
	return key, fontdesc.SyntheticNone, err == nil
}

func (b *atlasStyledTestBackend) RasterizeStyle(_ fontdesc.RequestedFaceStyle, _ rune, cellSpan int) (fontglyph.RasterizedGlyph, bool) {
	return b.raster(cellSpan), true
}

func (b *atlasStyledTestBackend) RasterizeClusterStyle(_ fontdesc.RequestedFaceStyle, _ string, cellSpan int) (fontglyph.RasterizedGlyph, bool) {
	return b.raster(cellSpan), true
}

func (b *atlasStyledTestBackend) RasterizeRunStyle(_ fontdesc.RequestedFaceStyle, run string, cellSpan int) (fontglyph.RasterizedGlyph, bool) {
	return b.RasterizeRun(run, cellSpan)
}

func TestAtlasDescriptorKeysPreserveOrderAndLegacyIdentity(t *testing.T) {
	spec := fontglyph.Spec{Family: "Legacy Mono", Size: 14, DPI: 96, TextRaster: "gray"}
	legacy, err := makeAtlasFontKey(spec, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	legacyDescriptor, err := makeAtlasFontKeyWithDescriptors(spec, 1, 0, []fontdesc.Descriptor{{Family: spec.Family}})
	if err != nil {
		t.Fatal(err)
	}
	if legacy != legacyDescriptor {
		t.Fatal("legacy atlas key no longer delegates to its one-family descriptor identity")
	}

	descriptors := []fontdesc.Descriptor{{Family: "First Mono"}, {Family: "Second Mono", Weight: 700}}
	ordered, err := makeAtlasFontKeyWithDescriptors(spec, 1, 0, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	shadowedSpec := spec
	shadowedSpec.Family = "Different Legacy Shorthand"
	shadowed, err := makeAtlasFontKeyWithDescriptors(shadowedSpec, 1, 0, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if shadowed != ordered {
		t.Fatal("shadowed font.family changed descriptor environment key")
	}
	reordered, err := makeAtlasFontKeyWithDescriptors(spec, 1, 0, []fontdesc.Descriptor{descriptors[1], descriptors[0]})
	if err != nil {
		t.Fatal(err)
	}
	mutated := append([]fontdesc.Descriptor(nil), descriptors...)
	mutated[1].Weight = 600
	changed, err := makeAtlasFontKeyWithDescriptors(spec, 1, 0, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if ordered.environment == reordered.environment || ordered.environment == changed.environment {
		t.Fatal("ordered descriptor environment did not include order and descriptor payload")
	}
}

func TestAtlasFontModelIdentityIncludesFallbackRules(t *testing.T) {
	spec := fontglyph.Spec{Family: "ignored", Size: 14, DPI: 96, TextRaster: "gray"}
	features, err := fontdesc.NewFeatureSet(false, map[string]int{"ss01": 1})
	if err != nil {
		t.Fatal(err)
	}
	model := atlasFontModel{
		descriptors: []fontdesc.Descriptor{{Family: "Primary"}},
		fallback:    []fontdesc.Descriptor{{Family: "Fallback One"}, {Family: "Fallback Two"}},
		rules:       []fontdesc.Rule{{Match: fontdesc.RuleMatch{Class: fontdesc.SymbolClassEmoji}, Use: fontdesc.Descriptor{Family: "Emoji"}}},
		features:    features,
	}
	base, err := makeAtlasFontKeyWithModel(spec, 1, 0, model)
	if err != nil {
		t.Fatal(err)
	}
	reordered := model
	reordered.fallback = []fontdesc.Descriptor{model.fallback[1], model.fallback[0]}
	reorderedKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, reordered)
	if err != nil {
		t.Fatal(err)
	}
	mutated := model
	mutated.rules = cloneAtlasRules(model.rules)
	mutated.rules[0].Match.Class = fontdesc.SymbolClassSymbols
	mutatedKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, mutated)
	if err != nil {
		t.Fatal(err)
	}
	changedFeatures, err := fontdesc.NewFeatureSet(false, map[string]int{"ss01": 2})
	if err != nil {
		t.Fatal(err)
	}
	featureMutated := model
	featureMutated.features = changedFeatures
	featureKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, featureMutated)
	if err != nil {
		t.Fatal(err)
	}
	if base.environment == reorderedKey.environment || base.environment == mutatedKey.environment || base.environment == featureKey.environment {
		t.Fatal("fallback/rule/feature mutation did not change atlas environment")
	}
}

func TestAtlasDescriptorContextUsesStyledResolvedFace(t *testing.T) {
	spec := fontglyph.Spec{Family: "ignored", Size: 14, DPI: 96, TextRaster: "gray"}
	descriptors := []fontdesc.Descriptor{{Family: "Styled Mono"}, {Family: "Fallback Mono"}}
	key, err := makeAtlasFontKeyWithDescriptors(spec, 1, 0, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	backend := &atlasStyledTestBackend{atlasTestBackend: &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}, environment: key.environment, faceSalt: "styled-normal"}
	ctx, err := makeAtlasFontContextFromBackendWithDescriptors(spec, 1, 0, descriptors, backend, fontInstallationMetrics{cellW: 8, cellH: 16, baseline: 12})
	if err != nil {
		t.Fatal(err)
	}
	want, _, _ := backend.StyleResolution(fontdesc.RequestedFaceStyleNormal)
	if ctx.resolvedFace != want || ctx.key.environment != key.environment {
		t.Fatal("descriptor context did not retain styled backend identities")
	}
}

func TestAtlasFontKeyAndFixedPoolConstants(t *testing.T) {
	if atlasPageCount != 2 || atlasPageSize != 2048 {
		t.Fatalf("atlas pool = %dx%d pages, want exactly 2x2048", atlasPageCount, atlasPageSize)
	}
	baseSpec := fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "gray"}
	base := newAtlasFontKey(baseSpec, 1, 0)
	keys := []atlasFontKey{
		newAtlasFontKey(fontglyph.Spec{Family: "Other", Size: 14, DPI: 96, TextRaster: "gray"}, 1, 0),
		newAtlasFontKey(fontglyph.Spec{Family: "Go Mono", Size: 15, DPI: 96, TextRaster: "gray"}, 1, 0),
		newAtlasFontKey(fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 120, TextRaster: "gray"}, 1, 0),
		newAtlasFontKey(fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "subpixel"}, 1, 0),
		newAtlasFontKey(baseSpec, 1.1, 0),
		newAtlasFontKey(baseSpec, 1, 0.1),
	}
	for i, key := range keys {
		if key == base {
			t.Fatalf("pixel-affecting key variant %d aliases base key", i)
		}
	}
}

func TestAtlasSpecSwitchNamespacesAndReusesEntries(t *testing.T) {
	renderer := &atlasTestRenderer{}
	var backends []*atlasTestBackend
	factory := func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: int(spec.Size) - 2, cellH: int(spec.Size) + 6, baseline: int(spec.Size)}
		backends = append(backends, backend)
		return backend, nil
	}
	spec1 := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96, TextRaster: "gray"}
	spec2 := fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "gray"}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, spec1, 1, 0, factory)
	if err != nil {
		t.Fatalf("newGlyphAtlasWithBackendFactory: %v", err)
	}
	if renderer.configureCalls != 1 || renderer.configuredPages != 2 || renderer.configuredSize != 2048 {
		t.Fatalf("ConfigureAtlas calls/config = %d/%dx%d, want 1/2x2048", renderer.configureCalls, renderer.configuredPages, renderer.configuredSize)
	}

	first, ok := atlas.cachedRune('λ')
	if !ok {
		t.Fatal("first spec failed to cache rune")
	}
	generation := atlas.generation
	if _, _, _, ok := atlas.useSpec(spec2, 1.1, 0.05); !ok {
		t.Fatal("second spec selection failed")
	}
	if renderer.clearCalls != 0 || atlas.generation != generation {
		t.Fatalf("context switch cleared/reset atlas: clears=%d generation=%d, want 0/%d", renderer.clearCalls, atlas.generation, generation)
	}
	second, ok := atlas.cachedRune('λ')
	if !ok {
		t.Fatal("second spec failed to cache rune")
	}
	ctx1 := atlas.contexts[newAtlasFontKey(spec1, 1, 0)]
	ctx2 := atlas.contexts[newAtlasFontKey(spec2, 1.1, 0.05)]
	key1 := atlasKey{spec: ctx1.key, face: ctx1.resolvedFace, kind: 'r', r: 'λ'}
	key2 := atlasKey{spec: ctx2.key, face: ctx2.resolvedFace, kind: 'r', r: 'λ'}
	if key1 == key2 {
		t.Fatal("distinct specs produced equal rune keys")
	}
	if _, ok := atlas.entries[key1]; !ok {
		t.Fatal("first spec rune entry missing")
	}
	if _, ok := atlas.entries[key2]; !ok {
		t.Fatal("second spec rune entry missing")
	}

	atlas.drawEntry(first, 0, 0, color.RGBA{}, 1, 0)
	draw := renderer.draws[len(renderer.draws)-1]
	if draw.width != float32(first.cellW*max(1, first.cellSpan)) || draw.height != float32(first.cellH) {
		t.Fatalf("first entry draw dimensions = %.0fx%.0f, want %dx%d", draw.width, draw.height, first.cellW*max(1, first.cellSpan), first.cellH)
	}
	if first.cellW == second.cellW || first.cellH == second.cellH {
		t.Fatal("test backends did not produce distinct metrics")
	}

	uploads := renderer.uploadCalls
	if _, _, _, ok := atlas.useSpec(spec1, 1, 0); !ok {
		t.Fatal("return to first spec failed")
	}
	reused, ok := atlas.cachedRune('λ')
	if !ok || reused != first {
		t.Fatal("return to first spec did not reuse its rune entry")
	}
	if renderer.uploadCalls != uploads {
		t.Fatalf("return to known spec uploaded %d regions, want 0", renderer.uploadCalls-uploads)
	}
	if renderer.clearCalls != 0 || atlas.generation != generation || renderer.configureCalls != 1 {
		t.Fatalf("known-spec reuse changed pool: clears=%d generation=%d configure=%d", renderer.clearCalls, atlas.generation, renderer.configureCalls)
	}

	atlas.close()
	atlas.close()
	if renderer.destroyCalls != 1 {
		t.Fatalf("renderer Destroy calls = %d, want 1", renderer.destroyCalls)
	}
	if len(backends) != 2 {
		t.Fatalf("backend creations = %d, want 2", len(backends))
	}
	for i, backend := range backends {
		if backend.closeCalls != 1 {
			t.Fatalf("backend %d Close calls = %d, want 1", i, backend.closeCalls)
		}
	}
}

func TestAtlasNegativeLigatureCacheIsSpecNamespaced(t *testing.T) {
	renderer := &atlasTestRenderer{}
	var backends []*atlasTestBackend
	factory := func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size) + 4, baseline: int(spec.Size)}
		backends = append(backends, backend)
		return backend, nil
	}
	spec1 := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96, TextRaster: "gray"}
	spec2 := fontglyph.Spec{Family: "Go Mono", Size: 12, DPI: 96, TextRaster: "gray"}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, spec1, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()

	if _, ok := atlas.cachedRun("->", 2); ok {
		t.Fatal("fake backend unexpectedly produced a ligature")
	}
	_, _ = atlas.cachedRun("->", 2)
	if backends[0].runCalls["->"] != 1 {
		t.Fatalf("first negative run rasterizations = %d, want 1", backends[0].runCalls["->"])
	}
	if _, _, _, ok := atlas.useSpec(spec2, 1, 0); !ok {
		t.Fatal("second spec selection failed")
	}
	_, _ = atlas.cachedRun("->", 2)
	if backends[1].runCalls["->"] != 1 {
		t.Fatalf("second negative run rasterizations = %d, want 1", backends[1].runCalls["->"])
	}
	if _, _, _, ok := atlas.useSpec(spec1, 1, 0); !ok {
		t.Fatal("return to first spec failed")
	}
	_, _ = atlas.cachedRun("->", 2)
	if backends[0].runCalls["->"] != 1 || atlasNegativeReasonCount(atlas.activeContext, negativeRun) != 1 {
		t.Fatalf("negative cache not context-isolated: first calls=%d entries=%d", backends[0].runCalls["->"], atlasNegativeReasonCount(atlas.activeContext, negativeRun))
	}
}

func TestAtlasCloseDeduplicatesSharedBackend(t *testing.T) {
	renderer := &atlasTestRenderer{}
	shared := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
	factory := func(fontglyph.Spec) (fontglyph.Backend, error) { return shared, nil }
	spec1 := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96}
	spec2 := fontglyph.Spec{Family: "Go Mono", Size: 12, DPI: 96}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, spec1, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := atlas.useSpec(spec2, 1, 0); !ok {
		t.Fatal("second spec selection failed")
	}
	atlas.close()
	atlas.close()
	if shared.closeCalls != 1 || renderer.destroyCalls != 1 {
		t.Fatalf("close lifetime calls backend/renderer = %d/%d, want 1/1", shared.closeCalls, renderer.destroyCalls)
	}
}

func TestAtlasPrewarmCapacityMissDoesNotReset(t *testing.T) {
	renderer := &atlasTestRenderer{}
	backend := &atlasTestBackend{cellW: 1, cellH: 1, baseline: 1}
	ctx := &atlasFontContext{key: newAtlasFontKey(fontglyph.Spec{Family: "tiny", Size: 1, DPI: 96}, 1, 0), backend: backend, cellW: 1, cellH: 1, baseline: 1}
	atlas := &glyphAtlas{
		r: renderer, entries: make(map[atlasKey]atlasEntry), contexts: map[atlasFontKey]*atlasFontContext{ctx.key: ctx},
		activeContext: ctx, backend: backend, generation: 1,
	}
	for i := range atlas.pages {
		atlas.pages[i].packer = newShelfPacker(1, 1)
	}
	atlas.prewarmASCII()
	if atlas.prewarming {
		t.Fatal("prewarm guard remained active")
	}
	if atlas.generation != 1 || renderer.clearCalls != 0 {
		t.Fatalf("prewarm evicted shared atlas: generation=%d clears=%d", atlas.generation, renderer.clearCalls)
	}
	if !ctx.prewarmed {
		t.Fatal("prewarm miss should be recorded so focus switches do not retry")
	}
	if _, ok := atlas.cachedRune('~'); ok {
		t.Fatal("capacity-constrained atlas unexpectedly inserted deferred glyph")
	}
	if atlas.generation != 2 || renderer.clearCalls != atlasPageCount {
		t.Fatalf("real request did not exercise bounded reset: generation=%d clears=%d", atlas.generation, renderer.clearCalls)
	}
}

func TestAtlasRetainsInactiveBackendContextsWithinBudget(t *testing.T) {
	renderer := &atlasTestRenderer{}
	backends := make(map[float64]*atlasTestBackend)
	factory := func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: int(spec.Size), cellH: int(spec.Size) + 2, baseline: int(spec.Size)}
		backends[spec.Size] = backend
		return backend, nil
	}
	spec1 := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96}
	spec2 := fontglyph.Spec{Family: "Go Mono", Size: 12, DPI: 96}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, spec1, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := atlas.useSpec(spec2, 1, 0); !ok {
		t.Fatal("second spec selection failed")
	}
	if _, _, _, ok := atlas.useSpec(spec1, 1, 0); !ok {
		t.Fatal("first spec reselection failed")
	}
	keep := map[atlasFontKey]struct{}{newAtlasFontKey(spec1, 1, 0): {}}
	if !atlas.retainContexts(keep) {
		t.Fatal("visible context pin was rejected")
	}
	if len(atlas.contexts) != 2 {
		t.Fatalf("retained backend contexts = %d, want 2", len(atlas.contexts))
	}
	if backends[12].closeCalls != 0 || backends[10].closeCalls != 0 {
		t.Fatalf("backend close calls active/inactive = %d/%d, want 0/0", backends[10].closeCalls, backends[12].closeCalls)
	}
	atlas.close()
}

func TestAtlasCapacityFailureResetsAtMostOncePerGeneration(t *testing.T) {
	renderer := &atlasTestRenderer{}
	backend := &atlasTestBackend{cellW: 1, cellH: 1, baseline: 1}
	ctx := &atlasFontContext{key: newAtlasFontKey(fontglyph.Spec{Family: "tiny", Size: 1, DPI: 96}, 1, 0), backend: backend, cellW: 1, cellH: 1, baseline: 1, prewarmed: true}
	atlas := &glyphAtlas{r: renderer, entries: make(map[atlasKey]atlasEntry), contexts: map[atlasFontKey]*atlasFontContext{ctx.key: ctx}, activeContext: ctx, backend: backend, generation: 1}
	for i := range atlas.pages {
		atlas.pages[i].packer = newShelfPacker(1, 1)
		if _, _, ok := atlas.pages[i].packer.Insert(1, 1); !ok {
			t.Fatal("failed to fill test atlas page")
		}
	}
	if _, ok := atlas.cachedRune('Ω'); ok {
		t.Fatal("capacity-constrained atlas unexpectedly inserted glyph")
	}
	generation, clears := atlas.generation, renderer.clearCalls
	if generation != 2 || clears != atlasPageCount {
		t.Fatalf("first capacity miss generation/clears = %d/%d, want 2/%d", generation, clears, atlasPageCount)
	}
	if _, ok := atlas.cachedRune('Ω'); ok {
		t.Fatal("negative capacity cache unexpectedly inserted glyph")
	}
	if atlas.generation != generation || renderer.clearCalls != clears {
		t.Fatalf("repeated capacity miss reset atlas again: generation/clears = %d/%d", atlas.generation, renderer.clearCalls)
	}
}

func TestFeatureIdentitySeparatesPositiveAndNegativeAtlasEntries(t *testing.T) {
	spec := fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "go"}
	enabled, err := fontdesc.NewFeatureSet(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := fontdesc.NewFeatureSet(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	enabledKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: "Go Mono"}}, features: enabled})
	if err != nil {
		t.Fatal(err)
	}
	disabledKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: "Go Mono"}}, features: disabled})
	if err != nil {
		t.Fatal(err)
	}
	if enabledKey == disabledKey {
		t.Fatal("enabled and disabled feature contexts aliased")
	}
	atlas := &glyphAtlas{generation: 1, entries: make(map[atlasKey]atlasEntry)}
	enabledRun := atlasKey{spec: enabledKey, kind: 'l', text: "->", span: 2}
	disabledRun := atlasKey{spec: disabledKey, kind: 'l', text: "->", span: 2}
	atlas.entries[enabledRun] = atlasEntry{generation: 1}
	enabledContext := &atlasFontContext{key: enabledKey}
	disabledContext := &atlasFontContext{key: disabledKey}
	enabledContext.negatives.record(atlas.generation, negativeRun, enabledRun)
	enabledContext.negatives.record(atlas.generation, negativeInsertion, enabledRun)
	if _, ok := atlas.currentEntry(disabledRun); ok {
		t.Fatal("positive feature entry aliased disabled context")
	}
	if disabledContext.negatives.contains(atlas.generation, negativeRun, disabledRun) || atlas.insertionFailedThisGeneration(disabledContext, disabledRun) {
		t.Fatal("negative feature entry aliased disabled context")
	}
}

func TestExplicitFeatureEnablesRunCollectionWithoutLigatureShorthand(t *testing.T) {
	features, err := fontdesc.NewFeatureSet(false, map[string]int{"dlig": 1})
	if err != nil {
		t.Fatal(err)
	}
	backend := &atlasTestBackend{}
	atlas := &glyphAtlas{activeContext: &atlasFontContext{backend: backend, features: features}}
	if !atlas.supportsLigatures(false) {
		t.Fatal("explicit substitution feature did not enable run collection")
	}
	disabled, err := fontdesc.NewFeatureSet(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	atlas.activeContext.features = disabled
	if atlas.supportsLigatures(false) {
		t.Fatal("disabled feature projection enabled run collection")
	}
	atlas.activeContext.features = fontdesc.FeatureSet{}
	if !atlas.supportsLigatures(true) {
		t.Fatal("legacy zero feature model stopped honoring ligature shorthand")
	}
}

func TestAtlasMetricProjectionChangesEnvironmentAndProjectedRaster(t *testing.T) {
	spec := fontglyph.Spec{Family: "Go Mono", Size: 14, DPI: 96}
	base := fontdesc.DefaultMetricProjection()
	baseKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: spec.Family}}, metrics: base})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := fontdesc.NewMetricProjection(1.5, 1.25, 2, 3, -4)
	if err != nil {
		t.Fatal(err)
	}
	projectedKey, err := makeAtlasFontKeyWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: spec.Family}}, metrics: projection})
	if err != nil {
		t.Fatal(err)
	}
	if projectedKey == baseKey {
		t.Fatal("metric projection did not change environment identity")
	}
	backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
	ctx, err := makeAtlasFontContextFromBackendWithModel(spec, 1, 0, atlasFontModel{descriptors: []fontdesc.Descriptor{{Family: spec.Family}}, metrics: projection}, backend, fontInstallationMetrics{cellW: 8, cellH: 16, baseline: 12})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.cellW != 10 || ctx.cellH != 24 || ctx.baseline != 18 {
		t.Fatalf("projected context metrics=%d/%d/%d, want 10/24/18", ctx.cellW, ctx.cellH, ctx.baseline)
	}
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			source.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	glyph := projectRasterToContext(fontglyph.RasterizedGlyph{Image: source, CellSpan: 1}, ctx)
	if glyph.Image.Bounds().Dx() != 10 || glyph.Image.Bounds().Dy() != 24 {
		t.Fatalf("projected raster bounds=%v", glyph.Image.Bounds())
	}
	if _, _, _, alpha := glyph.Image.At(7, 9).RGBA(); alpha == 0 {
		t.Fatal("glyph metric offsets were not projected into the fixed cell canvas")
	}
}

func TestAtlasRejectsSixtyFifthPinnedContextTransactionally(t *testing.T) {
	factoryCalls := 0
	factory := func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		factoryCalls++
		return &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}, nil
	}
	base := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96}
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, base, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	pins := cloneContextPins(atlas.pinnedContexts)
	for index := 1; index < maxAtlasFontContexts; index++ {
		spec := base
		spec.Size += float64(index)
		key, keyErr := atlas.fontKey(spec, 1, 0)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		pins[key] = struct{}{}
		if _, _, _, ok := atlas.usePinnedSpec(spec, 1, 0, pins); !ok {
			t.Fatalf("pinned context %d rejected", index+1)
		}
	}
	active, contextCount, callCount := atlas.activeContext, len(atlas.contexts), factoryCalls
	rejected := base
	rejected.Size += maxAtlasFontContexts
	rejectedKey, err := atlas.fontKey(rejected, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	pins[rejectedKey] = struct{}{}
	if _, _, _, ok := atlas.usePinnedSpec(rejected, 1, 0, pins); ok {
		t.Fatal("65th pinned context was admitted")
	}
	if atlas.activeContext != active || len(atlas.contexts) != contextCount || factoryCalls != callCount || len(atlas.pinnedContexts) != maxAtlasFontContexts {
		t.Fatalf("rejection mutated state: active=%v contexts=%d calls=%d pins=%d", atlas.activeContext == active, len(atlas.contexts), factoryCalls, len(atlas.pinnedContexts))
	}
}

func TestAtlasEvictsDeterministicInactiveLRUAtContextBudget(t *testing.T) {
	backends := make(map[float64]*atlasTestBackend)
	factory := func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
		backends[spec.Size] = backend
		return backend, nil
	}
	base := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96}
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, base, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	for index := 1; index < maxAtlasFontContexts; index++ {
		spec := base
		spec.Size += float64(index)
		if _, _, _, ok := atlas.useSpec(spec, 1, 0); !ok {
			t.Fatalf("context %d rejected", index+1)
		}
	}
	touched := base
	touched.Size++
	if _, _, _, ok := atlas.useSpec(touched, 1, 0); !ok {
		t.Fatal("LRU touch failed")
	}
	oldestUntouched := base
	oldestUntouched.Size += 2
	oldestKey, err := atlas.fontKey(oldestUntouched, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	admitted := base
	admitted.Size += maxAtlasFontContexts
	if _, _, _, ok := atlas.useSpec(admitted, 1, 0); !ok {
		t.Fatal("inactive LRU admission failed")
	}
	if len(atlas.contexts) != maxAtlasFontContexts || atlas.contexts[oldestKey] != nil || backends[oldestUntouched.Size].closeCalls != 1 {
		t.Fatalf("LRU eviction state: contexts=%d victimPresent=%v closeCalls=%d", len(atlas.contexts), atlas.contexts[oldestKey] != nil, backends[oldestUntouched.Size].closeCalls)
	}
}

func TestAtlasContextAdmissionAbortIsMutationFree(t *testing.T) {
	var backends []*atlasTestBackend
	factory := func(fontglyph.Spec) (fontglyph.Backend, error) {
		backend := &atlasTestBackend{cellW: 8, cellH: 16, baseline: 12}
		backends = append(backends, backend)
		return backend, nil
	}
	base := fontglyph.Spec{Family: "Go Mono", Size: 10, DPI: 96}
	atlas, err := newGlyphAtlasWithBackendFactory(&atlasTestRenderer{}, base, 1, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer atlas.close()
	active := atlas.activeContext
	pins := cloneContextPins(atlas.pinnedContexts)
	candidate := base
	candidate.Size = 12
	candidateKey, err := atlas.fontKey(candidate, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	delete(pins, active.key)
	pins[candidateKey] = struct{}{}
	admission, ok := atlas.prepareSpecWithPins(candidate, 1, 0, pins)
	if !ok {
		t.Fatal("candidate preparation failed")
	}
	if atlas.activeContext != active || len(atlas.contexts) != 1 || atlas.contexts[candidateKey] != nil || len(atlas.pinnedContexts) != 1 {
		t.Fatal("candidate preparation mutated active atlas state")
	}
	atlas.abortContextAdmission(admission)
	if atlas.activeContext != active || len(atlas.contexts) != 1 || backends[0].closeCalls != 0 || backends[1].closeCalls != 1 {
		t.Fatalf("abort state active=%v contexts=%d closes=%d/%d", atlas.activeContext == active, len(atlas.contexts), backends[0].closeCalls, backends[1].closeCalls)
	}
}
