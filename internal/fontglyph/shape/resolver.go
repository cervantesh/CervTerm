package shape

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cervterm/internal/fontdesc"
)

// PrimaryPlans resolves every usable primary descriptor into ordered attempts.
func (r Resolver) PrimaryPlans(environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, request fontdesc.RequestedFaceStyle) ([]Plan, error) {
	return r.DescriptorPlans(environment, descriptors, request, fontdesc.SourceTierPrimary, 0)
}

// DescriptorPlans is the shared pure resolver for primary, rule, and fallback tiers.
func (r Resolver) DescriptorPlans(environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, request fontdesc.RequestedFaceStyle, tier fontdesc.SourceTier, authoredOffset uint32) ([]Plan, error) {
	if environment == (fontdesc.FontEnvironmentKey{}) {
		return nil, fmt.Errorf("resolve primary face plan: zero font environment key")
	}
	if request > fontdesc.RequestedFaceStyleBoldItalic {
		return nil, fmt.Errorf("resolve primary face plan: invalid requested face style %d", request)
	}
	plans := make([]Plan, 0)
	failures := make([]error, 0)
	for authoredIndex, authored := range descriptors {
		descriptor, err := authored.Normalize()
		if err != nil {
			failures = append(failures, fmt.Errorf("descriptor %d: %w", authoredIndex, err))
			continue
		}
		target, err := descriptor.EffectiveTarget(request)
		if err != nil {
			failures = append(failures, fmt.Errorf("descriptor %d family %q target: %w", authoredIndex, descriptor.Family, err))
			continue
		}
		order := authoredOffset + uint32(authoredIndex)
		if r.lookup == nil {
			failures = append(failures, fmt.Errorf("descriptor %d family %q: resolve font family %q: nil font index", authoredIndex, descriptor.Family, descriptor.Family))
			continue
		}
		sources := r.lookup(descriptor.Family)
		if len(sources) == 0 {
			failures = append(failures, fmt.Errorf("descriptor %d family %q: resolve font family %q: no discovered faces", authoredIndex, descriptor.Family, descriptor.Family))
			continue
		}
		matchedSelector := false
		for _, source := range sources {
			metadata := source.Metadata.Normalized()
			if descriptor.CollectionIndex.Present && (source.Index < 0 || uint32(source.Index) != descriptor.CollectionIndex.Value) {
				continue
			}
			if descriptor.CollectionFace != "" && normalizeFamily(metadata.Subfamily) != normalizeFamily(descriptor.CollectionFace) {
				continue
			}
			matchedSelector = true
			synthetic, compatible := classifySyntheticNormalized(target, metadata)
			if !compatible {
				continue
			}
			rank, rankErr := fontdesc.Rank(target, metadata, fontdesc.RankingTieBreaks{
				Tier: tier, AuthoredOrder: order, Synthetic: synthetic != fontdesc.SyntheticNone, CanonicalSource: source.Source,
			})
			if rankErr != nil {
				failures = append(failures, fmt.Errorf("descriptor %d family %q candidate %q index %d: %w", authoredIndex, descriptor.Family, source.Source, source.Index, rankErr))
				continue
			}
			candidate := Candidate{Source: source.Source, Index: source.Index, Metadata: metadata, Rank: rank}
			plan, planErr := NewPlan(environment, descriptor, target, candidate, tier, order, synthetic)
			if planErr != nil {
				failures = append(failures, fmt.Errorf("descriptor %d family %q candidate %q index %d: %w", authoredIndex, descriptor.Family, candidate.Source, candidate.Index, planErr))
				continue
			}
			plans = append(plans, plan)
		}
		if !matchedSelector {
			switch {
			case descriptor.CollectionIndex.Present:
				failures = append(failures, fmt.Errorf("descriptor %d family %q: resolve font family %q: no face at collection_index %d", authoredIndex, descriptor.Family, descriptor.Family, descriptor.CollectionIndex.Value))
			case descriptor.CollectionFace != "":
				failures = append(failures, fmt.Errorf("descriptor %d family %q: resolve font family %q: no face named %q", authoredIndex, descriptor.Family, descriptor.Family, descriptor.CollectionFace))
			default:
				failures = append(failures, fmt.Errorf("descriptor %d family %q: resolve font family %q: no rankable faces", authoredIndex, descriptor.Family, descriptor.Family))
			}
		}
	}
	if len(plans) == 0 {
		if len(failures) == 0 {
			failures = append(failures, fmt.Errorf("no descriptors produced compatible candidates"))
		}
		return nil, fmt.Errorf("resolve descriptor face plans: no load attempts: %w", errors.Join(failures...))
	}
	sort.Slice(plans, func(i, j int) bool { return fontdesc.Compare(plans[i].Selected.Rank, plans[j].Selected.Rank) < 0 })
	return plans, nil
}

// ClassifySynthetic maps one concrete face onto an effective target.
func ClassifySynthetic(target fontdesc.FaceTarget, metadata fontdesc.FaceMetadata) (fontdesc.SyntheticMode, bool) {
	return classifySyntheticNormalized(target, metadata.Normalized())
}

func classifySyntheticNormalized(target fontdesc.FaceTarget, metadata fontdesc.FaceMetadata) (fontdesc.SyntheticMode, bool) {
	synthetic := fontdesc.SyntheticNone
	switch target.Style {
	case fontdesc.StyleNormal:
		if metadata.Style != fontdesc.StyleNormal {
			return fontdesc.SyntheticNone, false
		}
	case fontdesc.StyleItalic, fontdesc.StyleOblique:
		switch metadata.Style {
		case fontdesc.StyleItalic, fontdesc.StyleOblique:
		case fontdesc.StyleNormal:
			synthetic |= fontdesc.SyntheticItalic
		default:
			return fontdesc.SyntheticNone, false
		}
	default:
		return fontdesc.SyntheticNone, false
	}
	if target.Weight >= 700 && metadata.Weight < 600 {
		synthetic |= fontdesc.SyntheticBold
	}
	return synthetic, true
}

// NewPlan constructs the stable face and resolved identities for one attempt.
func NewPlan(environment fontdesc.FontEnvironmentKey, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, candidate Candidate, tier fontdesc.SourceTier, authoredIndex uint32, synthetic fontdesc.SyntheticMode) (Plan, error) {
	canonicalFaceID := fontdesc.CanonicalFaceIDFromBytes([]byte(candidate.Source + "#" + strconv.Itoa(candidate.Index)))
	resolvedKey, err := fontdesc.NewResolvedFaceKey(fontdesc.ResolvedFaceInput{
		Environment: environment, Face: canonicalFaceID, Tier: tier, SourceIndex: authoredIndex, Target: target, Synthetic: synthetic,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("build resolved face key: %w", err)
	}
	return Plan{
		Descriptor: descriptor, Target: target, Selected: candidate, Tier: tier, AuthoredIndex: authoredIndex,
		Synthetic: synthetic, CanonicalFaceID: canonicalFaceID, ResolvedKey: resolvedKey,
	}, nil
}

// EmbeddedPlan returns the stable final embedded fallback attempt.
func (r Resolver) EmbeddedPlan(environment fontdesc.FontEnvironmentKey, request fontdesc.RequestedFaceStyle) (Plan, error) {
	if environment == (fontdesc.FontEnvironmentKey{}) {
		return Plan{}, fmt.Errorf("resolve embedded fallback plan: zero font environment key")
	}
	descriptor := fontdesc.Descriptor{Family: "Go Mono"}.Normalized()
	target, err := descriptor.EffectiveTarget(request)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	metadata := fontdesc.FaceMetadata{Family: "Go Mono", Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100, CollectionIndex: 0}.Normalized()
	synthetic, _ := ClassifySynthetic(target, metadata)
	rank, err := fontdesc.Rank(target, metadata, fontdesc.RankingTieBreaks{
		Tier: fontdesc.SourceTierEmbedded, Synthetic: synthetic != fontdesc.SyntheticNone, CanonicalSource: "embedded:gomono",
	})
	if err != nil {
		return Plan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	plan, err := NewPlan(environment, descriptor, target, Candidate{Source: "embedded:gomono", Index: 0, Metadata: metadata, Rank: rank}, fontdesc.SourceTierEmbedded, 0, synthetic)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	return plan, nil
}

// Candidates performs deterministic selection without font I/O.
func (r Resolver) Candidates(descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, tier fontdesc.SourceTier, authoredOrder uint32) ([]Candidate, error) {
	descriptor, err := descriptor.Normalize()
	if err != nil {
		return nil, fmt.Errorf("normalize font descriptor: %w", err)
	}
	if r.lookup == nil {
		return nil, fmt.Errorf("resolve font family %q: nil font index", descriptor.Family)
	}
	faces := r.lookup(descriptor.Family)
	if len(faces) == 0 {
		return nil, fmt.Errorf("resolve font family %q: no discovered faces", descriptor.Family)
	}
	candidates := make([]Candidate, 0, len(faces))
	for _, source := range faces {
		metadata := source.Metadata.Normalized()
		if descriptor.CollectionIndex.Present && (source.Index < 0 || uint32(source.Index) != descriptor.CollectionIndex.Value) {
			continue
		}
		if descriptor.CollectionFace != "" && normalizeFamily(metadata.Subfamily) != normalizeFamily(descriptor.CollectionFace) {
			continue
		}
		rank, rankErr := fontdesc.Rank(target, metadata, fontdesc.RankingTieBreaks{
			Tier: tier, AuthoredOrder: authoredOrder, CanonicalSource: source.Source,
		})
		if rankErr != nil {
			return nil, fmt.Errorf("rank font face %q index %d: %w", source.Source, source.Index, rankErr)
		}
		candidates = append(candidates, Candidate{Source: source.Source, Index: source.Index, Metadata: metadata, Rank: rank})
	}
	if len(candidates) == 0 {
		if descriptor.CollectionIndex.Present {
			return nil, fmt.Errorf("resolve font family %q: no face at collection_index %d", descriptor.Family, descriptor.CollectionIndex.Value)
		}
		if descriptor.CollectionFace != "" {
			return nil, fmt.Errorf("resolve font family %q: no face named %q", descriptor.Family, descriptor.CollectionFace)
		}
		return nil, fmt.Errorf("resolve font family %q: no rankable faces", descriptor.Family)
	}
	sort.Slice(candidates, func(i, j int) bool { return fontdesc.Compare(candidates[i].Rank, candidates[j].Rank) < 0 })
	return candidates, nil
}

func normalizeFamily(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}
