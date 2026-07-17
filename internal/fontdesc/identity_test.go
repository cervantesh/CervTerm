package fontdesc

import "testing"

func TestCanonicalEncoderStableBoundedAndCollisionFree(t *testing.T) {
	encode := func(a, b string) [32]byte {
		e := NewCanonicalEncoder(7, 1024)
		e.AddString(10, a)
		e.AddString(11, b)
		sum, err := e.Sum()
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	if encode("a", "bc") == encode("ab", "c") {
		t.Fatal("length-prefixed fields collided")
	}
	if encode("stable", "input") != encode("stable", "input") {
		t.Fatal("same canonical input was unstable")
	}

	e := NewCanonicalEncoder(1, 24)
	e.AddString(10, "this field cannot fit")
	if _, err := e.Sum(); err == nil {
		t.Fatal("encoder accepted payload beyond its checked bound")
	}
	if _, err := NewCanonicalEncoder(1, -1).Bytes(); err == nil {
		t.Fatal("encoder accepted negative bound")
	}
}

func TestFontEnvironmentKeyStabilityAndOrderSensitivity(t *testing.T) {
	a := Descriptor{Family: "Alpha"}
	b := Descriptor{Family: "Beta", CollectionIndex: SomeCollectionIndex(0)}
	input := FontEnvironmentInput{
		Descriptors:  []Descriptor{a, b},
		Fallback:     []Descriptor{{Family: "Fallback"}},
		Rules:        [][]byte{[]byte("rule-a"), []byte("rule-b")},
		Features:     []byte("features-v1"),
		Metrics:      []byte("metrics-v1"),
		BaseSizeBits: 0x4028000000000000,
		PaneZoomBits: 0x3ff0000000000000,
		DPI:          96,
		RasterMode:   "gray",
		GammaBits:    0x3ff0000000000000,
	}
	first, err := NewFontEnvironmentKey(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFontEnvironmentKey(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first.String()) != 64 {
		t.Fatalf("environment identity is not stable: %s / %s", first, second)
	}

	caseVariant := input
	caseVariant.Descriptors = []Descriptor{{Family: " alpha "}, {Family: "BETA", CollectionIndex: SomeCollectionIndex(0)}}
	caseVariant.Fallback = []Descriptor{{Family: "FALLBACK"}}
	caseKey, err := NewFontEnvironmentKey(caseVariant)
	if err != nil {
		t.Fatal(err)
	}
	if first != caseKey {
		t.Fatal("case/whitespace-equivalent names changed canonical identity")
	}

	reordered := input
	reordered.Descriptors = []Descriptor{b, a}
	third, err := NewFontEnvironmentKey(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("ordered descriptors did not affect environment identity")
	}

	absent := FontEnvironmentInput{Descriptors: []Descriptor{{Family: "Alpha"}}}
	presentZero := FontEnvironmentInput{Descriptors: []Descriptor{{Family: "Alpha", CollectionIndex: SomeCollectionIndex(0)}}}
	absentKey, err := NewFontEnvironmentKey(absent)
	if err != nil {
		t.Fatal(err)
	}
	presentKey, err := NewFontEnvironmentKey(presentZero)
	if err != nil {
		t.Fatal(err)
	}
	if absentKey == presentKey {
		t.Fatal("absent collection index aliased explicit zero")
	}
}

func TestResolvedFaceKeySeparatesResolutionDimensions(t *testing.T) {
	environment, err := NewFontEnvironmentKey(FontEnvironmentInput{Descriptors: []Descriptor{{Family: "Face"}}})
	if err != nil {
		t.Fatal(err)
	}
	base := ResolvedFaceInput{
		Environment: environment,
		Face:        CanonicalFaceIDFromBytes([]byte("face-a")),
		Tier:        SourceTierPrimary,
		SourceIndex: 1,
		Target:      FaceTarget{Weight: 400, Style: StyleNormal, Stretch: 100},
		Synthetic:   SyntheticNone,
	}
	baseKey, err := NewResolvedFaceKey(base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*ResolvedFaceInput){
		func(v *ResolvedFaceInput) { v.Face = CanonicalFaceIDFromBytes([]byte("face-b")) },
		func(v *ResolvedFaceInput) { v.Tier = SourceTierFallback },
		func(v *ResolvedFaceInput) { v.SourceIndex++ },
		func(v *ResolvedFaceInput) { v.Target.Weight = 700 },
		func(v *ResolvedFaceInput) { v.Target.Style = StyleItalic },
		func(v *ResolvedFaceInput) { v.Target.Stretch = 90 },
		func(v *ResolvedFaceInput) { v.Synthetic = SyntheticItalic },
	}
	for i, mutate := range mutations {
		changed := base
		mutate(&changed)
		key, err := NewResolvedFaceKey(changed)
		if err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		if key == baseKey {
			t.Fatalf("mutation %d did not separate resolved key", i)
		}
	}
}
