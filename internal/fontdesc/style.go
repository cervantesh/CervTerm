package fontdesc

import (
	"fmt"
	"strings"
)

// RequestedFaceStyle is the terminal's four-state bold/italic request.
type RequestedFaceStyle uint8

const (
	RequestedFaceStyleNormal RequestedFaceStyle = iota
	RequestedFaceStyleBold
	RequestedFaceStyleItalic
	RequestedFaceStyleBoldItalic
)

// RequestedFaceStyleFromAttributes converts terminal attributes to a request.
func RequestedFaceStyleFromAttributes(bold, italic bool) RequestedFaceStyle {
	return RequestedFaceStyle(boolBit(bold) | boolBit(italic)<<1)
}

// Bold reports whether the request includes the bold attribute.
func (s RequestedFaceStyle) Bold() bool { return s <= RequestedFaceStyleBoldItalic && s&1 != 0 }

// Italic reports whether the request includes the italic attribute.
func (s RequestedFaceStyle) Italic() bool { return s <= RequestedFaceStyleBoldItalic && s&2 != 0 }

func (s RequestedFaceStyle) valid() bool { return s <= RequestedFaceStyleBoldItalic }

func boolBit(value bool) RequestedFaceStyle {
	if value {
		return 1
	}
	return 0
}

// EffectiveTarget returns the descriptor target after applying terminal
// attributes. Fixed descriptors retain their authored target. Augment descriptors
// raise bold weight to at least 700 and only turn a normal slant into italic.
func (d Descriptor) EffectiveTarget(request RequestedFaceStyle) (FaceTarget, error) {
	if !request.valid() {
		return FaceTarget{}, fmt.Errorf("invalid requested face style %d", request)
	}
	normalized, err := d.Normalize()
	if err != nil {
		return FaceTarget{}, err
	}
	target := FaceTarget{Weight: normalized.Weight, Style: normalized.Style, Stretch: normalized.Stretch}
	if normalized.AttributeMode == AttributeModeFixed {
		return target, nil
	}
	if request.Bold() && target.Weight < 700 {
		target.Weight = 700
	}
	if request.Italic() && target.Style == StyleNormal {
		target.Style = StyleItalic
	}
	return target, nil
}

// FaceMetadata is the normalized identity and ranking metadata for one face.
// Family and Subfamily are descriptive identity fields only; ranking never
// infers weight or style from their text.
type FaceMetadata struct {
	Family          string
	Subfamily       string
	Weight          int
	Style           Style
	Stretch         int
	CollectionIndex uint32
}

// Normalized applies canonical whitespace and numeric/style defaults.
func (m FaceMetadata) Normalized() FaceMetadata {
	m.Family = normalizeName(m.Family)
	m.Subfamily = normalizeName(m.Subfamily)
	if m.Weight == 0 {
		m.Weight = DefaultWeight
	}
	if m.Style == "" {
		m.Style = StyleNormal
	}
	if m.Stretch == 0 {
		m.Stretch = DefaultStretch
	}
	return m
}

// Normalize returns validated canonical face metadata.
func (m FaceMetadata) Normalize() (FaceMetadata, error) {
	m = m.Normalized()
	if err := m.Validate(); err != nil {
		return FaceMetadata{}, err
	}
	return m, nil
}

// Validate verifies face metadata after applying its defaults.
func (m FaceMetadata) Validate() error {
	m = m.Normalized()
	if m.Family == "" {
		return fmt.Errorf("face family is required")
	}
	if m.Subfamily == "" {
		return fmt.Errorf("face subfamily is required")
	}
	if m.Weight < 100 || m.Weight > 900 {
		return fmt.Errorf("face weight %d is outside 100..900", m.Weight)
	}
	switch m.Style {
	case StyleNormal, StyleItalic, StyleOblique:
	default:
		return fmt.Errorf("invalid face style %q", m.Style)
	}
	if m.Stretch < 50 || m.Stretch > 200 {
		return fmt.Errorf("face stretch %d is outside 50..200", m.Stretch)
	}
	if m.CollectionIndex >= MaxFacesPerFile {
		return fmt.Errorf("face collection index %d is outside 0..%d", m.CollectionIndex, MaxFacesPerFile-1)
	}
	return nil
}

// RankingTieBreaks supplies authored and stable source ordering information which
// is not font metadata. CanonicalSource must be a caller-defined stable source
// identity. SourceOrder only disambiguates otherwise identical canonical faces.
type RankingTieBreaks struct {
	Tier            SourceTier
	AuthoredOrder   uint32
	Synthetic       bool
	CanonicalSource string
	SourceOrder     uint32
}

// RankingTuple is ordered lexicographically by Compare. Lower values win.
// WeightDistance and StretchDistance are absolute distances. Their direction
// slots resolve equal distances: weights below 500 prefer the candidate toward
// 500, weight 500 prefers downward, and weights above 500 prefer upward;
// stretches at or below normal prefer narrower, and wider stretches prefer
// wider. Thus 400 ranks 500 before 300, while 500 ranks 400 before 600.
type RankingTuple struct {
	Tier               SourceTier
	AuthoredOrder      uint32
	StyleDistance      uint8
	WeightDistance     uint16
	WeightDirection    uint8
	StretchDistance    uint16
	StretchDirection   uint8
	SyntheticPenalty   uint8
	CanonicalSource    string
	CollectionIndex    uint32
	SourceOrder        uint32
	CandidateWeight    int
	CandidateStyle     Style
	CandidateStretch   int
	CanonicalFamily    string
	CanonicalSubfamily string
}

// Rank validates its inputs and constructs a deterministic ranking tuple.
func Rank(target FaceTarget, candidate FaceMetadata, tie RankingTieBreaks) (RankingTuple, error) {
	if err := validateFaceTarget(target); err != nil {
		return RankingTuple{}, err
	}
	if tie.Tier > SourceTierEmbedded {
		return RankingTuple{}, fmt.Errorf("invalid source tier %d", tie.Tier)
	}
	candidate, err := candidate.Normalize()
	if err != nil {
		return RankingTuple{}, err
	}
	syntheticPenalty := uint8(0)
	if tie.Synthetic {
		syntheticPenalty = 1
	}
	return RankingTuple{
		Tier:               tie.Tier,
		AuthoredOrder:      tie.AuthoredOrder,
		StyleDistance:      styleDistance(target.Style, candidate.Style),
		WeightDistance:     uint16(abs(target.Weight - candidate.Weight)),
		WeightDirection:    weightDirection(target.Weight, candidate.Weight),
		StretchDistance:    uint16(abs(target.Stretch - candidate.Stretch)),
		StretchDirection:   stretchDirection(target.Stretch, candidate.Stretch),
		SyntheticPenalty:   syntheticPenalty,
		CanonicalSource:    tie.CanonicalSource,
		CollectionIndex:    candidate.CollectionIndex,
		SourceOrder:        tie.SourceOrder,
		CandidateWeight:    candidate.Weight,
		CandidateStyle:     candidate.Style,
		CandidateStretch:   candidate.Stretch,
		CanonicalFamily:    strings.ToLower(candidate.Family),
		CanonicalSubfamily: strings.ToLower(candidate.Subfamily),
	}, nil
}

// Compare returns -1 if a ranks before b, 1 if b ranks before a, and 0 only
// when all deterministic tuple slots are equal.
func Compare(a, b RankingTuple) int {
	if c := compareUint8(uint8(a.Tier), uint8(b.Tier)); c != 0 {
		return c
	}
	if c := compareUint32(a.AuthoredOrder, b.AuthoredOrder); c != 0 {
		return c
	}
	if c := compareUint8(a.StyleDistance, b.StyleDistance); c != 0 {
		return c
	}
	if c := compareUint16(a.WeightDistance, b.WeightDistance); c != 0 {
		return c
	}
	if c := compareUint8(a.WeightDirection, b.WeightDirection); c != 0 {
		return c
	}
	if c := compareUint16(a.StretchDistance, b.StretchDistance); c != 0 {
		return c
	}
	if c := compareUint8(a.StretchDirection, b.StretchDirection); c != 0 {
		return c
	}
	if c := compareUint8(a.SyntheticPenalty, b.SyntheticPenalty); c != 0 {
		return c
	}
	if c := strings.Compare(a.CanonicalSource, b.CanonicalSource); c != 0 {
		return c
	}
	if c := compareUint32(a.CollectionIndex, b.CollectionIndex); c != 0 {
		return c
	}
	if c := compareUint32(a.SourceOrder, b.SourceOrder); c != 0 {
		return c
	}
	if a.CandidateWeight != b.CandidateWeight {
		if a.CandidateWeight < b.CandidateWeight {
			return -1
		}
		return 1
	}
	if c := strings.Compare(string(a.CandidateStyle), string(b.CandidateStyle)); c != 0 {
		return c
	}
	if a.CandidateStretch != b.CandidateStretch {
		if a.CandidateStretch < b.CandidateStretch {
			return -1
		}
		return 1
	}
	if c := strings.Compare(a.CanonicalFamily, b.CanonicalFamily); c != 0 {
		return c
	}
	if c := strings.Compare(a.CanonicalSubfamily, b.CanonicalSubfamily); c != 0 {
		return c
	}
	return 0
}

// Compare is the method form of Compare.
func (a RankingTuple) Compare(b RankingTuple) int { return Compare(a, b) }

func validateFaceTarget(target FaceTarget) error {
	if target.Weight < 100 || target.Weight > 900 {
		return fmt.Errorf("target weight %d is outside 100..900", target.Weight)
	}
	switch target.Style {
	case StyleNormal, StyleItalic, StyleOblique:
	default:
		return fmt.Errorf("invalid target style %q", target.Style)
	}
	if target.Stretch < 50 || target.Stretch > 200 {
		return fmt.Errorf("target stretch %d is outside 50..200", target.Stretch)
	}
	return nil
}

func styleDistance(target, candidate Style) uint8 {
	if target == candidate {
		return 0
	}
	if target != StyleNormal && candidate != StyleNormal {
		return 1
	}
	if target != StyleNormal && candidate == StyleNormal {
		return 2
	}
	return 3
}

func weightDirection(target, candidate int) uint8 {
	if target < 500 {
		if candidate > target {
			return 0
		}
		return 1
	}
	if target == 500 {
		if candidate < target {
			return 0
		}
		return 1
	}
	if candidate > target {
		return 0
	}
	return 1
}

func stretchDirection(target, candidate int) uint8 {
	if target <= DefaultStretch {
		if candidate < target {
			return 0
		}
		return 1
	}
	if candidate > target {
		return 0
	}
	return 1
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
func compareUint8(a, b uint8) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func compareUint16(a, b uint16) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func compareUint32(a, b uint32) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
