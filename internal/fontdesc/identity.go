package fontdesc

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
)

const canonicalFormatVersion uint16 = 2

var canonicalDomain = []byte("cervterm/fontdesc")

// CanonicalEncoder writes tagged, length-prefixed fields into a bounded buffer.
// It retains the first error so callers cannot accidentally hash a partial ID.
type CanonicalEncoder struct {
	buf      []byte
	maxBytes int
	err      error
}

// NewCanonicalEncoder starts a versioned canonical record.
func NewCanonicalEncoder(version uint16, maxBytes int) *CanonicalEncoder {
	e := &CanonicalEncoder{maxBytes: maxBytes}
	if maxBytes < 0 {
		e.err = fmt.Errorf("negative canonical encoding bound %d", maxBytes)
		return e
	}
	e.AddBytes(0, canonicalDomain)
	e.AddUint16(1, version)
	return e
}

// AddBytes appends one tagged length-prefixed field.
func (e *CanonicalEncoder) AddBytes(tag uint16, value []byte) {
	if e.err != nil {
		return
	}
	if uint64(len(value)) > uint64(math.MaxUint32) {
		e.err = errors.New("canonical field exceeds uint32 length")
		return
	}
	const header = 6
	if len(e.buf) > e.maxBytes || len(value) > e.maxBytes-len(e.buf) || header > e.maxBytes-len(e.buf)-len(value) {
		e.err = fmt.Errorf("canonical encoding exceeds %d bytes", e.maxBytes)
		return
	}
	var h [header]byte
	binary.BigEndian.PutUint16(h[0:2], tag)
	binary.BigEndian.PutUint32(h[2:6], uint32(len(value)))
	e.buf = append(e.buf, h[:]...)
	e.buf = append(e.buf, value...)
}

func (e *CanonicalEncoder) AddString(tag uint16, value string) { e.AddBytes(tag, []byte(value)) }

func (e *CanonicalEncoder) AddBool(tag uint16, value bool) {
	v := byte(0)
	if value {
		v = 1
	}
	e.AddBytes(tag, []byte{v})
}

func (e *CanonicalEncoder) AddUint8(tag uint16, value uint8) {
	e.AddBytes(tag, []byte{value})
}

func (e *CanonicalEncoder) AddUint16(tag uint16, value uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], value)
	e.AddBytes(tag, b[:])
}

func (e *CanonicalEncoder) AddUint32(tag uint16, value uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], value)
	e.AddBytes(tag, b[:])
}

func (e *CanonicalEncoder) AddUint64(tag uint16, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	e.AddBytes(tag, b[:])
}

// Bytes returns an independent copy of the checked canonical record.
func (e *CanonicalEncoder) Bytes() ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	return append([]byte(nil), e.buf...), nil
}

// Sum returns the stable SHA-256 digest of the checked canonical record.
func (e *CanonicalEncoder) Sum() ([sha256.Size]byte, error) {
	if e.err != nil {
		return [sha256.Size]byte{}, e.err
	}
	return sha256.Sum256(e.buf), nil
}

// CanonicalFaceID is the comparable identity of one concrete source face.
type CanonicalFaceID [sha256.Size]byte

// CanonicalFaceIDFromBytes creates a domain-separated face identity.
func CanonicalFaceIDFromBytes(canonical []byte) CanonicalFaceID {
	e := NewCanonicalEncoder(canonicalFormatVersion, len(canonical)+64)
	e.AddBytes(10, canonical)
	sum, err := e.Sum()
	if err != nil {
		panic(err) // the exact bound above cannot fail for an in-memory byte slice
	}
	return CanonicalFaceID(sum)
}

func (id CanonicalFaceID) String() string { return hex.EncodeToString(id[:]) }

// FontEnvironmentKey identifies all configuration inputs shared by a retained
// font/raster environment.
type FontEnvironmentKey [sha256.Size]byte

func (id FontEnvironmentKey) String() string { return hex.EncodeToString(id[:]) }

// FontEnvironmentInput contains current fields plus canonical extension slots
// for later rules, features and metrics. Ordered slices remain order-sensitive.
type FontEnvironmentInput struct {
	Version       uint16
	Descriptors   []Descriptor
	Fallback      []Descriptor
	Rules         []Rule
	Features      []byte
	Metrics       []byte
	BaseSizeBits  uint64
	PaneZoomBits  uint64
	DPI           uint32
	RasterMode    string
	GammaBits     uint64
	DarkeningBits uint64
}

// NewFontEnvironmentKey validates and hashes a canonical environment record.
func NewFontEnvironmentKey(input FontEnvironmentInput) (FontEnvironmentKey, error) {
	if input.Version == 0 {
		input.Version = canonicalFormatVersion
	}
	if len(input.Descriptors) > MaxPrimaryDescriptors {
		return FontEnvironmentKey{}, fmt.Errorf("primary descriptor count %d exceeds %d", len(input.Descriptors), MaxPrimaryDescriptors)
	}
	if len(input.Fallback) > MaxFallbackDescriptors {
		return FontEnvironmentKey{}, fmt.Errorf("fallback descriptor count %d exceeds %d", len(input.Fallback), MaxFallbackDescriptors)
	}
	if len(input.Rules) > MaxRules {
		return FontEnvironmentKey{}, fmt.Errorf("rule count %d exceeds %d", len(input.Rules), MaxRules)
	}

	e := NewCanonicalEncoder(input.Version, MaxDescriptorPayloadBytes)
	if err := addDescriptors(e, 10, input.Descriptors); err != nil {
		return FontEnvironmentKey{}, err
	}
	if err := addDescriptors(e, 20, input.Fallback); err != nil {
		return FontEnvironmentKey{}, err
	}
	if err := addRules(e, 30, input.Rules); err != nil {
		return FontEnvironmentKey{}, err
	}
	e.AddBytes(40, input.Features)
	e.AddBytes(50, input.Metrics)
	e.AddUint64(60, input.BaseSizeBits)
	e.AddUint64(61, input.PaneZoomBits)
	e.AddUint32(62, input.DPI)
	e.AddString(63, input.RasterMode)
	e.AddUint64(64, input.GammaBits)
	e.AddUint64(65, input.DarkeningBits)
	sum, err := e.Sum()
	return FontEnvironmentKey(sum), err
}

func addDescriptors(e *CanonicalEncoder, tag uint16, descriptors []Descriptor) error {
	e.AddUint32(tag, uint32(len(descriptors)))
	for _, descriptor := range descriptors {
		encoded, err := encodeDescriptor(descriptor)
		if err != nil {
			return err
		}
		e.AddBytes(tag+1, encoded)
	}
	return nil
}

func addRules(e *CanonicalEncoder, tag uint16, rules []Rule) error {
	e.AddUint32(tag, uint32(len(rules)))
	totalRanges := 0
	for _, rule := range rules {
		normalized, err := rule.Normalize()
		if err != nil {
			return err
		}
		totalRanges += len(normalized.Match.Ranges)
		if totalRanges > MaxTotalRanges {
			return fmt.Errorf("total rule range count %d exceeds %d", totalRanges, MaxTotalRanges)
		}
		encoded, err := normalized.CanonicalBytes()
		if err != nil {
			return err
		}
		e.AddBytes(tag+1, encoded)
	}
	return nil
}

func encodeDescriptor(descriptor Descriptor) ([]byte, error) {
	d := descriptor.Normalized()
	if err := d.Validate(); err != nil {
		return nil, err
	}
	e := NewCanonicalEncoder(canonicalFormatVersion, MaxDescriptorPayloadBytes)
	e.AddString(10, strings.ToLower(d.Family))
	e.AddString(11, strings.ToLower(d.CollectionFace))
	e.AddBool(12, d.CollectionIndex.Present)
	if d.CollectionIndex.Present {
		e.AddUint32(13, d.CollectionIndex.Value)
	}
	e.AddUint16(14, uint16(d.Weight))
	e.AddString(15, string(d.Style))
	e.AddUint16(16, uint16(d.Stretch))
	e.AddString(17, string(d.AttributeMode))
	return e.Bytes()
}

// SourceTier identifies where a resolved candidate originated.
type SourceTier uint8

const (
	SourceTierRule SourceTier = iota
	SourceTierPrimary
	SourceTierFallback
	SourceTierEmbedded
)

// SyntheticMode records synthetic styling applied after face selection.
type SyntheticMode uint8

const (
	SyntheticNone   SyntheticMode = 0
	SyntheticBold   SyntheticMode = 1 << 0
	SyntheticItalic SyntheticMode = 1 << 1
)

// ResolvedFaceKey identifies one concrete resolution within an environment.
type ResolvedFaceKey [sha256.Size]byte

func (id ResolvedFaceKey) String() string { return hex.EncodeToString(id[:]) }

// FaceTarget is the effective weight, style and stretch incorporated into a
// resolved face identity. Style derivation and ranking land with descriptors.
type FaceTarget struct {
	Weight  int
	Style   Style
	Stretch int
}

// ResolvedFaceInput contains every face-selection dimension added to the
// environment identity.
type ResolvedFaceInput struct {
	Environment FontEnvironmentKey
	Face        CanonicalFaceID
	Tier        SourceTier
	SourceIndex uint32
	Target      FaceTarget
	Synthetic   SyntheticMode
}

// NewResolvedFaceKey hashes a concrete face resolution.
func NewResolvedFaceKey(input ResolvedFaceInput) (ResolvedFaceKey, error) {
	if input.Target.Weight < 100 || input.Target.Weight > 900 {
		return ResolvedFaceKey{}, fmt.Errorf("target weight %d is outside 100..900", input.Target.Weight)
	}
	if input.Target.Stretch < 50 || input.Target.Stretch > 200 {
		return ResolvedFaceKey{}, fmt.Errorf("target stretch %d is outside 50..200", input.Target.Stretch)
	}
	switch input.Target.Style {
	case StyleNormal, StyleItalic, StyleOblique:
	default:
		return ResolvedFaceKey{}, fmt.Errorf("invalid target style %q", input.Target.Style)
	}
	if input.Tier > SourceTierEmbedded {
		return ResolvedFaceKey{}, fmt.Errorf("invalid source tier %d", input.Tier)
	}
	if input.Synthetic & ^(SyntheticBold|SyntheticItalic) != 0 {
		return ResolvedFaceKey{}, fmt.Errorf("invalid synthetic mode %d", input.Synthetic)
	}

	e := NewCanonicalEncoder(canonicalFormatVersion, 1024)
	e.AddBytes(10, input.Environment[:])
	e.AddBytes(11, input.Face[:])
	e.AddUint8(12, uint8(input.Tier))
	e.AddUint32(13, input.SourceIndex)
	e.AddUint16(14, uint16(input.Target.Weight))
	e.AddString(15, string(input.Target.Style))
	e.AddUint16(16, uint16(input.Target.Stretch))
	e.AddUint8(17, uint8(input.Synthetic))
	sum, err := e.Sum()
	return ResolvedFaceKey(sum), err
}
