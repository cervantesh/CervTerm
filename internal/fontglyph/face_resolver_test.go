package fontglyph

import (
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/fontdesc"
)

func TestResolveFaceCandidatesUsesNumericMetadataNotNames(t *testing.T) {
	extraLightNamed400 := resolverTestFace("test:z", 0, "ExtraLight", 400, fontdesc.StyleNormal, 100)
	regularNamed200 := resolverTestFace("test:a", 1, "Regular", 200, fontdesc.StyleNormal, 100)
	descriptor := fontdesc.Descriptor{Family: " Example   Mono "}
	target := fontdesc.FaceTarget{Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}

	first := resolveTestCandidates(t, []faceInfo{regularNamed200, extraLightNamed400}, descriptor, target, fontdesc.SourceTierPrimary, 0)
	second := resolveTestCandidates(t, []faceInfo{extraLightNamed400, regularNamed200}, descriptor, target, fontdesc.SourceTierPrimary, 0)
	if first[0].metadata.Subfamily != "ExtraLight" || first[0].metadata.Weight != 400 {
		t.Fatalf("numeric ranking winner = %+v", first[0])
	}
	if got, want := candidateSources(first), candidateSources(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("reversed discovery order changed ranking: %v != %v", got, want)
	}
}

func TestResolveFaceCandidatesCollectionSelectorsAreExact(t *testing.T) {
	faces := []faceInfo{
		resolverTestFace("test:collection", 0, "Regular", 400, fontdesc.StyleNormal, 100),
		resolverTestFace("test:collection", 1, "Display Face", 700, fontdesc.StyleItalic, 125),
	}
	target := fontdesc.FaceTarget{Weight: 400, Style: fontdesc.StyleNormal, Stretch: 100}

	byIndex := resolveTestCandidates(t, faces, fontdesc.Descriptor{Family: "example mono", CollectionIndex: fontdesc.SomeCollectionIndex(1)}, target, fontdesc.SourceTierPrimary, 0)
	if len(byIndex) != 1 || byIndex[0].index != 1 || byIndex[0].rank.CollectionIndex != 1 {
		t.Fatalf("collection_index candidates = %+v", byIndex)
	}
	byFace := resolveTestCandidates(t, faces, fontdesc.Descriptor{Family: "example mono", CollectionFace: "  display   FACE "}, target, fontdesc.SourceTierPrimary, 0)
	if len(byFace) != 1 || byFace[0].index != 1 {
		t.Fatalf("collection_face candidates = %+v", byFace)
	}

	_, err := resolveFaceCandidates(&FontIndex{families: map[string][]faceInfo{"example mono": faces}}, fontdesc.Descriptor{Family: "example mono", CollectionIndex: fontdesc.SomeCollectionIndex(2)}, target, fontdesc.SourceTierPrimary, 0)
	if err == nil || !strings.Contains(err.Error(), "no face at collection_index 2") {
		t.Fatalf("missing selector error = %v", err)
	}
}

func TestResolveFaceCandidatesTierAndAuthoredOrderPrecedeCloseness(t *testing.T) {
	target := fontdesc.FaceTarget{Weight: 400, Style: fontdesc.StyleItalic, Stretch: 100}
	descriptor := fontdesc.Descriptor{Family: "example mono"}
	far := []faceInfo{resolverTestFace("test:far", 0, "Far", 900, fontdesc.StyleNormal, 200)}
	close := []faceInfo{resolverTestFace("test:close", 0, "Close", 400, fontdesc.StyleItalic, 100)}

	rule := resolveTestCandidates(t, far, descriptor, target, fontdesc.SourceTierRule, 99)[0]
	primary := resolveTestCandidates(t, close, descriptor, target, fontdesc.SourceTierPrimary, 0)[0]
	if fontdesc.Compare(rule.rank, primary.rank) >= 0 {
		t.Fatal("resolver rank did not put tier before metric closeness")
	}
	earlier := resolveTestCandidates(t, far, descriptor, target, fontdesc.SourceTierPrimary, 3)[0]
	laterCloser := resolveTestCandidates(t, close, descriptor, target, fontdesc.SourceTierPrimary, 4)[0]
	if fontdesc.Compare(earlier.rank, laterCloser.rank) >= 0 {
		t.Fatal("resolver rank did not put authored order before metric closeness")
	}
	if earlier.path != "test:far" || earlier.rank.CanonicalSource != "test:far" || earlier.rank.CollectionIndex != 0 {
		t.Fatalf("candidate canonical identity groundwork = %+v", earlier)
	}
}
func TestResolvePrimaryFacePlanFourStyleMatrix(t *testing.T) {
	descriptor := fontdesc.Descriptor{Family: "Matrix Mono"}
	faces := []faceInfo{
		resolverNamedTestFace("Matrix Mono", "test:normal", 0, "Regular", 400, fontdesc.StyleNormal, 100),
		resolverNamedTestFace("Matrix Mono", "test:bold", 0, "Bold", 700, fontdesc.StyleNormal, 100),
		resolverNamedTestFace("Matrix Mono", "test:italic", 0, "Italic", 400, fontdesc.StyleItalic, 100),
		resolverNamedTestFace("Matrix Mono", "test:bold-italic", 0, "Bold Italic", 700, fontdesc.StyleItalic, 100),
	}
	tests := []struct {
		name    string
		request fontdesc.RequestedFaceStyle
		want    map[string]fontdesc.SyntheticMode
	}{
		{name: "normal", request: fontdesc.RequestedFaceStyleNormal, want: map[string]fontdesc.SyntheticMode{
			"test:normal": fontdesc.SyntheticNone, "test:bold": fontdesc.SyntheticNone,
		}},
		{name: "bold", request: fontdesc.RequestedFaceStyleBold, want: map[string]fontdesc.SyntheticMode{
			"test:normal": fontdesc.SyntheticBold, "test:bold": fontdesc.SyntheticNone,
		}},
		{name: "italic", request: fontdesc.RequestedFaceStyleItalic, want: map[string]fontdesc.SyntheticMode{
			"test:normal": fontdesc.SyntheticItalic, "test:bold": fontdesc.SyntheticItalic,
			"test:italic": fontdesc.SyntheticNone, "test:bold-italic": fontdesc.SyntheticNone,
		}},
		{name: "bold-italic", request: fontdesc.RequestedFaceStyleBoldItalic, want: map[string]fontdesc.SyntheticMode{
			"test:normal": fontdesc.SyntheticBold | fontdesc.SyntheticItalic, "test:bold": fontdesc.SyntheticItalic,
			"test:italic": fontdesc.SyntheticBold, "test:bold-italic": fontdesc.SyntheticNone,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := resolverTestEnvironment(t, []fontdesc.Descriptor{descriptor})
			plans, err := resolvePrimaryFacePlan(resolverTestIndex(faces), environment, []fontdesc.Descriptor{descriptor}, test.request)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]fontdesc.SyntheticMode, len(plans))
			for _, plan := range plans {
				got[plan.selected.path] = plan.synthetic
				if plan.selected.rank.SyntheticPenalty != boolPenalty(plan.synthetic != fontdesc.SyntheticNone) {
					t.Fatalf("synthetic rank slot for %q = %d", plan.selected.path, plan.selected.rank.SyntheticPenalty)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("synthetic matrix = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolvePrimaryFacePlanAuthoredPrecedenceAndMissingContinuation(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Missing Mono"}, {Family: "Earlier Mono"}, {Family: "Later Mono"}}
	index := &FontIndex{families: map[string][]faceInfo{
		"earlier mono": {resolverNamedTestFace("Earlier Mono", "test:earlier", 0, "Regular", 400, fontdesc.StyleNormal, 100)},
		"later mono":   {resolverNamedTestFace("Later Mono", "test:later-exact", 0, "Italic", 400, fontdesc.StyleItalic, 100)},
	}}
	plans, err := resolvePrimaryFacePlan(index, resolverTestEnvironment(t, descriptors), descriptors, fontdesc.RequestedFaceStyleItalic)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].authoredIndex != 1 || plans[0].selected.path != "test:earlier" || plans[1].authoredIndex != 2 {
		t.Fatalf("authored plans = %+v", plans)
	}
	if plans[0].synthetic != fontdesc.SyntheticItalic || plans[1].synthetic != fontdesc.SyntheticNone {
		t.Fatalf("authored synthetic modes = %d, %d", plans[0].synthetic, plans[1].synthetic)
	}
}

func TestResolvePrimaryFacePlanSelectorsAndCollectionIdentity(t *testing.T) {
	faces := []faceInfo{
		resolverNamedTestFace("Collection Mono", "test:collection.ttc", 0, "Regular", 400, fontdesc.StyleNormal, 100),
		resolverNamedTestFace("Collection Mono", "test:collection.ttc", 1, "Display", 400, fontdesc.StyleNormal, 100),
	}
	index := resolverTestIndex(faces)
	descriptors := []fontdesc.Descriptor{
		{Family: "Collection Mono", CollectionIndex: fontdesc.SomeCollectionIndex(0)},
		{Family: "Collection Mono", CollectionFace: "display"},
	}
	plans, err := resolvePrimaryFacePlan(index, resolverTestEnvironment(t, descriptors), descriptors, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].selected.index != 0 || plans[1].selected.index != 1 {
		t.Fatalf("selector plans = %+v", plans)
	}
	if plans[0].canonicalFaceID == plans[1].canonicalFaceID {
		t.Fatal("TTC collection indices shared a canonical face ID")
	}
	if plans[0].resolvedKey == plans[1].resolvedKey {
		t.Fatal("TTC collection indices shared a resolved key")
	}
}

func TestResolvePrimaryFacePlanFixedDescriptorIgnoresRequestedSynthesis(t *testing.T) {
	descriptor := fontdesc.Descriptor{Family: "Fixed Mono", AttributeMode: fontdesc.AttributeModeFixed}
	face := resolverNamedTestFace("Fixed Mono", "test:fixed", 0, "Regular", 400, fontdesc.StyleNormal, 100)
	plans, err := resolvePrimaryFacePlan(resolverTestIndex([]faceInfo{face}), resolverTestEnvironment(t, []fontdesc.Descriptor{descriptor}), []fontdesc.Descriptor{descriptor}, fontdesc.RequestedFaceStyleBoldItalic)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].synthetic != fontdesc.SyntheticNone || plans[0].target.Weight != 400 || plans[0].target.Style != fontdesc.StyleNormal {
		t.Fatalf("fixed plan = %+v", plans)
	}
}

func TestResolvePrimaryFacePlanRejectsInvalidInputsAndAggregatesFailures(t *testing.T) {
	descriptors := []fontdesc.Descriptor{{Family: "Missing One"}, {Family: "Missing Two", CollectionIndex: fontdesc.SomeCollectionIndex(2)}}
	environment := resolverTestEnvironment(t, descriptors)
	if _, err := resolvePrimaryFacePlan(&FontIndex{families: map[string][]faceInfo{}}, fontdesc.FontEnvironmentKey{}, descriptors, fontdesc.RequestedFaceStyleNormal); err == nil || !strings.Contains(err.Error(), "zero font environment key") {
		t.Fatalf("zero environment error = %v", err)
	}
	if _, err := resolvePrimaryFacePlan(&FontIndex{families: map[string][]faceInfo{}}, environment, descriptors, fontdesc.RequestedFaceStyle(99)); err == nil || !strings.Contains(err.Error(), "invalid requested face style 99") {
		t.Fatalf("invalid request error = %v", err)
	}
	_, err := resolvePrimaryFacePlan(&FontIndex{families: map[string][]faceInfo{}}, environment, descriptors, fontdesc.RequestedFaceStyleNormal)
	if err == nil {
		t.Fatal("missing descriptors unexpectedly resolved")
	}
	for _, fragment := range []string{"no load attempts", "descriptor 0 family \"Missing One\"", "descriptor 1 family \"Missing Two\""} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("aggregate error %q missing %q", err, fragment)
		}
	}
}

func TestResolvePrimaryFacePlanDeterministicForReversedDiscovery(t *testing.T) {
	descriptor := fontdesc.Descriptor{Family: "Deterministic Mono"}
	faces := []faceInfo{
		resolverNamedTestFace("Deterministic Mono", "test:z", 0, "Regular Z", 400, fontdesc.StyleNormal, 100),
		resolverNamedTestFace("Deterministic Mono", "test:a", 0, "Regular A", 400, fontdesc.StyleNormal, 100),
	}
	environment := resolverTestEnvironment(t, []fontdesc.Descriptor{descriptor})
	first, err := resolvePrimaryFacePlan(resolverTestIndex(faces), environment, []fontdesc.Descriptor{descriptor}, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolvePrimaryFacePlan(resolverTestIndex([]faceInfo{faces[1], faces[0]}), environment, []fontdesc.Descriptor{descriptor}, fontdesc.RequestedFaceStyleNormal)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolvedPlanSignatures(first), resolvedPlanSignatures(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("reversed discovery changed plans: %v != %v", got, want)
	}
}

func TestResolveEmbeddedFallbackPlanIsStableAndLastTier(t *testing.T) {
	descriptor := fontdesc.Descriptor{Family: "Primary Mono"}
	environment := resolverTestEnvironment(t, []fontdesc.Descriptor{descriptor})
	plan, err := resolveEmbeddedFallbackPlan(environment, fontdesc.RequestedFaceStyleBoldItalic)
	if err != nil {
		t.Fatal(err)
	}
	if plan.selected.path != "embedded:gomono" || plan.selected.index != 0 || plan.tier != fontdesc.SourceTierEmbedded {
		t.Fatalf("embedded plan = %+v", plan)
	}
	if plan.synthetic != fontdesc.SyntheticBold|fontdesc.SyntheticItalic {
		t.Fatalf("embedded synthetic mode = %d", plan.synthetic)
	}
	wantID := fontdesc.CanonicalFaceIDFromBytes([]byte("embedded:gomono#0"))
	if plan.canonicalFaceID != wantID {
		t.Fatalf("embedded canonical face ID = %s, want %s", plan.canonicalFaceID, wantID)
	}
}

func boolPenalty(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func resolverTestEnvironment(t *testing.T, descriptors []fontdesc.Descriptor) fontdesc.FontEnvironmentKey {
	t.Helper()
	environment, err := fontdesc.NewFontEnvironmentKey(fontdesc.FontEnvironmentInput{Descriptors: descriptors})
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func resolverTestIndex(faces []faceInfo) *FontIndex {
	families := make(map[string][]faceInfo)
	for _, face := range faces {
		families[normalizeFamily(face.metadata.Family)] = append(families[normalizeFamily(face.metadata.Family)], face)
	}
	return &FontIndex{families: families}
}

func resolvedPlanSignatures(plans []resolvedFacePlan) []string {
	signatures := make([]string, len(plans))
	for i, plan := range plans {
		signatures[i] = plan.selected.path + "#" + plan.canonicalFaceID.String() + "#" + plan.resolvedKey.String()
	}
	return signatures
}

func resolverTestFace(path string, index int, subfamily string, weight int, style fontdesc.Style, stretch int) faceInfo {
	return resolverNamedTestFace("Example Mono", path, index, subfamily, weight, style, stretch)
}

func resolverNamedTestFace(family, path string, index int, subfamily string, weight int, style fontdesc.Style, stretch int) faceInfo {
	metadata := fontdesc.FaceMetadata{
		Family: family, Subfamily: subfamily, Weight: weight, Style: style,
		Stretch: stretch, CollectionIndex: uint32(index),
	}.Normalized()
	return faceInfo{path: path, index: index, family: metadata.Family, subfamily: metadata.Subfamily, metadata: metadata}
}

func resolveTestCandidates(t *testing.T, faces []faceInfo, descriptor fontdesc.Descriptor, target fontdesc.FaceTarget, tier fontdesc.SourceTier, authoredOrder uint32) []faceCandidate {
	t.Helper()
	index := &FontIndex{families: map[string][]faceInfo{"example mono": faces}}
	candidates, err := resolveFaceCandidates(index, descriptor, target, tier, authoredOrder)
	if err != nil {
		t.Fatal(err)
	}
	return candidates
}

func candidateSources(candidates []faceCandidate) []string {
	sources := make([]string, len(candidates))
	for i := range candidates {
		sources[i] = candidates[i].path
	}
	return sources
}
