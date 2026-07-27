// Package shape owns font face resolution policy and renderer-neutral shaping.
// It never imports the fontglyph compatibility facade or sibling subsystems.
package shape

import (
	"cervterm/internal/fontdesc"
	"cervterm/internal/fontglyph/internal/face"
)

// Glyph is the detached renderer-neutral output of one shaping operation.
type Glyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

// Shaper shapes one complete cluster against an immutable face view.
type Shaper interface {
	Shape(cluster string, face face.Ref, ppem uint16) ([]Glyph, bool)
}

// FeatureShaper applies an immutable effective feature projection.
type FeatureShaper interface {
	ShapeFeatures(cluster string, face face.Ref, ppem uint16, features fontdesc.FeatureSet) ([]Glyph, bool)
}

// FeatureCapabilityReporter is the narrow optional diagnostic seam implemented
// by portable and injected platform shapers.
type FeatureCapabilityReporter interface {
	FeatureCapability() string
}

// Simple is the portable one-glyph-per-rune shaper.
type Simple struct{}

// PlatformFactory may inject a platform shaping implementation. Native face
// construction and raster ownership remain outside this package for Slice 5.5c.
type PlatformFactory func(fallback Shaper) Shaper
