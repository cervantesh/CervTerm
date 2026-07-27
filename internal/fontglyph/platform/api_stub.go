//go:build !windows

package platform

import "errors"

// Available reports that DirectWrite is unavailable off Windows.
func Available() bool { return false }

// Shape fails soft off Windows.
func Shape(ShapeRequest) ([]ShapeGlyph, bool) { return nil, false }

// ShapeInto fails soft off Windows without allocating a result.
func ShapeInto[T any](ShapeRequest, ShapeAllocator[T], ShapeAssigner[T]) ([]T, bool) {
	return nil, false
}

// NewTextRasterizer preserves the portable fallback off Windows.
func NewTextRasterizer(TextRasterSpec) GlyphRasterizer { return nil }

// CachedGoMonoPath is not used off Windows.
func CachedGoMonoPath() (string, error) { return "", errors.New("DirectWrite is unavailable") }

// NewDirectWriteRasterizer is unavailable off Windows.
func NewDirectWriteRasterizer(FaceSource, float64, float64) (GlyphRasterizer, error) {
	return nil, errors.New("DirectWrite is unavailable")
}
