//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/render"
)

type atlasFontKey struct {
	environment    fontdesc.FontEnvironmentKey
	family         string
	sizeBits       uint64
	dpiBits        uint64
	textRaster     string
	textGammaBits  uint64
	textDarkenBits uint64
}

type atlasFontContext struct {
	key          atlasFontKey
	resolvedFace fontdesc.ResolvedFaceKey
	backend      fontglyph.Backend
	cellW        int
	cellH        int
	baseline     int
	coverageLUT  *[256]uint8
	prewarmed    bool
}

type atlasBackendFactory func(fontglyph.Spec) (fontglyph.Backend, error)

type atlasLigatureBackend interface {
	SupportsLigatures() bool
	RasterizeRun(run string, cellSpan int) (fontglyph.RasterizedGlyph, bool)
}

func newAtlasFontKey(spec fontglyph.Spec, textGamma, textDarken float64) atlasFontKey {
	key, err := makeAtlasFontKey(spec, textGamma, textDarken)
	if err != nil {
		panic(err)
	}
	return key
}

func makeAtlasFontKey(spec fontglyph.Spec, textGamma, textDarken float64) (atlasFontKey, error) {
	descriptor, err := (fontdesc.Descriptor{Family: spec.Family}).Normalize()
	if err != nil {
		return atlasFontKey{}, err
	}
	if spec.DPI <= 0 || spec.DPI > math.MaxUint32 {
		return atlasFontKey{}, fmt.Errorf("font DPI %.2f is outside identity bounds", spec.DPI)
	}
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{
		Descriptors:   []fontdesc.Descriptor{descriptor},
		BaseSizeBits:  stableFloatBits(spec.Size),
		PaneZoomBits:  stableFloatBits(1),
		DPI:           uint32(math.Round(spec.DPI)),
		RasterMode:    spec.TextRaster,
		GammaBits:     stableFloatBits(textGamma),
		DarkeningBits: stableFloatBits(textDarken),
	})
	if err != nil {
		return atlasFontKey{}, err
	}
	if environment == (fontdesc.FontEnvironmentKey{}) {
		return atlasFontKey{}, errors.New("font environment identity is zero")
	}
	return atlasFontKey{
		environment:    environment,
		family:         spec.Family,
		sizeBits:       stableFloatBits(spec.Size),
		dpiBits:        stableFloatBits(spec.DPI),
		textRaster:     spec.TextRaster,
		textGammaBits:  stableFloatBits(textGamma),
		textDarkenBits: stableFloatBits(textDarken),
	}, nil
}

func stableFloatBits(value float64) uint64 {
	if value == 0 {
		return 0
	}
	return math.Float64bits(value)
}

func newAtlasBackend(spec fontglyph.Spec) (fontglyph.Backend, error) {
	return fontglyph.NewOpenTypeBackend(spec)
}

func newGlyphAtlasWithBackendFactory(
	r gpu.Renderer,
	spec fontglyph.Spec,
	textGamma, textDarken float64,
	factory atlasBackendFactory,
) (*glyphAtlas, error) {
	ctx, err := makeAtlasFontContext(spec, textGamma, textDarken, factory)
	if err != nil {
		return nil, err
	}
	return newGlyphAtlasWithPreparedContext(r, ctx, factory), nil
}

func newGlyphAtlasWithPreparedContext(r gpu.Renderer, ctx *atlasFontContext, factory atlasBackendFactory) *glyphAtlas {
	a := &glyphAtlas{
		r:              r,
		entries:        make(map[atlasKey]atlasEntry),
		generation:     1,
		contexts:       map[atlasFontKey]*atlasFontContext{ctx.key: ctx},
		backendFactory: factory,
	}
	for i := range a.pages {
		a.pages[i].packer = newShelfPacker(atlasPageSize, atlasPageSize)
	}
	// The atlas owns one fixed two-page texture pool for every font context.
	r.ConfigureAtlas(atlasPageCount, atlasPageSize)
	a.activateContext(ctx)
	a.prewarmASCII()
	return a
}

func makeAtlasFontContext(
	spec fontglyph.Spec,
	textGamma, textDarken float64,
	factory atlasBackendFactory,
) (*atlasFontContext, error) {
	if factory == nil {
		return nil, errors.New("nil atlas backend factory")
	}
	backend, err := factory(spec)
	if err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, errors.New("atlas backend factory returned nil backend")
	}
	cellW, cellH, baseline := backend.CellMetrics()
	ctx, err := makeAtlasFontContextFromBackend(spec, textGamma, textDarken, backend, fontInstallationMetrics{cellW: cellW, cellH: cellH, baseline: baseline})
	if err != nil {
		backend.Close()
		return nil, err
	}
	return ctx, nil
}

func makeAtlasFontContextFromBackend(spec fontglyph.Spec, textGamma, textDarken float64, backend fontglyph.Backend, metrics fontInstallationMetrics) (*atlasFontContext, error) {
	if backend == nil {
		return nil, errors.New("nil atlas backend")
	}
	key, err := makeAtlasFontKey(spec, textGamma, textDarken)
	if err != nil {
		return nil, fmt.Errorf("font environment identity: %w", err)
	}
	face := fontdesc.CanonicalFaceIDFromBytes([]byte("legacy:" + strings.ToLower(strings.TrimSpace(spec.Family))))
	resolvedFace, err := fontdesc.NewResolvedFaceKey(fontdesc.ResolvedFaceInput{
		Environment: key.environment,
		Face:        face,
		Tier:        fontdesc.SourceTierPrimary,
		Target: fontdesc.FaceTarget{
			Weight:  fontdesc.DefaultWeight,
			Style:   fontdesc.StyleNormal,
			Stretch: fontdesc.DefaultStretch,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("resolved face identity: %w", err)
	}
	if resolvedFace == (fontdesc.ResolvedFaceKey{}) {
		return nil, errors.New("resolved face identity is zero")
	}
	ctx := &atlasFontContext{
		key:          key,
		resolvedFace: resolvedFace,
		backend:      backend,
		cellW:        metrics.cellW,
		cellH:        metrics.cellH,
		baseline:     metrics.baseline,
	}
	if textGamma != 1 || textDarken != 0 {
		lut := render.CoverageLUT(textGamma, textDarken)
		ctx.coverageLUT = &lut
	}
	return ctx, nil
}

// useSpec selects an existing raster context or creates one without touching the
// shared GPU pages or atlas generation. It returns the selected cell metrics.
func (a *glyphAtlas) useSpec(spec fontglyph.Spec, textGamma, textDarken float64) (int, int, int, bool) {
	if a == nil || a.closed {
		return 0, 0, 0, false
	}
	key := newAtlasFontKey(spec, textGamma, textDarken)
	if ctx, ok := a.contexts[key]; ok {
		a.activateContext(ctx)
		a.prewarmASCII()
		return ctx.cellW, ctx.cellH, ctx.baseline, true
	}
	ctx, err := makeAtlasFontContext(spec, textGamma, textDarken, a.backendFactory)
	if err != nil {
		return 0, 0, 0, false
	}
	a.contexts[key] = ctx
	a.activateContext(ctx)
	a.prewarmASCII()
	return ctx.cellW, ctx.cellH, ctx.baseline, true
}

// retainContexts bounds CPU-side raster resources to the specs used by visible
// panes. Uploaded atlas entries remain valid after their backend is closed.
func (a *glyphAtlas) retainContexts(keep map[atlasFontKey]struct{}) {
	if a == nil || a.closed {
		return
	}
	if a.activeContext != nil {
		keep[a.activeContext.key] = struct{}{}
	}
	closed := make([]fontglyph.Backend, 0)
	for key, ctx := range a.contexts {
		if _, ok := keep[key]; ok {
			continue
		}
		sharedWithKept := false
		for keptKey := range keep {
			if kept := a.contexts[keptKey]; kept != nil && sameAtlasBackend(ctx.backend, kept.backend) {
				sharedWithKept = true
				break
			}
		}
		if !sharedWithKept {
			alreadyClosed := false
			for _, backend := range closed {
				if sameAtlasBackend(ctx.backend, backend) {
					alreadyClosed = true
					break
				}
			}
			if !alreadyClosed {
				ctx.backend.Close()
				closed = append(closed, ctx.backend)
			}
		}
		delete(a.contexts, key)
	}
}

// reconfigure remains as a compatibility wrapper until pane rendering selects
// contexts directly. Switching specs does not clear or reallocate the atlas.
func (a *glyphAtlas) reconfigure(spec fontglyph.Spec, textGamma, textDarken float64) bool {
	_, _, _, ok := a.useSpec(spec, textGamma, textDarken)
	return ok
}

func (a *glyphAtlas) activateContext(ctx *atlasFontContext) {
	a.activeContext = ctx
	// Keep the legacy app-facing fields coherent during the Phase 3 transition.
	a.backend = ctx.backend
	a.cellW, a.cellH, a.baseline = ctx.cellW, ctx.cellH, ctx.baseline
	a.coverageLUT = ctx.coverageLUT
}

func (a *glyphAtlas) prewarmASCII() {
	ctx := a.activeContext
	if ctx == nil || ctx.prewarmed || a.prewarming {
		return
	}
	a.prewarming = true
	defer func() { a.prewarming = false }()

	// Prewarming is opportunistic: it must never evict glyphs already needed by
	// another visible size. Misses remain lazy and may use the normal overflow
	// policy when the glyph is actually drawn.
	for r := rune(32); r <= 126; r++ {
		_, _ = a.cachedRune(r)
	}
	ctx.prewarmed = true
}

func activeLigatureBackend(ctx *atlasFontContext) (atlasLigatureBackend, bool) {
	if ctx == nil {
		return nil, false
	}
	backend, ok := ctx.backend.(atlasLigatureBackend)
	return backend, ok
}

func (a *glyphAtlas) close() {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	closedBackends := make([]fontglyph.Backend, 0, len(a.contexts))
	for _, ctx := range a.contexts {
		duplicate := false
		for _, backend := range closedBackends {
			if sameAtlasBackend(backend, ctx.backend) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		ctx.backend.Close()
		closedBackends = append(closedBackends, ctx.backend)
	}
	a.r.Destroy()
	a.entries = nil
	a.runNegative = nil
	a.contexts = nil
	a.activeContext = nil
	a.backend = nil
	a.coverageLUT = nil
}

func sameAtlasBackend(left, right fontglyph.Backend) bool {
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if !leftValue.IsValid() || !rightValue.IsValid() || leftValue.Type() != rightValue.Type() {
		return false
	}
	if !leftValue.Type().Comparable() {
		return false
	}
	return leftValue.Interface() == rightValue.Interface()
}
