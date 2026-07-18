package fontdesc

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestRuleNormalizeAndIdentity(t *testing.T) {
	rule := Rule{
		Match: RuleMatch{
			Weight: OptionalIntRange{Min: 400, Max: 700, Present: true},
			Styles: []Style{StyleOblique, StyleNormal},
			Ranges: []RuneRange{{First: 0xE010, Last: 0xE020}, {First: 0xE000, Last: 0xE00F}, {First: 0xE008, Last: 0xE018}},
			Class:  SymbolClassPowerline,
		},
		Use: Descriptor{Family: " Test  Mono "},
	}
	normalized, err := rule.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if got := normalized.Match.Styles; len(got) != 2 || got[0] != StyleNormal || got[1] != StyleOblique {
		t.Fatalf("normalized styles = %v", got)
	}
	if got := normalized.Match.Ranges; len(got) != 1 || got[0] != (RuneRange{First: 0xE000, Last: 0xE020}) {
		t.Fatalf("normalized ranges = %#v", got)
	}
	if normalized.Use.Family != "Test Mono" {
		t.Fatalf("normalized family = %q", normalized.Use.Family)
	}
	first, err := rule.ID()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := rule.CanonicalBytes()
	if err != nil || !bytes.Contains(encoded, []byte(FontSymbolClassDataVersion)) {
		t.Fatalf("rule identity omits class data version: %v", err)
	}
	reordered := rule
	reordered.Match.Styles = []Style{StyleNormal, StyleOblique}
	reordered.Match.Ranges = []RuneRange{{First: 0xE000, Last: 0xE020}}
	second, err := reordered.ID()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent rules have different IDs: %s != %s", first, second)
	}
	reordered.Use.Weight = 500
	third, err := reordered.ID()
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("use descriptor mutation did not change rule identity")
	}
}

func TestRuleValidationBounds(t *testing.T) {
	base := Rule{Match: RuleMatch{Class: SymbolClassEmoji}, Use: Descriptor{Family: "F"}}
	cases := []struct {
		name string
		rule Rule
	}{
		{"empty match", Rule{Use: Descriptor{Family: "F"}}},
		{"missing use", Rule{Match: RuleMatch{Class: SymbolClassEmoji}}},
		{"invalid class", Rule{Match: RuleMatch{Class: "icons"}, Use: Descriptor{Family: "F"}}},
		{"invalid weight", Rule{Match: RuleMatch{Weight: OptionalIntRange{Min: 99, Max: 400, Present: true}}, Use: Descriptor{Family: "F"}}},
		{"reverse stretch", Rule{Match: RuleMatch{Stretch: OptionalIntRange{Min: 110, Max: 90, Present: true}}, Use: Descriptor{Family: "F"}}},
		{"duplicate style", Rule{Match: RuleMatch{Styles: []Style{StyleNormal, StyleNormal}}, Use: Descriptor{Family: "F"}}},
		{"surrogate", Rule{Match: RuleMatch{Ranges: []RuneRange{{First: 0xD800, Last: 0xD800}}}, Use: Descriptor{Family: "F"}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.rule.Validate(); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
}

func TestRuleMatchesTargetAndCompleteClusterClass(t *testing.T) {
	rule := Rule{
		Match: RuleMatch{
			Weight:  OptionalIntRange{Min: 700, Max: 900, Present: true},
			Styles:  []Style{StyleItalic},
			Stretch: OptionalIntRange{Min: 90, Max: 110, Present: true},
			Class:   SymbolClassEmoji,
		},
		Use: Descriptor{Family: "Emoji"},
	}
	target := FaceTarget{Weight: 700, Style: StyleItalic, Stretch: 100}
	if !rule.Matches("👩\u200d💻\ufe0f", target) {
		t.Fatal("emoji ZWJ cluster did not match")
	}
	if rule.Matches("😀A", target) {
		t.Fatal("mixed cluster matched complete-cluster class")
	}
	if rule.Matches(string([]byte{0xff}), target) {
		t.Fatal("invalid UTF-8 matched a font rule")
	}
	spacingMarkRule := Rule{Match: RuleMatch{Ranges: []RuneRange{{First: 0x0915, Last: 0x0915}}}, Use: Descriptor{Family: "Indic"}}
	if !spacingMarkRule.Matches("का", FaceTarget{}) {
		t.Fatal("spacing combining mark did not inherit base classification")
	}
	target.Weight = 400
	if rule.Matches("😀", target) {
		t.Fatal("weight predicate was ignored")
	}
}

func TestGeneratedSymbolClassBoundariesAndOverlaps(t *testing.T) {
	checks := []struct {
		class SymbolClass
		rune  rune
		want  bool
	}{
		{SymbolClassEmoji, '😀', true},
		{SymbolClassEmoji, '#', true},
		{SymbolClassEmoji, 0x1FC00, false},
		{SymbolClassCJK, '漢', true},
		{SymbolClassCJK, 0x20000, true},
		{SymbolClassCJK, 0x1100, true},
		{SymbolClassCJK, 0xD7A4, false},
		{SymbolClassNerdFont, 0xE0B0, true},
		{SymbolClassNerdFont, 0xE00B, false},
		{SymbolClassPowerline, 0xE0B0, true},
		{SymbolClassPowerline, 0xE0D8, false},
		{SymbolClassBoxDrawing, 0x2500, true},
		{SymbolClassBoxDrawing, 0x2580, false},
		{SymbolClassBraille, 0x28FF, true},
		{SymbolClassSymbols, 0x2192, true},
	}
	for _, check := range checks {
		if got := check.class.Contains(check.rune); got != check.want {
			t.Errorf("%s.Contains(U+%04X) = %v, want %v", check.class, check.rune, got, check.want)
		}
	}
	for _, value := range []rune{0x200C, 0x200D, 0xFE0F, 0xE0100} {
		if !IsDefaultIgnorableRune(value) {
			t.Errorf("U+%04X is not default ignorable", value)
		}
	}
}

func TestGeneratedSymbolClassesCurrent(t *testing.T) {
	command := exec.Command("go", "run", "./scripts/generate-font-symbol-classes.go", "-check")
	command.Dir = "../.."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated symbol classes are stale: %v\n%s", err, output)
	}
}
