package fontdesc

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"
)

// SymbolClass is the fixed, versioned taxonomy accepted by font rule matches.
type SymbolClass string

const (
	SymbolClassEmoji      SymbolClass = "emoji"
	SymbolClassCJK        SymbolClass = "cjk"
	SymbolClassNerdFont   SymbolClass = "nerd_font"
	SymbolClassPowerline  SymbolClass = "powerline"
	SymbolClassBoxDrawing SymbolClass = "box_drawing"
	SymbolClassBraille    SymbolClass = "braille"
	SymbolClassSymbols    SymbolClass = "symbols"
)

// RuneRange is one inclusive Unicode scalar interval.
type RuneRange struct {
	First rune
	Last  rune
}

// OptionalIntRange distinguishes an absent match predicate from an authored
// inclusive range whose endpoints happen to contain zero values.
type OptionalIntRange struct {
	Min, Max int
	Present  bool
}

// RuleMatch selects requested style targets and/or complete cluster classes.
type RuleMatch struct {
	Weight  OptionalIntRange
	Styles  []Style
	Stretch OptionalIntRange
	Ranges  []RuneRange
	Class   SymbolClass
}

// Rule routes the first matching, fully-covered cluster to one inline face
// descriptor. Rules never mutate shaping features.
type Rule struct {
	Match RuleMatch
	Use   Descriptor
}

// FaceRuleID is the canonical identity of one normalized rule.
type FaceRuleID [sha256.Size]byte

func (id FaceRuleID) String() string { return fmt.Sprintf("%x", id[:]) }

// Normalize canonicalizes a rule and verifies all predicates and bounds.
func (r Rule) Normalize() (Rule, error) {
	use, err := r.Use.Normalize()
	if err != nil {
		return Rule{}, fmt.Errorf("use: %w", err)
	}
	r.Use = use
	if err := validateOptionalRange("weight", r.Match.Weight, 100, 900); err != nil {
		return Rule{}, err
	}
	if err := validateOptionalRange("stretch", r.Match.Stretch, 50, 200); err != nil {
		return Rule{}, err
	}
	if r.Match.Class != "" && !r.Match.Class.Valid() {
		return Rule{}, fmt.Errorf("invalid symbol class %q", r.Match.Class)
	}
	styles := append([]Style(nil), r.Match.Styles...)
	seenStyles := make(map[Style]struct{}, len(styles))
	for _, style := range styles {
		switch style {
		case StyleNormal, StyleItalic, StyleOblique:
		default:
			return Rule{}, fmt.Errorf("invalid match style %q", style)
		}
		if _, exists := seenStyles[style]; exists {
			return Rule{}, fmt.Errorf("duplicate match style %q", style)
		}
		seenStyles[style] = struct{}{}
	}
	sort.Slice(styles, func(i, j int) bool { return styleOrder(styles[i]) < styleOrder(styles[j]) })
	r.Match.Styles = styles

	ranges := append([]RuneRange(nil), r.Match.Ranges...)
	for _, item := range ranges {
		if !validScalar(item.First) || !validScalar(item.Last) || item.First > item.Last {
			return Rule{}, fmt.Errorf("invalid Unicode range U+%04X..U+%04X", item.First, item.Last)
		}
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].First != ranges[j].First {
			return ranges[i].First < ranges[j].First
		}
		return ranges[i].Last < ranges[j].Last
	})
	normalizedRanges := make([]RuneRange, 0, len(ranges))
	for _, item := range ranges {
		if len(normalizedRanges) == 0 || int64(item.First) > int64(normalizedRanges[len(normalizedRanges)-1].Last)+1 {
			normalizedRanges = append(normalizedRanges, item)
			continue
		}
		if item.Last > normalizedRanges[len(normalizedRanges)-1].Last {
			normalizedRanges[len(normalizedRanges)-1].Last = item.Last
		}
	}
	if len(normalizedRanges) > MaxRangesPerRule {
		return Rule{}, fmt.Errorf("range count %d exceeds %d", len(normalizedRanges), MaxRangesPerRule)
	}
	r.Match.Ranges = normalizedRanges
	if !r.Match.Weight.Present && len(r.Match.Styles) == 0 && !r.Match.Stretch.Present && len(r.Match.Ranges) == 0 && r.Match.Class == "" {
		return Rule{}, fmt.Errorf("match must contain at least one predicate")
	}
	return r, nil
}

// Validate checks a rule without mutating the caller's slices.
func (r Rule) Validate() error {
	_, err := r.Normalize()
	return err
}

// CanonicalBytes returns the bounded, order-independent encoding of one rule's
// set/range predicates and inline descriptor.
func (r Rule) CanonicalBytes() ([]byte, error) {
	normalized, err := r.Normalize()
	if err != nil {
		return nil, err
	}
	e := NewCanonicalEncoder(canonicalFormatVersion, MaxDescriptorPayloadBytes)
	addOptionalIntRange(e, 10, normalized.Match.Weight)
	e.AddUint32(20, uint32(len(normalized.Match.Styles)))
	for _, style := range normalized.Match.Styles {
		e.AddString(21, string(style))
	}
	addOptionalIntRange(e, 30, normalized.Match.Stretch)
	e.AddUint32(40, uint32(len(normalized.Match.Ranges)))
	for _, item := range normalized.Match.Ranges {
		e.AddUint32(41, uint32(item.First))
		e.AddUint32(42, uint32(item.Last))
	}
	e.AddString(50, string(normalized.Match.Class))
	if normalized.Match.Class != "" {
		e.AddString(51, FontSymbolClassDataVersion)
	}
	use, err := encodeDescriptor(normalized.Use)
	if err != nil {
		return nil, err
	}
	e.AddBytes(60, use)
	return e.Bytes()
}

// ID returns the canonical rule digest.
func (r Rule) ID() (FaceRuleID, error) {
	encoded, err := r.CanonicalBytes()
	if err != nil {
		return FaceRuleID{}, err
	}
	return FaceRuleID(sha256.Sum256(encoded)), nil
}

// Matches reports whether requested target predicates and complete-cluster
// classification predicates match. Combining marks and default ignorables
// inherit base classification; face coverage still validates them separately.
func (r Rule) Matches(cluster string, target FaceTarget) bool {
	normalized, err := r.Normalize()
	if err != nil || cluster == "" || !utf8.ValidString(cluster) {
		return false
	}
	match := normalized.Match
	if match.Weight.Present && (target.Weight < match.Weight.Min || target.Weight > match.Weight.Max) {
		return false
	}
	if match.Stretch.Present && (target.Stretch < match.Stretch.Min || target.Stretch > match.Stretch.Max) {
		return false
	}
	if len(match.Styles) != 0 {
		found := false
		for _, style := range match.Styles {
			if style == target.Style {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(match.Ranges) == 0 && match.Class == "" {
		return true
	}
	bases := 0
	for _, value := range cluster {
		if IsDefaultIgnorableRune(value) || unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Mc, value) || unicode.Is(unicode.Me, value) {
			continue
		}
		bases++
		if !match.matchesRune(value) {
			return false
		}
	}
	return bases != 0
}

func (m RuleMatch) matchesRune(value rune) bool {
	if m.Class != "" && m.Class.Contains(value) {
		return true
	}
	index := sort.Search(len(m.Ranges), func(i int) bool { return m.Ranges[i].Last >= value })
	return index < len(m.Ranges) && m.Ranges[index].First <= value
}

// Valid reports whether the class is part of the fixed public taxonomy.
func (c SymbolClass) Valid() bool {
	switch c {
	case SymbolClassEmoji, SymbolClassCJK, SymbolClassNerdFont, SymbolClassPowerline, SymbolClassBoxDrawing, SymbolClassBraille, SymbolClassSymbols:
		return true
	default:
		return false
	}
}

// Contains tests generated, versioned class membership.
func (c SymbolClass) Contains(value rune) bool {
	switch c {
	case SymbolClassEmoji:
		return generatedRangeContains(value, generatedEmojiRanges[:])
	case SymbolClassCJK:
		return generatedRangeContains(value, generatedCJKRanges[:])
	case SymbolClassNerdFont:
		return generatedRangeContains(value, generatedNerdFontRanges[:])
	case SymbolClassPowerline:
		return generatedRangeContains(value, generatedPowerlineRanges[:])
	case SymbolClassBoxDrawing:
		return generatedRangeContains(value, generatedBoxDrawingRanges[:])
	case SymbolClassBraille:
		return generatedRangeContains(value, generatedBrailleRanges[:])
	case SymbolClassSymbols:
		return generatedRangeContains(value, generatedSymbolsRanges[:])
	default:
		return false
	}
}

// IsDefaultIgnorableRune reports checked-in Unicode 15.1 policy membership.
func IsDefaultIgnorableRune(value rune) bool {
	return generatedRangeContains(value, generatedDefaultIgnorableRanges[:])
}

func addOptionalIntRange(e *CanonicalEncoder, tag uint16, value OptionalIntRange) {
	e.AddBool(tag, value.Present)
	if value.Present {
		e.AddUint16(tag+1, uint16(value.Min))
		e.AddUint16(tag+2, uint16(value.Max))
	}
}

func validateOptionalRange(name string, value OptionalIntRange, minimum, maximum int) error {
	if !value.Present {
		return nil
	}
	if value.Min < minimum || value.Max > maximum || value.Min > value.Max {
		return fmt.Errorf("%s range %d..%d is outside %d..%d", name, value.Min, value.Max, minimum, maximum)
	}
	return nil
}

func styleOrder(style Style) int {
	switch style {
	case StyleNormal:
		return 0
	case StyleItalic:
		return 1
	case StyleOblique:
		return 2
	default:
		return 3
	}
}

func validScalar(value rune) bool {
	return value >= 0 && value <= unicode.MaxRune && (value < 0xD800 || value > 0xDFFF)
}
