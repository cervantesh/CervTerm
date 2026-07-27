package fontglyph

import (
	"fmt"
	"sync"

	"cervterm/internal/fontdesc"
	shapepkg "cervterm/internal/fontglyph/shape"

	"golang.org/x/image/font/sfnt"
)

// contentResolvedFontBackend lets the atlas obtain the concrete face identity
// before positive or negative cache lookup.
type contentResolvedFontBackend interface {
	styledFontBackend
	RuneResolution(request fontdesc.RequestedFaceStyle, value rune) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool)
	ClusterResolution(request fontdesc.RequestedFaceStyle, cluster string) (fontdesc.ResolvedFaceKey, fontdesc.SyntheticMode, bool)
}

type fallbackSelection struct {
	backend *OpenTypeBackend
	plan    resolvedFacePlan
}

type fallbackBackend struct {
	primary     *descriptorBackend
	spec        Spec
	environment fontdesc.FontEnvironmentKey
	index       *FontIndex
	features    fontdesc.FeatureSet
	loaded      map[fontdesc.ResolvedFaceKey]*OpenTypeBackend
	loadedOrder []fontdesc.ResolvedFaceKey
	load        resolvedFacePlanLoader
	covers      func(*OpenTypeBackend, string) bool
	policy      *shapepkg.Policy
	closed      bool
	closeOnce   sync.Once
}

// NewFallbackBackend extends primary descriptor routing with authored rules and
// ordered lazy fallback. Rule and fallback faces are loaded only after the first
// complete-cluster miss.
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
		load: loadResolvedFacePlan, covers: backendCoversCluster,
		loaded: make(map[fontdesc.ResolvedFaceKey]*OpenTypeBackend),
	}
	// Descriptor/rule fallback owns all non-primary resolution. Prevent the
	// legacy OpenTypeBackend from eagerly appending its implicit fallback set.
	for _, child := range primary.backends {
		if child != nil {
			child.fallbacksLoaded = true
		}
	}
	if err := backend.installShapePolicy(descriptors, fallback, rules); err != nil {
		backend.Close()
		return nil, err
	}
	return backend, nil
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

func backendCoversCluster(backend *OpenTypeBackend, cluster string) bool {
	if backend == nil || backend.closed || len(backend.faces) == 0 || cluster == "" {
		return false
	}
	selected := backend.faces[0]
	for _, value := range cluster {
		if value == 0 || value < 32 || fontdesc.IsDefaultIgnorableRune(value) {
			continue
		}
		if !loadedFaceMapsRune(backend, selected, value) {
			return false
		}
	}
	if backend.shaper == nil {
		return false
	}
	shaped, ok := shapeWithFeatures(backend.shaper, cluster, selected, backend.ppem, backend.features)
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

func loadedFaceMapsRune(backend *OpenTypeBackend, selected loadedFace, value rune) bool {
	if selected.sfnt == nil {
		return false
	}
	var buffer sfnt.Buffer
	glyph, err := selected.sfnt.GlyphIndex(&buffer, value)
	if err == nil && glyph != 0 {
		return true
	}
	return backend.faceHasColorGlyph(selected, value)
}

func (b *fallbackBackend) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.closed = true
		if b.policy != nil {
			b.policy.Close()
			b.policy = nil
		}
		for index := len(b.loadedOrder) - 1; index >= 0; index-- {
			key := b.loadedOrder[index]
			if backend := b.loaded[key]; backend != nil {
				backend.Close()
				delete(b.loaded, key)
			}
		}
		clear(b.loaded)
		b.loadedOrder = nil
		if b.primary != nil {
			b.primary.Close()
			b.primary = nil
		}
	})
}

var _ contentResolvedFontBackend = (*fallbackBackend)(nil)
