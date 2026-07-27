//go:build windows

package platform

import (
	"log"
	"sync"
)

var embeddedFontLogOnce sync.Once

// Available reports whether DirectWrite analysis and shaping entry points are usable.
func Available() bool { return directWriteTextAnalyzerAvailable() }

// Shape executes one detached DirectWrite shaping request.
func Shape(request ShapeRequest) ([]ShapeGlyph, bool) {
	return ShapeInto(request, allocatePlatformShapeGlyphs, assignPlatformShapeGlyph)
}

// ShapeInto executes one DirectWrite shaping request while allocating only the
// caller's concrete result slice. Native analysis and buffers remain owned here.
func ShapeInto[T any](request ShapeRequest, allocate ShapeAllocator[T], assign ShapeAssigner[T]) ([]T, bool) {
	if request.Text == "" || request.Face.Path == "" || allocate == nil || assign == nil {
		return nil, false
	}
	factory, err := newDirectWriteFactory()
	if err != nil {
		return nil, false
	}
	defer factory.release()
	fontFace, err := factory.createFontFaceFromPathIndex(request.Face.Path, request.Face.Index)
	if err != nil {
		return nil, false
	}
	defer fontFace.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		return nil, false
	}
	defer analyzer.release()
	shaped, ok, err := shapeTextInto(analyzer, request.Text, fontFace, request.PPEM, request.Features, allocate, assign)
	if err != nil {
		return nil, false
	}
	return shaped, ok
}

func allocatePlatformShapeGlyphs(count int) []ShapeGlyph { return make([]ShapeGlyph, count) }

func assignPlatformShapeGlyph(target *ShapeGlyph, glyph ShapeGlyph) { *target = glyph }

// NewTextRasterizer selects the historical DirectWrite raster path.
func NewTextRasterizer(spec TextRasterSpec) GlyphRasterizer {
	if spec.Mode != "auto" {
		return nil
	}
	path := spec.Source.Path
	if path == "" {
		var err error
		path, err = cachedGoMonoPath()
		if err != nil {
			embeddedFontLogOnce.Do(func() { log.Printf("DirectWrite raster disabled: cache embedded Go Mono: %v", err) })
			return nil
		}
	}
	rasterizer, err := newDWriteRasterizer(path, spec.Source.Index, spec.SizePoints, spec.DPI)
	if err != nil {
		log.Printf("DirectWrite raster unavailable for %s: %v", path, err)
		return nil
	}
	return rasterizer
}

// CachedGoMonoPath materializes the embedded fallback through the same atomic
// cache path used by production native raster selection.
func CachedGoMonoPath() (string, error) { return cachedGoMonoPath() }

// NewDirectWriteRasterizer is the platform-native construction seam used by
// root compatibility tests and adapters.
func NewDirectWriteRasterizer(source FaceSource, sizePoints, dpi float64) (GlyphRasterizer, error) {
	return newDWriteRasterizer(source.Path, source.Index, sizePoints, dpi)
}
