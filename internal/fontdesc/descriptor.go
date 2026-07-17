// Package fontdesc defines dependency-light font descriptor and identity primitives.
package fontdesc

import (
	"fmt"
	"strings"
)

const (
	DefaultWeight  = 400
	DefaultStretch = 100
)

// Style is the requested slant of a font face.
type Style string

const (
	StyleNormal  Style = "normal"
	StyleItalic  Style = "italic"
	StyleOblique Style = "oblique"
)

// AttributeMode controls whether terminal bold and italic attributes alter a
// descriptor's authored target.
type AttributeMode string

const (
	AttributeModeAugment AttributeMode = "augment"
	AttributeModeFixed   AttributeMode = "fixed"
)

// OptionalCollectionIndex distinguishes an absent collection selector from an
// explicitly selected face at index zero.
type OptionalCollectionIndex struct {
	Value   uint32
	Present bool
}

// SomeCollectionIndex constructs a present collection index.
func SomeCollectionIndex(value uint32) OptionalCollectionIndex {
	return OptionalCollectionIndex{Value: value, Present: true}
}

// Descriptor is the canonical vocabulary for selecting a font face.
type Descriptor struct {
	Family          string
	CollectionFace  string
	CollectionIndex OptionalCollectionIndex
	Weight          int
	Style           Style
	Stretch         int
	AttributeMode   AttributeMode
}

// Normalized applies descriptor defaults and canonical whitespace. It does not
// silently repair invalid non-zero values; callers should then call Validate.
func (d Descriptor) Normalized() Descriptor {
	d.Family = normalizeName(d.Family)
	d.CollectionFace = normalizeName(d.CollectionFace)
	if d.Weight == 0 {
		d.Weight = DefaultWeight
	}
	if d.Style == "" {
		d.Style = StyleNormal
	}
	if d.Stretch == 0 {
		d.Stretch = DefaultStretch
	}
	if d.AttributeMode == "" {
		d.AttributeMode = AttributeModeAugment
	}
	return d
}

// Normalize applies defaults and returns a validated canonical descriptor.
func (d Descriptor) Normalize() (Descriptor, error) {
	d = d.Normalized()
	if err := d.Validate(); err != nil {
		return Descriptor{}, err
	}
	return d, nil
}

// Validate verifies a normalized or authored descriptor. Zero-valued fields
// that have defaults are accepted.
func (d Descriptor) Validate() error {
	d = d.Normalized()
	if d.Family == "" {
		return fmt.Errorf("font family is required")
	}
	if d.CollectionFace != "" && d.CollectionIndex.Present {
		return fmt.Errorf("collection_face and collection_index are mutually exclusive")
	}
	if d.CollectionIndex.Present && d.CollectionIndex.Value >= MaxFacesPerFile {
		return fmt.Errorf("collection index %d is outside 0..%d", d.CollectionIndex.Value, MaxFacesPerFile-1)
	}
	if d.Weight < 100 || d.Weight > 900 {
		return fmt.Errorf("font weight %d is outside 100..900", d.Weight)
	}
	switch d.Style {
	case StyleNormal, StyleItalic, StyleOblique:
	default:
		return fmt.Errorf("invalid font style %q", d.Style)
	}
	if d.Stretch < 50 || d.Stretch > 200 {
		return fmt.Errorf("font stretch %d is outside 50..200", d.Stretch)
	}
	switch d.AttributeMode {
	case AttributeModeAugment, AttributeModeFixed:
	default:
		return fmt.Errorf("invalid font attribute mode %q", d.AttributeMode)
	}
	return nil
}

func normalizeName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
