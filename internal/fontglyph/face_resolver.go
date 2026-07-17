package fontglyph

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/sfnt"
)

// faceCandidate is a ranked concrete face. path and index provide the stable
// source material needed to construct a CanonicalFaceID when loading lands.
type faceCandidate struct {
	path     string
	index    int
	metadata fontdesc.FaceMetadata
	rank     fontdesc.RankingTuple
}

// resolvedFacePlan is one pure, ordered load attempt. It contains all identity
// inputs needed by a later loader without performing font I/O here.
type resolvedFacePlan struct {
	descriptor      fontdesc.Descriptor
	target          fontdesc.FaceTarget
	selected        faceCandidate
	tier            fontdesc.SourceTier
	authoredIndex   uint32
	synthetic       fontdesc.SyntheticMode
	canonicalFaceID fontdesc.CanonicalFaceID
	resolvedKey     fontdesc.ResolvedFaceKey
}

// resolvePrimaryFacePlan resolves every usable primary descriptor into ordered
// load attempts. Missing families and selectors do not prevent later authored
// descriptors from contributing attempts.
func resolvePrimaryFacePlan(index *FontIndex, environment fontdesc.FontEnvironmentKey, descriptors []fontdesc.Descriptor, request fontdesc.RequestedFaceStyle) ([]resolvedFacePlan, error) {
	if environment == (fontdesc.FontEnvironmentKey{}) {
		return nil, fmt.Errorf("resolve primary face plan: zero font environment key")
	}
	if request > fontdesc.RequestedFaceStyleBoldItalic {
		return nil, fmt.Errorf("resolve primary face plan: invalid requested face style %d", request)
	}

	plans := make([]resolvedFacePlan, 0)
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
		candidates, err := resolveFaceCandidates(index, descriptor, target, fontdesc.SourceTierPrimary, uint32(authoredIndex))
		if err != nil {
			failures = append(failures, fmt.Errorf("descriptor %d family %q: %w", authoredIndex, descriptor.Family, err))
			continue
		}
		for _, candidate := range candidates {
			synthetic, compatible := classifySyntheticFallback(target, candidate.metadata)
			if !compatible {
				continue
			}
			candidate.rank, err = fontdesc.Rank(target, candidate.metadata, fontdesc.RankingTieBreaks{
				Tier:            fontdesc.SourceTierPrimary,
				AuthoredOrder:   uint32(authoredIndex),
				Synthetic:       synthetic != fontdesc.SyntheticNone,
				CanonicalSource: candidate.path,
			})
			if err != nil {
				failures = append(failures, fmt.Errorf("descriptor %d family %q candidate %q index %d: %w", authoredIndex, descriptor.Family, candidate.path, candidate.index, err))
				continue
			}
			plan, err := newResolvedFacePlan(environment, descriptor, target, candidate, fontdesc.SourceTierPrimary, uint32(authoredIndex), synthetic)
			if err != nil {
				failures = append(failures, fmt.Errorf("descriptor %d family %q candidate %q index %d: %w", authoredIndex, descriptor.Family, candidate.path, candidate.index, err))
				continue
			}
			plans = append(plans, plan)
		}
	}
	if len(plans) == 0 {
		if len(failures) == 0 {
			failures = append(failures, fmt.Errorf("no descriptors produced compatible candidates"))
		}
		return nil, fmt.Errorf("resolve primary face plan: no load attempts: %w", errors.Join(failures...))
	}
	sort.Slice(plans, func(i, j int) bool {
		return fontdesc.Compare(plans[i].selected.rank, plans[j].selected.rank) < 0
	})
	return plans, nil
}

// classifySyntheticFallback maps a concrete face onto the effective target.
// A weight of 600 is the documented real-bold threshold: lighter faces need
// synthetic bold for targets of 700 or above. Italic and oblique are mutually
// real-compatible; a normal face can provide either through synthetic italic.
func classifySyntheticFallback(target fontdesc.FaceTarget, metadata fontdesc.FaceMetadata) (fontdesc.SyntheticMode, bool) {
	metadata = metadata.Normalized()
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

func newResolvedFacePlan(environment fontdesc.FontEnvironmentKey, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, candidate faceCandidate, tier fontdesc.SourceTier, authoredIndex uint32, synthetic fontdesc.SyntheticMode) (resolvedFacePlan, error) {
	canonicalFaceID := fontdesc.CanonicalFaceIDFromBytes([]byte(fmt.Sprintf("%s#%d", candidate.path, candidate.index)))
	resolvedKey, err := fontdesc.NewResolvedFaceKey(fontdesc.ResolvedFaceInput{
		Environment: environment,
		Face:        canonicalFaceID,
		Tier:        tier,
		SourceIndex: authoredIndex,
		Target:      target,
		Synthetic:   synthetic,
	})
	if err != nil {
		return resolvedFacePlan{}, fmt.Errorf("build resolved face key: %w", err)
	}
	return resolvedFacePlan{
		descriptor: descriptor, target: target, selected: candidate, tier: tier,
		authoredIndex: authoredIndex, synthetic: synthetic, canonicalFaceID: canonicalFaceID, resolvedKey: resolvedKey,
	}, nil
}

// resolveEmbeddedFallbackPlan returns the final fallback separately so callers
// can append it only after exhausting all primary attempts. Its concrete stable
// source identity is embedded:gomono#0.
func resolveEmbeddedFallbackPlan(environment fontdesc.FontEnvironmentKey, request fontdesc.RequestedFaceStyle) (resolvedFacePlan, error) {
	if environment == (fontdesc.FontEnvironmentKey{}) {
		return resolvedFacePlan{}, fmt.Errorf("resolve embedded fallback plan: zero font environment key")
	}
	descriptor := fontdesc.Descriptor{Family: "Go Mono"}.Normalized()
	target, err := descriptor.EffectiveTarget(request)
	if err != nil {
		return resolvedFacePlan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	metadata := fontdesc.FaceMetadata{
		Family: "Go Mono", Subfamily: "Regular", Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100, CollectionIndex: 0,
	}.Normalized()
	synthetic, _ := classifySyntheticFallback(target, metadata)
	rank, err := fontdesc.Rank(target, metadata, fontdesc.RankingTieBreaks{
		Tier: fontdesc.SourceTierEmbedded, Synthetic: synthetic != fontdesc.SyntheticNone, CanonicalSource: "embedded:gomono",
	})
	if err != nil {
		return resolvedFacePlan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	candidate := faceCandidate{path: "embedded:gomono", index: 0, metadata: metadata, rank: rank}
	plan, err := newResolvedFacePlan(environment, descriptor, target, candidate, fontdesc.SourceTierEmbedded, 0, synthetic)
	if err != nil {
		return resolvedFacePlan{}, fmt.Errorf("resolve embedded fallback plan: %w", err)
	}
	return plan, nil
}

// resolveFaceCandidates performs deterministic selection over discovery data.
// It deliberately performs no font I/O or loading.
func resolveFaceCandidates(index *FontIndex, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, tier fontdesc.SourceTier, authoredOrder uint32) ([]faceCandidate, error) {
	descriptor, err := descriptor.Normalize()
	if err != nil {
		return nil, fmt.Errorf("normalize font descriptor: %w", err)
	}
	if index == nil {
		return nil, fmt.Errorf("resolve font family %q: nil font index", descriptor.Family)
	}
	faces := index.families[normalizeFamily(descriptor.Family)]
	if len(faces) == 0 {
		return nil, fmt.Errorf("resolve font family %q: no discovered faces", descriptor.Family)
	}

	candidates := make([]faceCandidate, 0, len(faces))
	for _, face := range faces {
		metadata := face.metadata.Normalized()
		if descriptor.CollectionIndex.Present && (face.index < 0 || uint32(face.index) != descriptor.CollectionIndex.Value) {
			continue
		}
		if descriptor.CollectionFace != "" && normalizeFamily(metadata.Subfamily) != normalizeFamily(descriptor.CollectionFace) {
			continue
		}
		canonical := canonicalFontCacheSource(face.path)
		rank, err := fontdesc.Rank(target, metadata, fontdesc.RankingTieBreaks{
			Tier: tier, AuthoredOrder: authoredOrder, CanonicalSource: canonical,
		})
		if err != nil {
			return nil, fmt.Errorf("rank font face %q index %d: %w", canonical, face.index, err)
		}
		candidates = append(candidates, faceCandidate{path: canonical, index: face.index, metadata: metadata, rank: rank})
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
	sort.Slice(candidates, func(i, j int) bool { return fontdesc.Compare(candidates[i].rank, candidates[j].rank) < 0 })
	return candidates, nil
}

func normalizeFamily(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func classifySubfamily(value string) (bold, italic bool) {
	normalized := normalizeFamily(value)
	return strings.Contains(normalized, "bold"), strings.Contains(normalized, "italic") || strings.Contains(normalized, "oblique")
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc":
		return true
	default:
		return false
	}
}

func fontName(font *sfnt.Font, preferred, fallback sfnt.NameID) string {
	var buffer sfnt.Buffer
	if name, err := font.Name(&buffer, preferred); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if name, err := font.Name(&buffer, fallback); err == nil {
		return strings.TrimSpace(name)
	}
	return ""
}

func selectTopKPaths(paths []string, limit int) []string {
	selector := newTopKPathSelector(limit)
	for _, path := range paths {
		selector.add(path)
	}
	return selector.sorted()
}

func fontFaces(path string) []faceInfo {
	faces, _, _, _ := fontFacesBounded(path, fontdesc.MaxFacesPerFile)
	return faces
}
