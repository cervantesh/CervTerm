package fontglyph

import (
	"fmt"
	"sync"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/sfnt"
)

// contentResolvedFontBackend lets the atlas obtain the concrete face identity
// before positive or negative cache lookup. Resolution may lazily prepare one
// bounded fallback face; rasterization repeats the now-cached pure selection.
type contentResolvedFontBackend interface {
	styledFontBackend
	RuneResolution(request fontdesc.RequestedFaceStyle, value rune) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool)
	ClusterResolution(request fontdesc.RequestedFaceStyle, cluster string) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool)
}

type fallbackSelection struct {
	backend *OpenTypeBackend
	plan    resolvedFacePlan
}

type contentResolutionKey struct {
	request fontdesc.RequestedFaceStyle
	content string
}

type fallbackBackend struct {
	primary        *descriptorBackend
	spec           Spec
	environment    fontdesc.FontEnvironmentKey
	index          *FontIndex
	descriptors    []fontdesc.Descriptor
	fallback       []fontdesc.Descriptor
	rules          []fontdesc.Rule
	loaded         map[fontdesc.ResolvedFaceKey]*OpenTypeBackend
	loadFailed     map[fontdesc.ResolvedFaceKey]struct{}
	loadFailedRing []fontdesc.ResolvedFaceKey
	loadFailedNext int
	load           resolvedFacePlanLoader
	covers         func(*OpenTypeBackend, string) bool
	resolved       map[contentResolutionKey]fallbackSelection
	resolvedRing   []contentResolutionKey
	resolvedNext   int
	closed         bool
	closeOnce      sync.Once
}

// NewFallbackBackend extends primary descriptor routing with authored rules and
// ordered lazy fallback. The primary installation is still prepared atomically;
// rule and fallback faces are loaded only on the first matching cluster miss.
func NewFallbackBackend(spec Spec, environment fontdesc.FontEnvironmentKey, descriptors, fallback []fontdesc.Descriptor, rules []fontdesc.Rule) (Backend, error) {
	if len(fallback) > fontdesc.MaxFallbackDescriptors {
		return nil, fmt.Errorf("fallback descriptor count %d exceeds %d", len(fallback), fontdesc.MaxFallbackDescriptors)
	}
	if len(rules) > fontdesc.MaxRules {
		return nil, fmt.Errorf("font rule count %d exceeds %d", len(rules), fontdesc.MaxRules)
	}
	index := loadSystemFontIndex()
	primary, err := newDescriptorBackend(spec, environment, descriptors, index)
	if err != nil {
		return nil, err
	}
	backend := &fallbackBackend{
		primary: primary, spec: spec, environment: environment, index: index,
		descriptors: append([]fontdesc.Descriptor(nil), descriptors...),
		fallback:    append([]fontdesc.Descriptor(nil), fallback...),
		load:        loadResolvedFacePlan,
		covers:      backendCoversCluster,
		rules:       cloneResolvedRules(rules),
		loaded:      make(map[fontdesc.ResolvedFaceKey]*OpenTypeBackend),
		loadFailed:  make(map[fontdesc.ResolvedFaceKey]struct{}),
		resolved:    make(map[contentResolutionKey]fallbackSelection),
	}
	// Descriptor/rule fallback owns all non-primary resolution. Prevent the
	// legacy OpenTypeBackend from eagerly appending its implicit fallback set.
	for _, child := range primary.backends {
		if child != nil {
			child.fallbacksLoaded = true
		}
	}
	return backend, nil
}

func cloneResolvedRules(rules []fontdesc.Rule) []fontdesc.Rule {
	cloned := make([]fontdesc.Rule, len(rules))
	for index, rule := range rules {
		cloned[index] = rule
		cloned[index].Match.Styles = append([]fontdesc.Style(nil), rule.Match.Styles...)
		cloned[index].Match.Ranges = append([]fontdesc.RuneRange(nil), rule.Match.Ranges...)
	}
	return cloned
}

func (b *fallbackBackend) CellMetrics() (int, int, int) {
	if b == nil || b.primary == nil || b.closed {
		return 0, 0, 0
	}
	return b.primary.CellMetrics()
}

func (b *fallbackBackend) TextRasterEngine() string {
	if b == nil || b.primary == nil || b.closed {
		return "go"
	}
	return b.primary.TextRasterEngine()
}

func (b *fallbackBackend) SupportsLigatures() bool {
	return b != nil && !b.closed && b.primary != nil && b.primary.SupportsLigatures()
}

func (b *fallbackBackend) StyleResolution(request fontdesc.RequestedFaceStyle) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	if b == nil || b.closed || b.primary == nil {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	return b.primary.StyleResolution(request)
}

func (b *fallbackBackend) RuneResolution(request fontdesc.RequestedFaceStyle, value rune) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	selection, ok := b.resolveContent(request, string(value))
	if !ok {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	return selection.plan.resolvedKey, selection.plan.synthetic, true
}

func (b *fallbackBackend) ClusterResolution(request fontdesc.RequestedFaceStyle, cluster string) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool) {
	selection, ok := b.resolveContent(request, cluster)
	if !ok {
		return fontdesc.ResolvedFaceKey{}, fontdesc.SyntheticNone, false
	}
	return selection.plan.resolvedKey, selection.plan.synthetic, true
}

func (b *fallbackBackend) Rasterize(value rune, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeStyle(fontdesc.RequestedFaceStyleNormal, value, cellSpan)
}

func (b *fallbackBackend) RasterizeCluster(cluster string, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeClusterStyle(fontdesc.RequestedFaceStyleNormal, cluster, cellSpan)
}

func (b *fallbackBackend) RasterizeRun(run string, cellSpan int) (RasterizedGlyph, bool) {
	return b.RasterizeRunStyle(fontdesc.RequestedFaceStyleNormal, run, cellSpan)
}

func (b *fallbackBackend) RasterizeStyle(request fontdesc.RequestedFaceStyle, value rune, cellSpan int) (RasterizedGlyph, bool) {
	selection, ok := b.resolveContent(request, string(value))
	if !ok {
		return RasterizedGlyph{}, false
	}
	return selection.backend.Rasterize(value, cellSpan)
}

func (b *fallbackBackend) RasterizeClusterStyle(request fontdesc.RequestedFaceStyle, cluster string, cellSpan int) (RasterizedGlyph, bool) {
	selection, ok := b.resolveContent(request, cluster)
	if !ok {
		return RasterizedGlyph{}, false
	}
	return selection.backend.RasterizeCluster(cluster, cellSpan)
}

func (b *fallbackBackend) RasterizeRunStyle(request fontdesc.RequestedFaceStyle, run string, cellSpan int) (RasterizedGlyph, bool) {
	selection, ok := b.resolveContent(request, run)
	if !ok {
		return RasterizedGlyph{}, false
	}
	return selection.backend.RasterizeRun(run, cellSpan)
}

func (b *fallbackBackend) resolveContent(request fontdesc.RequestedFaceStyle, content string) (fallbackSelection, bool) {
	if b == nil || b.closed || b.primary == nil || content == "" || request > fontdesc.RequestedFaceStyleBoldItalic {
		return fallbackSelection{}, false
	}
	cacheKey := contentResolutionKey{request: request, content: content}
	if selected, ok := b.resolved[cacheKey]; ok {
		return selected, true
	}
	requestedTarget, _ := (fontdesc.Descriptor{Family: "requested-style"}).EffectiveTarget(request)
	for index, rule := range b.rules {
		if !rule.Matches(content, requestedTarget) {
			continue
		}
		plans, err := resolveDescriptorFacePlans(b.index, b.environment, []fontdesc.Descriptor{rule.Use}, request, fontdesc.SourceTierRule, uint32(index))
		if err == nil {
			if selected, ok := b.tryPlans(content, plans, nil); ok {
				return b.rememberResolution(cacheKey, selected), true
			}
		}
	}

	primaryBackend, primaryOK := b.primary.backendForStyle(request)
	primaryPlan := b.primary.plans[request]
	if primaryOK && primaryPlan.tier == fontdesc.SourceTierPrimary && b.covers(primaryBackend, content) {
		return b.rememberResolution(cacheKey, fallbackSelection{backend: primaryBackend, plan: primaryPlan}), true
	}
	primaryPlans, _ := resolveDescriptorFacePlans(b.index, b.environment, b.descriptors, request, fontdesc.SourceTierPrimary, 0)
	if selected, ok := b.tryPlans(content, primaryPlans, &primaryPlan.resolvedKey); ok {
		return b.rememberResolution(cacheKey, selected), true
	}
	fallbackPlans, _ := resolveDescriptorFacePlans(b.index, b.environment, b.fallback, request, fontdesc.SourceTierFallback, 0)
	if selected, ok := b.tryPlans(content, fallbackPlans, nil); ok {
		return b.rememberResolution(cacheKey, selected), true
	}

	if primaryOK && primaryPlan.tier == fontdesc.SourceTierEmbedded {
		return b.rememberResolution(cacheKey, fallbackSelection{backend: primaryBackend, plan: primaryPlan}), true
	}
	embedded, err := resolveEmbeddedFallbackPlan(b.environment, request)
	if err != nil {
		return fallbackSelection{}, false
	}
	selected, ok := b.loadFinalPlan(embedded)
	if !ok {
		return fallbackSelection{}, false
	}
	return b.rememberResolution(cacheKey, selected), true
}

func (b *fallbackBackend) recordLoadFailure(key fontdesc.ResolvedFaceKey) {
	if _, exists := b.loadFailed[key]; exists {
		return
	}
	if len(b.loadFailedRing) < fontdesc.MaxNegativeEntries {
		b.loadFailedRing = append(b.loadFailedRing, key)
	} else {
		victim := b.loadFailedRing[b.loadFailedNext]
		delete(b.loadFailed, victim)
		b.loadFailedRing[b.loadFailedNext] = key
		b.loadFailedNext = (b.loadFailedNext + 1) % len(b.loadFailedRing)
	}
	b.loadFailed[key] = struct{}{}
}

func (b *fallbackBackend) rememberResolution(key contentResolutionKey, selected fallbackSelection) fallbackSelection {
	if _, exists := b.resolved[key]; exists {
		b.resolved[key] = selected
		return selected
	}
	if len(b.resolvedRing) < fontdesc.MaxNegativeEntries {
		b.resolvedRing = append(b.resolvedRing, key)
	} else {
		victim := b.resolvedRing[b.resolvedNext]
		delete(b.resolved, victim)
		b.resolvedRing[b.resolvedNext] = key
		b.resolvedNext = (b.resolvedNext + 1) % len(b.resolvedRing)
	}
	b.resolved[key] = selected
	return selected
}

func (b *fallbackBackend) loadFinalPlan(plan resolvedFacePlan) (fallbackSelection, bool) {
	if backend := b.loaded[plan.resolvedKey]; backend != nil {
		return fallbackSelection{backend: backend, plan: plan}, true
	}
	if _, failed := b.loadFailed[plan.resolvedKey]; failed {
		return fallbackSelection{}, false
	}
	face, metrics, err := b.load(b.spec, plan)
	if err != nil {
		b.recordLoadFailure(plan.resolvedKey)
		return fallbackSelection{}, false
	}
	backend := newOpenTypeBackendFromPrimary(b.spec, face, metrics)
	backend.fallbacksLoaded = true
	normalW, normalH, normalBaseline := b.primary.CellMetrics()
	backend.cellW, backend.cellH, backend.baseline = normalW, normalH, normalBaseline
	b.loaded[plan.resolvedKey] = backend
	return fallbackSelection{backend: backend, plan: plan}, true
}

func (b *fallbackBackend) tryPlans(content string, plans []resolvedFacePlan, skip *fontdesc.ResolvedFaceKey) (fallbackSelection, bool) {
	attemptedFaces := make(map[fontdesc.CanonicalFaceID]struct{}, len(plans))
	for _, plan := range plans {
		if skip != nil && plan.resolvedKey == *skip {
			continue
		}
		if _, duplicate := attemptedFaces[plan.canonicalFaceID]; duplicate {
			continue
		}
		attemptedFaces[plan.canonicalFaceID] = struct{}{}
		if backend := b.loaded[plan.resolvedKey]; backend != nil {
			if b.covers(backend, content) {
				return fallbackSelection{backend: backend, plan: plan}, true
			}
			continue
		}
		if _, failed := b.loadFailed[plan.resolvedKey]; failed {
			continue
		}
		face, metrics, err := b.load(b.spec, plan)
		if err != nil {
			b.recordLoadFailure(plan.resolvedKey)
			continue
		}
		backend := newOpenTypeBackendFromPrimary(b.spec, face, metrics)
		backend.fallbacksLoaded = true
		_, normalH, normalBaseline := b.primary.CellMetrics()
		normalW, _, _ := b.primary.CellMetrics()
		backend.cellW, backend.cellH, backend.baseline = normalW, normalH, normalBaseline
		if !b.covers(backend, content) {
			backend.Close()
			continue
		}
		b.loaded[plan.resolvedKey] = backend
		return fallbackSelection{backend: backend, plan: plan}, true
	}
	return fallbackSelection{}, false
}

func backendCoversCluster(backend *OpenTypeBackend, cluster string) bool {
	if backend == nil || backend.closed || len(backend.faces) == 0 || cluster == "" {
		return false
	}
	face := backend.faces[0]
	for _, value := range cluster {
		if value == 0 || value < 32 || fontdesc.IsDefaultIgnorableRune(value) {
			continue
		}
		if !loadedFaceMapsRune(backend, face, value) {
			return false
		}
	}
	if backend.shaper == nil {
		return false
	}
	shaped, ok := backend.shaper.Shape(cluster, face, backend.ppem)
	if !ok || len(shaped) == 0 {
		return false
	}
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			return false
		}
	}
	return true
}

func loadedFaceMapsRune(backend *OpenTypeBackend, face loadedFace, value rune) bool {
	if face.sfnt == nil {
		return false
	}
	var buffer sfnt.Buffer
	glyph, err := face.sfnt.GlyphIndex(&buffer, value)
	if err == nil && glyph != 0 {
		return true
	}
	return backend.faceHasColorGlyph(face, value)
}

func (b *fallbackBackend) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.closed = true
		clear(b.resolved)
		b.resolvedRing = nil
		clear(b.loadFailed)
		b.loadFailedRing = nil
		for key, backend := range b.loaded {
			if backend != nil {
				backend.Close()
			}
			delete(b.loaded, key)
		}
		if b.primary != nil {
			b.primary.Close()
			b.primary = nil
		}
	})
}

var _ contentResolvedFontBackend = (*fallbackBackend)(nil)
