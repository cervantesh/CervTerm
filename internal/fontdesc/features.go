package fontdesc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

const (
	MaxFeatureTags      = 64
	featureEncodingV1   = 1
	FeatureValueMaximum = 65535
)

// FeatureSetID identifies the complete effective shaping feature projection.
type FeatureSetID [32]byte

func (id FeatureSetID) String() string { return fmt.Sprintf("%x", id[:]) }

// Feature is one normalized OpenType tag/value pair.
type Feature struct {
	Tag   string
	Value uint16
}

// FeatureSet is immutable by convention. Entries are sorted by tag.
type FeatureSet struct {
	entries []Feature
	id      FeatureSetID
}

// NewFeatureSet projects the compatibility ligature switch, then overlays the
// explicit authored map. Tombstones have already removed entries during config
// composition, so an absent explicit tag reveals the compatibility projection.
func NewFeatureSet(ligatures bool, explicit map[string]int) (FeatureSet, error) {
	projected := map[string]int{"calt": 0, "clig": 0, "liga": 0}
	if ligatures {
		projected["calt"], projected["clig"], projected["liga"] = 1, 1, 1
	}
	for tag, value := range explicit {
		if err := ValidateFeatureTag(tag); err != nil {
			return FeatureSet{}, err
		}
		if value < 0 || value > FeatureValueMaximum {
			return FeatureSet{}, fmt.Errorf("feature %q value must be in 0..%d", tag, FeatureValueMaximum)
		}
		projected[tag] = value
	}
	if len(projected) > MaxFeatureTags {
		return FeatureSet{}, fmt.Errorf("effective feature count %d exceeds %d", len(projected), MaxFeatureTags)
	}
	tags := make([]string, 0, len(projected))
	for tag := range projected {
		if err := ValidateFeatureTag(tag); err != nil {
			return FeatureSet{}, err
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	set := FeatureSet{entries: make([]Feature, 0, len(tags))}
	for _, tag := range tags {
		set.entries = append(set.entries, Feature{Tag: tag, Value: uint16(projected[tag])})
	}
	encoded := set.CanonicalBytes()
	set.id = FeatureSetID(sha256.Sum256(encoded))
	return set, nil
}

func ValidateFeatureTag(tag string) error {
	if len(tag) != 4 {
		return fmt.Errorf("feature tag %q must contain exactly 4 ASCII bytes", tag)
	}
	for index := 0; index < len(tag); index++ {
		if tag[index] < 0x20 || tag[index] > 0x7e {
			return fmt.Errorf("feature tag %q must contain only printable ASCII bytes", tag)
		}
	}
	return nil
}

func (s FeatureSet) ID() FeatureSetID { return s.id }

func (s FeatureSet) IsZero() bool { return len(s.entries) == 0 }

func (s FeatureSet) Entries() []Feature { return append([]Feature(nil), s.entries...) }

func (s FeatureSet) CanonicalBytes() []byte {
	if len(s.entries) == 0 {
		return nil
	}
	var out bytes.Buffer
	out.WriteByte(featureEncodingV1)
	_ = binary.Write(&out, binary.BigEndian, uint16(len(s.entries)))
	for _, feature := range s.entries {
		out.WriteString(feature.Tag)
		_ = binary.Write(&out, binary.BigEndian, feature.Value)
	}
	return out.Bytes()
}

func (s FeatureSet) Value(tag string) (uint16, bool) {
	index := sort.Search(len(s.entries), func(i int) bool { return s.entries[i].Tag >= tag })
	if index >= len(s.entries) || s.entries[index].Tag != tag {
		return 0, false
	}
	return s.entries[index].Value, true
}

// RequestsFeatureCapability reports whether the effective set differs from the
// disabled compatibility projection and therefore warrants a capability report.
func (s FeatureSet) RequestsFeatureCapability() bool {
	for _, feature := range s.entries {
		if feature.Value != 0 || (feature.Tag != "calt" && feature.Tag != "clig" && feature.Tag != "liga") {
			return true
		}
	}
	return false
}

// RequiresRunShaping reports whether an enabled feature, including positioning-
// only features such as kern, must be presented to the shaper. The renderer still
// accepts only glyph-ID substitutions; advance-only changes fall back per cell.
func (s FeatureSet) RequiresRunShaping() bool {
	for _, feature := range s.entries {
		if feature.Value != 0 {
			return true
		}
	}
	return false
}

// EnablesRunSubstitution reports whether whole-run shaping can affect glyph IDs.
// Positioning-only kern never enables programming-run collection by itself.
func (s FeatureSet) EnablesRunSubstitution() bool {
	for _, feature := range s.entries {
		if feature.Value != 0 && feature.Tag != "kern" {
			return true
		}
	}
	return false
}

// EnablesSingleGlyphSubstitution excludes the compatibility ligature projection,
// whose substitutions require context across multiple characters.
func (s FeatureSet) EnablesSingleGlyphSubstitution() bool {
	for _, feature := range s.entries {
		if feature.Value == 0 {
			continue
		}
		switch feature.Tag {
		case "calt", "clig", "kern", "liga":
			continue
		default:
			return true
		}
	}
	return false
}
