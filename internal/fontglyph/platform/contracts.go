// Package platform owns native font analysis, shaping, and raster adapters.
// It never imports the fontglyph facade or renderer packages.
package platform

import (
	"image"

	"cervterm/internal/fontdesc"
)

// ShapeGlyph is the detached ABI-neutral output of native shaping.
type ShapeGlyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

// ShapeAllocator creates the caller-owned concrete result slice exactly once.
type ShapeAllocator[T any] func(int) []T

// ShapeAssigner projects one detached native glyph into caller-owned storage.
type ShapeAssigner[T any] func(*T, ShapeGlyph)

// FaceSource identifies one native face without transferring source bytes,
// cache leases, parsed-face ownership, or native handles.
type FaceSource struct {
	Path  string
	Index int
}

// TextRasterSpec contains the exact inputs required to select a native text
// rasterizer.
type TextRasterSpec struct {
	Mode       string
	Source     FaceSource
	SizePoints float64
	DPI        float64
}

// GlyphRasterizer is an injected native raster resource. Close is idempotent;
// owners close resources in reverse acquisition order.
type GlyphRasterizer interface {
	RasterizeGlyph(glyphID uint16, cellW, cellH, baseline, cellSpan int, advancePx float32) (*image.RGBA, bool)
	Close()
}

// ShapeRequest is detached from root and renderer types.
type ShapeRequest struct {
	Text     string
	Face     FaceSource
	PPEM     uint16
	Features fontdesc.FeatureSet
}

// NativeHooks provides an injection seam for platform tests and for callers
// that cannot exchange package-private native pointer types.
type NativeHooks struct {
	Shape  func(ShapeRequest) ([]ShapeGlyph, bool)
	Raster func(TextRasterSpec) GlyphRasterizer
}
