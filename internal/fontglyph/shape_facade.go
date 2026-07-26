package fontglyph

import (
	"cervterm/internal/fontdesc"
	faceleaf "cervterm/internal/fontglyph/internal/face"
	shapepkg "cervterm/internal/fontglyph/shape"
)

func shapeResolver(index *FontIndex) shapepkg.Resolver {
	return shapepkg.NewResolver(func(family string) []shapepkg.SourceFace {
		faces := fontIndexFaces(index, family)
		out := make([]shapepkg.SourceFace, len(faces))
		for i, source := range faces {
			out[i] = shapepkg.SourceFace{
				Source: canonicalFontCacheSource(source.path), Index: source.index,
				Metadata: source.metadata,
			}
		}
		return out
	})
}

func resolvedPlanFromShape(plan shapepkg.Plan) resolvedFacePlan {
	return resolvedFacePlan{
		descriptor: plan.Descriptor,
		target:     plan.Target,
		selected: faceCandidate{
			path: plan.Selected.Source, index: plan.Selected.Index,
			metadata: plan.Selected.Metadata, rank: plan.Selected.Rank,
		},
		tier: plan.Tier, authoredIndex: plan.AuthoredIndex, synthetic: plan.Synthetic,
		canonicalFaceID: plan.CanonicalFaceID, resolvedKey: plan.ResolvedKey,
	}
}

func resolvedPlanToShape(plan resolvedFacePlan) shapepkg.Plan {
	return shapepkg.Plan{
		Descriptor: plan.descriptor,
		Target:     plan.target,
		Selected: shapepkg.Candidate{
			Source: plan.selected.path, Index: plan.selected.index,
			Metadata: plan.selected.metadata, Rank: plan.selected.rank,
		},
		Tier: plan.tier, AuthoredIndex: plan.authoredIndex, Synthetic: plan.synthetic,
		CanonicalFaceID: plan.canonicalFaceID, ResolvedKey: plan.resolvedKey,
	}
}

func resolvedPlansFromShape(plans []shapepkg.Plan) []resolvedFacePlan {
	out := make([]resolvedFacePlan, len(plans))
	for index, plan := range plans {
		out[index] = resolvedPlanFromShape(plan)
	}
	return out
}

func resolvePrimaryFacePlan(index *FontIndex, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, request fontdesc.RequestedFaceStyle) ([]resolvedFacePlan, error) {
	plans, err := shapeResolver(index).PrimaryPlans(environment, descriptors, request)
	return resolvedPlansFromShape(plans), err
}

func resolveDescriptorFacePlans(index *FontIndex, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, request fontdesc.RequestedFaceStyle, tier fontdesc.SourceTier, authoredOffset uint32) ([]resolvedFacePlan, error) {
	plans, err := shapeResolver(index).DescriptorPlans(environment, descriptors, request, tier, authoredOffset)
	return resolvedPlansFromShape(plans), err
}

func classifySyntheticFallback(target fontdesc.FaceTarget, metadata fontdesc.FaceMetadata) (fontdesc.SyntheticMode, bool) {
	return shapepkg.ClassifySynthetic(target, metadata)
}

func newResolvedFacePlan(environment fontdesc.FontEnvironmentKey, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, candidate faceCandidate, tier fontdesc.SourceTier, authoredIndex uint32, synthetic fontdesc.SyntheticMode) (resolvedFacePlan, error) {
	plan, err := shapepkg.NewPlan(environment, descriptor, target, shapepkg.Candidate{
		Source: candidate.path, Index: candidate.index, Metadata: candidate.metadata, Rank: candidate.rank,
	}, tier, authoredIndex, synthetic)
	return resolvedPlanFromShape(plan), err
}

func resolveEmbeddedFallbackPlan(environment fontdesc.FontEnvironmentKey, request fontdesc.RequestedFaceStyle) (resolvedFacePlan, error) {
	plan, err := (shapepkg.Resolver{}).EmbeddedPlan(environment, request)
	return resolvedPlanFromShape(plan), err
}

func resolveFaceCandidates(index *FontIndex, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, tier fontdesc.SourceTier, authoredOrder uint32) ([]faceCandidate, error) {
	candidates, err := shapeResolver(index).Candidates(descriptor, target, tier, authoredOrder)
	out := make([]faceCandidate, len(candidates))
	for i, candidate := range candidates {
		out[i] = faceCandidate{path: candidate.Source, index: candidate.Index, metadata: candidate.Metadata, rank: candidate.Rank}
	}
	return out, err
}

func (b *fallbackBackend) installShapePolicy() error {
	resolver := shapeResolver(b.index)
	policy, err := shapepkg.NewPolicy(shapepkg.PolicyConfig{
		Resolver: resolver, Environment: b.environment, Descriptors: b.descriptors, Fallback: b.fallback, Rules: b.rules,
		FeaturePayload: b.features.CanonicalBytes(),
		Hooks: shapepkg.PolicyHooks{
			Primary: func(request fontdesc.RequestedFaceStyle) (shapepkg.Plan, bool) {
				if b.primary == nil {
					return shapepkg.Plan{}, false
				}
				if _, ok := b.primary.backendForStyle(request); !ok {
					return shapepkg.Plan{}, false
				}
				return resolvedPlanToShape(b.primary.plans[request]), true
			},
			PrimaryCovers: func(request fontdesc.RequestedFaceStyle, content string) bool {
				backend, ok := b.primary.backendForStyle(request)
				return ok && b.covers(backend, content)
			},
			Loaded: func(plan shapepkg.Plan) bool { return b.loaded[plan.ResolvedKey] != nil },
			Load:   func(plan shapepkg.Plan) bool { return b.loadShapePlan(plan) },
			Covers: func(plan shapepkg.Plan, content string) bool {
				return b.covers(b.loaded[plan.ResolvedKey], content)
			},
			Discard: func(plan shapepkg.Plan) {
				if backend := b.loaded[plan.ResolvedKey]; backend != nil {
					backend.Close()
					delete(b.loaded, plan.ResolvedKey)
					b.removeLoadedOrder(plan.ResolvedKey)
				}
			},
		},
	})
	if err != nil {
		return err
	}
	b.policy = policy
	return nil
}

func (b *fallbackBackend) loadShapePlan(plan shapepkg.Plan) bool {
	if b == nil || b.closed {
		return false
	}
	if b.loaded[plan.ResolvedKey] != nil {
		return true
	}
	face, metrics, err := b.load(b.spec, resolvedPlanFromShape(plan))
	if err != nil {
		return false
	}
	backend := newOpenTypeBackendFromPrimary(b.spec, face, metrics)
	backend.features = b.features
	backend.fallbacksLoaded = true
	normalW, normalH, normalBaseline := b.primary.CellMetrics()
	backend.cellW, backend.cellH, backend.baseline = normalW, normalH, normalBaseline
	b.loaded[plan.ResolvedKey] = backend
	b.loadedOrder = append(b.loadedOrder, plan.ResolvedKey)
	return true
}

func (b *fallbackBackend) removeLoadedOrder(key fontdesc.ResolvedFaceKey) {
	for index := len(b.loadedOrder) - 1; index >= 0; index-- {
		if b.loadedOrder[index] != key {
			continue
		}
		copy(b.loadedOrder[index:], b.loadedOrder[index+1:])
		b.loadedOrder[len(b.loadedOrder)-1] = fontdesc.ResolvedFaceKey{}
		b.loadedOrder = b.loadedOrder[:len(b.loadedOrder)-1]
		return
	}
}

func (b *fallbackBackend) resolveContent(request fontdesc.RequestedFaceStyle, content string) (fallbackSelection, bool) {
	if b == nil || b.closed || b.primary == nil || b.policy == nil {
		return b.legacyResolveContent(request, content)
	}
	plan, ok := b.policy.Resolve(request, content)
	if !ok {
		return fallbackSelection{}, false
	}
	rootPlan := resolvedPlanFromShape(plan)
	if primary, primaryOK := b.primary.backendForStyle(request); primaryOK && b.primary.plans[request].resolvedKey == rootPlan.resolvedKey {
		return fallbackSelection{backend: primary, plan: rootPlan}, true
	}
	backend := b.loaded[rootPlan.resolvedKey]
	if backend == nil {
		return fallbackSelection{}, false
	}
	return fallbackSelection{backend: backend, plan: rootPlan}, true
}

func shapingFaceRef(source loadedFace) faceleaf.Ref {
	return faceleaf.NewRef(source.sfnt, source.sourcePath, source.faceIndex)
}

func shapedGlyphsFromShape(glyphs []shapepkg.Glyph) []ShapedGlyph {
	if glyphs == nil {
		return nil
	}
	out := make([]ShapedGlyph, len(glyphs))
	for index, glyph := range glyphs {
		out[index] = ShapedGlyph{GlyphID: glyph.GlyphID, XOffset: glyph.XOffset, YOffset: glyph.YOffset, XAdvance: glyph.XAdvance}
	}
	return out
}

func shapedGlyphsToShape(glyphs []ShapedGlyph) []shapepkg.Glyph {
	if glyphs == nil {
		return nil
	}
	out := make([]shapepkg.Glyph, len(glyphs))
	for index, glyph := range glyphs {
		out[index] = shapepkg.Glyph{GlyphID: glyph.GlyphID, XOffset: glyph.XOffset, YOffset: glyph.YOffset, XAdvance: glyph.XAdvance}
	}
	return out
}

type rootToShapeShaper struct{ root Shaper }

func (adapter rootToShapeShaper) Shape(cluster string, ref faceleaf.Ref, ppem uint16) ([]shapepkg.Glyph, bool) {
	glyphs, ok := adapter.root.Shape(cluster, loadedFaceFromShapingRef(ref), ppem)
	return shapedGlyphsToShape(glyphs), ok
}

func (adapter rootToShapeShaper) ShapeFeatures(cluster string, ref faceleaf.Ref, ppem uint16, features fontdesc.FeatureSet) ([]shapepkg.Glyph, bool) {
	if featureShaper, ok := adapter.root.(FeatureShaper); ok {
		glyphs, shaped := featureShaper.ShapeFeatures(cluster, loadedFaceFromShapingRef(ref), ppem, features)
		return shapedGlyphsToShape(glyphs), shaped
	}
	return adapter.Shape(cluster, ref, ppem)
}

func loadedFaceFromShapingRef(ref faceleaf.Ref) loadedFace {
	source, index, _ := ref.Source()
	return loadedFace{sfnt: ref.Parsed(), sourcePath: source, faceIndex: index}
}

type shapeToRootShaper struct{ inner shapepkg.Shaper }

func (adapter shapeToRootShaper) Shape(cluster string, source loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	glyphs, ok := adapter.inner.Shape(cluster, shapingFaceRef(source), ppem)
	return shapedGlyphsFromShape(glyphs), ok
}

func (adapter shapeToRootShaper) ShapeFeatures(cluster string, source loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	glyphs, ok := shapepkg.WithFeatures(adapter.inner, cluster, shapingFaceRef(source), ppem, features)
	return shapedGlyphsFromShape(glyphs), ok
}

func (adapter shapeToRootShaper) FeatureCapability() string {
	if reporter, ok := adapter.inner.(shapepkg.FeatureCapabilityReporter); ok {
		return reporter.FeatureCapability()
	}
	return "unsupported"
}

func rootShaperFromShape(shaper shapepkg.Shaper) Shaper {
	if adapter, ok := shaper.(rootToShapeShaper); ok {
		return adapter.root
	}
	return shapeToRootShaper{inner: shaper}
}
