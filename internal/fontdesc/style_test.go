package fontdesc

import (
	"testing"
)

func TestRequestedFaceStyleAttributes(t *testing.T) {
	for _, tc := range []struct {
		bold, italic bool
		want         RequestedFaceStyle
	}{
		{false, false, RequestedFaceStyleNormal},
		{true, false, RequestedFaceStyleBold},
		{false, true, RequestedFaceStyleItalic},
		{true, true, RequestedFaceStyleBoldItalic},
	} {
		got := RequestedFaceStyleFromAttributes(tc.bold, tc.italic)
		if got != tc.want || got.Bold() != tc.bold || got.Italic() != tc.italic {
			t.Fatalf("attributes (%v,%v): got %v bold=%v italic=%v", tc.bold, tc.italic, got, got.Bold(), got.Italic())
		}
	}
	invalid := RequestedFaceStyle(4)
	if invalid.Bold() || invalid.Italic() {
		t.Fatalf("invalid request exposes attributes: %d", invalid)
	}
}

func TestEffectiveTargetExhaustiveAttributeMatrix(t *testing.T) {
	for _, mode := range []AttributeMode{AttributeModeAugment, AttributeModeFixed} {
		for _, authoredWeight := range []int{400, 700, 800} {
			for _, authoredStyle := range []Style{StyleNormal, StyleItalic, StyleOblique} {
				for request := RequestedFaceStyleNormal; request <= RequestedFaceStyleBoldItalic; request++ {
					d := Descriptor{Family: " Face ", Weight: authoredWeight, Style: authoredStyle, Stretch: 125, AttributeMode: mode}
					got, err := d.EffectiveTarget(request)
					if err != nil {
						t.Fatalf("mode=%s weight=%d style=%s request=%d: %v", mode, authoredWeight, authoredStyle, request, err)
					}
					want := FaceTarget{Weight: authoredWeight, Style: authoredStyle, Stretch: 125}
					if mode == AttributeModeAugment {
						if request.Bold() && want.Weight < 700 {
							want.Weight = 700
						}
						if request.Italic() && want.Style == StyleNormal {
							want.Style = StyleItalic
						}
					}
					if got != want {
						t.Fatalf("mode=%s weight=%d style=%s request=%d: got %+v want %+v", mode, authoredWeight, authoredStyle, request, got, want)
					}
				}
			}
		}
	}
}

func TestEffectiveTargetRejectsInvalidDescriptorAndRequest(t *testing.T) {
	valid := Descriptor{Family: "Face"}
	if _, err := valid.EffectiveTarget(RequestedFaceStyle(4)); err == nil {
		t.Fatal("invalid request accepted")
	}
	if _, err := (Descriptor{Family: "Face", Weight: 99}).EffectiveTarget(RequestedFaceStyleNormal); err == nil {
		t.Fatal("invalid descriptor accepted")
	}
	got, err := valid.EffectiveTarget(RequestedFaceStyleBoldItalic)
	if err != nil {
		t.Fatal(err)
	}
	if got != (FaceTarget{Weight: 700, Style: StyleItalic, Stretch: 100}) {
		t.Fatalf("defaults not normalized before augment: %+v", got)
	}
}

func TestFaceMetadataNormalizationAndValidation(t *testing.T) {
	got, err := (FaceMetadata{Family: "  JetBrainsMono   Nerd Font ", Subfamily: " Extra Light "}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	want := FaceMetadata{Family: "JetBrainsMono Nerd Font", Subfamily: "Extra Light", Weight: 400, Style: StyleNormal, Stretch: 100}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for name, value := range map[string]FaceMetadata{
		"family":           {Subfamily: "Regular"},
		"subfamily":        {Family: "Face"},
		"weight-low":       {Family: "Face", Subfamily: "Regular", Weight: 99},
		"weight-high":      {Family: "Face", Subfamily: "Regular", Weight: 901},
		"style":            {Family: "Face", Subfamily: "Regular", Style: "roman"},
		"stretch-low":      {Family: "Face", Subfamily: "Regular", Stretch: 49},
		"stretch-high":     {Family: "Face", Subfamily: "Regular", Stretch: 201},
		"collection-index": {Family: "Face", Subfamily: "Regular", CollectionIndex: MaxFacesPerFile},
	} {
		t.Run(name, func(t *testing.T) {
			if err := value.Validate(); err == nil {
				t.Fatalf("invalid metadata accepted: %+v", value)
			}
		})
	}
}

func TestRankUsesNumericMetadataNotNames(t *testing.T) {
	target := FaceTarget{Weight: 400, Style: StyleNormal, Stretch: 100}
	candidates := []FaceMetadata{
		{Family: "Looks Thin", Subfamily: "Regular", Weight: 200, Style: StyleNormal, Stretch: 100},
		{Family: "Looks Bold Italic", Subfamily: "ExtraLight", Weight: 400, Style: StyleNormal, Stretch: 100},
		{Family: "Looks Regular", Subfamily: "Book Bold Italic", Weight: 500, Style: StyleNormal, Stretch: 100},
	}
	ranks := make([]RankingTuple, len(candidates))
	for i := range candidates {
		ranks[i] = mustRank(t, target, candidates[i])
	}
	if Compare(ranks[1], ranks[2]) >= 0 || Compare(ranks[2], ranks[0]) >= 0 {
		t.Fatalf("numeric ranking ignored: exact=%+v near=%+v far=%+v", ranks[1], ranks[2], ranks[0])
	}
}

func TestRankTierAndAuthoredOrderPrecedeFaceMetrics(t *testing.T) {
	target := FaceTarget{Weight: 400, Style: StyleItalic, Stretch: 100}
	ruleTier := checksRank(t, target, FaceMetadata{Family: "F", Subfamily: "Wrong", Weight: 900, Style: StyleNormal, Stretch: 200}, RankingTieBreaks{Tier: SourceTierRule, AuthoredOrder: 99})
	primaryTier := checksRank(t, target, FaceMetadata{Family: "F", Subfamily: "Exact", Weight: 400, Style: StyleItalic, Stretch: 100}, RankingTieBreaks{Tier: SourceTierPrimary})
	if Compare(ruleTier, primaryTier) >= 0 {
		t.Fatal("source tier did not precede authored order and style distance")
	}

	earlier := checksRank(t, target, FaceMetadata{Family: "F", Subfamily: "Earlier", Weight: 900, Style: StyleItalic, Stretch: 200}, RankingTieBreaks{Tier: SourceTierPrimary, AuthoredOrder: 3})
	laterCloser := checksRank(t, target, FaceMetadata{Family: "F", Subfamily: "Later", Weight: 400, Style: StyleItalic, Stretch: 100}, RankingTieBreaks{Tier: SourceTierPrimary, AuthoredOrder: 4})
	if Compare(earlier, laterCloser) >= 0 {
		t.Fatal("earlier authored candidate did not precede a later numerically closer candidate")
	}
}

func TestRankStyleDistanceIsAsymmetric(t *testing.T) {
	cases := []struct {
		name      string
		target    Style
		candidate Style
		want      uint8
	}{
		{"exact", StyleItalic, StyleItalic, 0},
		{"italic-oblique", StyleItalic, StyleOblique, 1},
		{"oblique-italic", StyleOblique, StyleItalic, 1},
		{"italic-synthetic-normal", StyleItalic, StyleNormal, 2},
		{"oblique-synthetic-normal", StyleOblique, StyleNormal, 2},
		{"normal-incompatible-italic", StyleNormal, StyleItalic, 3},
		{"normal-incompatible-oblique", StyleNormal, StyleOblique, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rank := checksRank(t, FaceTarget{Weight: 400, Style: tc.target, Stretch: 100}, FaceMetadata{Family: "F", Subfamily: "Candidate", Weight: 400, Style: tc.candidate, Stretch: 100}, RankingTieBreaks{Synthetic: tc.candidate == StyleNormal && tc.target != StyleNormal})
			if rank.StyleDistance != tc.want {
				t.Fatalf("style distance = %d, want %d", rank.StyleDistance, tc.want)
			}
		})
	}

	target := FaceTarget{Weight: 400, Style: StyleItalic, Stretch: 100}
	oblique := mustRank(t, target, FaceMetadata{Family: "F", Subfamily: "Oblique", Weight: 400, Style: StyleOblique, Stretch: 100})
	normal := checksRank(t, target, FaceMetadata{Family: "F", Subfamily: "Normal", Weight: 400, Style: StyleNormal, Stretch: 100}, RankingTieBreaks{Synthetic: true})
	if Compare(oblique, normal) >= 0 {
		t.Fatal("italic-to-oblique distance should beat synthetic normal")
	}
}

func TestRankWeightStretchAndSyntheticPrecedence(t *testing.T) {
	for _, tc := range []struct{ target, preferred, other int }{
		{400, 500, 300},
		{500, 400, 600},
		{300, 400, 200},
		{600, 700, 500},
	} {
		target := FaceTarget{Weight: tc.target, Style: StyleNormal, Stretch: 100}
		preferred := mustRank(t, target, FaceMetadata{Family: "F", Subfamily: "Preferred", Weight: tc.preferred, Style: StyleNormal, Stretch: 100})
		other := mustRank(t, target, FaceMetadata{Family: "F", Subfamily: "Other", Weight: tc.other, Style: StyleNormal, Stretch: 100})
		if Compare(preferred, other) >= 0 {
			t.Fatalf("weight %d: %d should precede %d", tc.target, tc.preferred, tc.other)
		}
	}

	for _, tc := range []struct{ target, preferred, other int }{{100, 90, 110}, {120, 130, 110}} {
		target := FaceTarget{Weight: 400, Style: StyleNormal, Stretch: tc.target}
		preferred := mustRank(t, target, FaceMetadata{Family: "F", Subfamily: "Preferred", Weight: 400, Style: StyleNormal, Stretch: tc.preferred})
		other := mustRank(t, target, FaceMetadata{Family: "F", Subfamily: "Other", Weight: 400, Style: StyleNormal, Stretch: tc.other})
		if Compare(preferred, other) >= 0 {
			t.Fatalf("stretch %d: %d should precede %d", tc.target, tc.preferred, tc.other)
		}
	}

	target := FaceTarget{Weight: 400, Style: StyleItalic, Stretch: 100}
	real := checksRank(t, target, FaceMetadata{Family: "Z", Subfamily: "Real", Weight: 400, Style: StyleItalic, Stretch: 100}, RankingTieBreaks{CanonicalSource: "z-real"})
	synthetic := checksRank(t, target, FaceMetadata{Family: "A", Subfamily: "Synthetic", Weight: 400, Style: StyleItalic, Stretch: 100}, RankingTieBreaks{Synthetic: true, CanonicalSource: "a-synthetic"})
	if Compare(real, synthetic) >= 0 {
		t.Fatal("real face did not beat synthetic face after equal metric distances")
	}
}

func TestRankDeterministicCanonicalIdentity(t *testing.T) {
	target := FaceTarget{Weight: 400, Style: StyleNormal, Stretch: 100}
	face0 := FaceMetadata{Family: "Z", Subfamily: "Regular", Weight: 400, Style: StyleNormal, Stretch: 100}
	face1 := face0
	face1.CollectionIndex = 1

	sourceA := checksRank(t, target, face1, RankingTieBreaks{CanonicalSource: "a", SourceOrder: 99})
	sourceB := checksRank(t, target, face0, RankingTieBreaks{CanonicalSource: "b", SourceOrder: 0})
	if Compare(sourceA, sourceB) >= 0 {
		t.Fatal("canonical source identity did not precede source order")
	}
	collection0 := checksRank(t, target, face0, RankingTieBreaks{CanonicalSource: "same", SourceOrder: 99})
	collection1 := checksRank(t, target, face1, RankingTieBreaks{CanonicalSource: "same", SourceOrder: 0})
	if Compare(collection0, collection1) >= 0 {
		t.Fatal("collection index did not precede source order")
	}
	order0 := checksRank(t, target, face0, RankingTieBreaks{CanonicalSource: "same", SourceOrder: 0})
	order1 := checksRank(t, target, face0, RankingTieBreaks{CanonicalSource: "same", SourceOrder: 1})
	if Compare(order0, order1) >= 0 {
		t.Fatal("source order did not deterministically disambiguate the same canonical face")
	}

	nameA := checksRank(t, target, FaceMetadata{Family: "A", Subfamily: "Z", Weight: 400, Style: StyleNormal, Stretch: 100}, RankingTieBreaks{CanonicalSource: "same"})
	nameZ := checksRank(t, target, FaceMetadata{Family: "Z", Subfamily: "A", Weight: 400, Style: StyleNormal, Stretch: 100}, RankingTieBreaks{CanonicalSource: "same"})
	if Compare(nameA, nameZ) >= 0 {
		t.Fatal("canonical metadata names did not provide the final deterministic tie")
	}
	if Compare(order0, order0) != 0 {
		t.Fatal("identical tuple did not compare equal")
	}
}

func TestRankRejectsInvalidInputs(t *testing.T) {
	validTarget := FaceTarget{Weight: 400, Style: StyleNormal, Stretch: 100}
	validFace := FaceMetadata{Family: "F", Subfamily: "Regular"}
	for _, target := range []FaceTarget{
		{Weight: 99, Style: StyleNormal, Stretch: 100},
		{Weight: 400, Style: "roman", Stretch: 100},
		{Weight: 400, Style: StyleNormal, Stretch: 201},
	} {
		if _, err := Rank(target, validFace, RankingTieBreaks{}); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
	if _, err := Rank(validTarget, FaceMetadata{Family: "F", Subfamily: "Regular", Weight: 99}, RankingTieBreaks{}); err == nil {
		t.Fatal("invalid candidate accepted")
	}
	if _, err := Rank(validTarget, validFace, RankingTieBreaks{Tier: SourceTierEmbedded + 1}); err == nil {
		t.Fatal("invalid source tier accepted")
	}
}

func mustRank(t *testing.T, target FaceTarget, metadata FaceMetadata) RankingTuple {
	t.Helper()
	return checksRank(t, target, metadata, RankingTieBreaks{})
}

func checksRank(t *testing.T, target FaceTarget, metadata FaceMetadata, tie RankingTieBreaks) RankingTuple {
	t.Helper()
	rank, err := Rank(target, metadata, tie)
	if err != nil {
		t.Fatal(err)
	}
	return rank
}
