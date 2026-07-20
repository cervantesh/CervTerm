package fontdesc

import (
	"bytes"
	"testing"
)

func TestFeatureSetProjectionExplicitPrecedenceAndIdentity(t *testing.T) {
	set, err := NewFeatureSet(true, map[string]int{"liga": 0, "ss01": 2, "kern": 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []Feature{{Tag: "calt", Value: 1}, {Tag: "clig", Value: 1}, {Tag: "kern", Value: 1}, {Tag: "liga", Value: 0}, {Tag: "ss01", Value: 2}}
	if got := set.Entries(); len(got) != len(want) {
		t.Fatalf("entries = %#v", got)
	} else {
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("entry %d = %#v, want %#v", index, got[index], want[index])
			}
		}
	}
	if !set.EnablesRunSubstitution() || !set.EnablesSingleGlyphSubstitution() {
		t.Fatal("effective substitution features were not reported")
	}
	reordered, err := NewFeatureSet(true, map[string]int{"kern": 1, "ss01": 2, "liga": 0})
	if err != nil {
		t.Fatal(err)
	}
	if set.ID() != reordered.ID() || !bytes.Equal(set.CanonicalBytes(), reordered.CanonicalBytes()) {
		t.Fatal("map iteration order changed canonical feature identity")
	}
	mutated, err := NewFeatureSet(true, map[string]int{"liga": 1, "ss01": 2, "kern": 1})
	if err != nil {
		t.Fatal(err)
	}
	if set.ID() == mutated.ID() {
		t.Fatal("feature value mutation did not change identity")
	}
}

func TestFeatureSetLigatureCompatibilityProjection(t *testing.T) {
	on, err := NewFeatureSet(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	off, err := NewFeatureSet(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"liga", "clig", "calt"} {
		if value, ok := on.Value(tag); !ok || value != 1 {
			t.Fatalf("enabled %s = %d/%v", tag, value, ok)
		}
		if value, ok := off.Value(tag); !ok || value != 0 {
			t.Fatalf("disabled %s = %d/%v", tag, value, ok)
		}
	}
	for _, tag := range []string{"kern", "dlig"} {
		if _, ok := on.Value(tag); ok {
			t.Fatalf("compatibility projection unexpectedly changed %s", tag)
		}
	}
	kernOnly, err := NewFeatureSet(false, map[string]int{"kern": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !kernOnly.RequiresRunShaping() || kernOnly.EnablesRunSubstitution() {
		t.Fatal("kern must reach run shaping without being classified as substitution")
	}
	zeroExplicit, err := NewFeatureSet(false, map[string]int{"ss01": 0})
	if err != nil {
		t.Fatal(err)
	}
	if !zeroExplicit.RequestsFeatureCapability() || off.RequestsFeatureCapability() {
		t.Fatal("explicit non-projected tag capability detection is incorrect")
	}
	if on.ID() == off.ID() {
		t.Fatal("ligature projection did not affect identity")
	}
	if off.EnablesRunSubstitution() || off.EnablesSingleGlyphSubstitution() {
		t.Fatal("disabled projection reported substitutions")
	}
}

func TestFeatureSetValidationAndBounds(t *testing.T) {
	for _, tag := range []string{"abc", "abcde", "éabc", "a\x00bc"} {
		if _, err := NewFeatureSet(false, map[string]int{tag: 1}); err == nil {
			t.Fatalf("invalid tag %q accepted", tag)
		}
	}
	for _, value := range []int{-1, FeatureValueMaximum + 1} {
		if _, err := NewFeatureSet(false, map[string]int{"ss01": value}); err == nil {
			t.Fatalf("invalid value %d accepted", value)
		}
	}
	features := make(map[string]int)
	for index := 0; index < MaxFeatureTags-3; index++ {
		features[string([]byte{'x', byte('0' + index/100%10), byte('0' + index/10%10), byte('0' + index%10)})] = index
	}
	if _, err := NewFeatureSet(false, features); err != nil {
		t.Fatalf("boundary feature set rejected: %v", err)
	}
	features["over"] = 1
	if _, err := NewFeatureSet(false, features); err == nil {
		t.Fatal("over-limit effective feature set accepted")
	}
}
