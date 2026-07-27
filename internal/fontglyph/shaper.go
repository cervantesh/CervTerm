package fontglyph

import (
	"image"
	"log"
	"sync"

	"cervterm/internal/fontdesc"
	rasterpkg "cervterm/internal/fontglyph/raster"
	shapepkg "cervterm/internal/fontglyph/shape"
)

type ShapedGlyph struct {
	GlyphID  uint16
	XOffset  float64
	YOffset  float64
	XAdvance float64
}

type Shaper interface {
	Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)
}

type FeatureShaper interface {
	ShapeFeatures(cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool)
}

func shapeWithFeatures(shaper Shaper, cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	if featureShaper, ok := shaper.(FeatureShaper); ok {
		return featureShaper.ShapeFeatures(cluster, face, ppem, features)
	}
	return shaper.Shape(cluster, face, ppem)
}

var portableFeatureDiagnosticOnce sync.Once

// ConfigureBackendFeatures installs one immutable effective feature set into a
// context-local backend graph. Parsed font cache entries remain feature-neutral.
func ConfigureBackendFeatures(backend Backend, features fontdesc.FeatureSet) {
	var configureOpenType func(*OpenTypeBackend)
	configureOpenType = func(item *OpenTypeBackend) {
		if item == nil {
			return
		}
		item.features = features
		if features.RequestsFeatureCapability() {
			if isPortableShaper(item.shaper) {
				portableFeatureDiagnosticOnce.Do(func() {
					log.Printf("font feature capability: portable SimpleShaper preserves the fixed grid but does not apply OpenType substitutions")
				})
			}
		}
	}
	switch typed := backend.(type) {
	case *OpenTypeBackend:
		configureOpenType(typed)
	case *descriptorBackend:
		for _, item := range typed.backends {
			configureOpenType(item)
		}
	case *fallbackBackend:
		typed.features = features
		if typed.primary != nil {
			for _, item := range typed.primary.backends {
				configureOpenType(item)
			}
		}
		for _, item := range typed.loaded {
			configureOpenType(item)
		}
	}
}

type featureCapabilityReporter interface {
	FeatureCapability() string
}

// BackendFeatureCapability reports whether the active platform shaper applies
// configured OpenType features without exposing font paths or glyph data.
func BackendFeatureCapability(backend Backend) string {
	var shaper Shaper
	switch typed := backend.(type) {
	case *OpenTypeBackend:
		shaper = typed.shaper
	case *descriptorBackend:
		if typed.backends[fontdesc.RequestedFaceStyleNormal] != nil {
			shaper = typed.backends[fontdesc.RequestedFaceStyleNormal].shaper
		}
	case *fallbackBackend:
		if typed.primary != nil && typed.primary.backends[fontdesc.RequestedFaceStyleNormal] != nil {
			shaper = typed.primary.backends[fontdesc.RequestedFaceStyleNormal].shaper
		}
	}
	if reporter, ok := shaper.(featureCapabilityReporter); ok {
		return reporter.FeatureCapability()
	}
	return "unsupported"
}

func (b *OpenTypeBackend) SetShaper(shaper Shaper) {
	b.shaper = shaper
}

func centerShapedGlyphsInCells(shaped []ShapedGlyph, cellPixels int) []ShapedGlyph {
	return shapedGlyphsFromShape(shapepkg.CenterInCells(shapedGlyphsToShape(shaped), cellPixels))
}

func isPortableShaper(shaper Shaper) bool {
	switch typed := shaper.(type) {
	case SimpleShaper:
		return true
	case shapeToRootShaper:
		_, portable := typed.inner.(shapepkg.Simple)
		return portable
	default:
		return false
	}
}

func (b *OpenTypeBackend) rasterizeShapedCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if lf.sfnt == nil || len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	if glyph, ok := b.rasterizeShapedBitmapColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	if glyph, ok := b.rasterizeShapedColorCluster(lf, shaped, cellSpan); ok {
		return glyph, true
	}
	renderer := rasterpkg.Renderer{Metrics: rasterMetrics(b)}
	glyph, ok := renderer.RasterizeShapedMonochrome(rasterFace(lf), rasterShapeGlyphs(shaped), cellSpan)
	return rasterGlyphToRoot(glyph), ok
}

func (b *OpenTypeBackend) rasterizeShapedBitmapColorCluster(lf loadedFace, shaped []ShapedGlyph, cellSpan int) (RasterizedGlyph, bool) {
	if len(shaped) == 0 {
		return RasterizedGlyph{}, false
	}
	bitmaps := make([]rasterpkg.BitmapGlyph, 0, len(shaped))
	for _, glyph := range shaped {
		if glyph.GlyphID == 0 {
			return RasterizedGlyph{}, false
		}
		bitmap, ok := bitmapColorGlyph(lf, glyph.GlyphID, b.ppem)
		if !ok {
			return RasterizedGlyph{}, false
		}
		bitmaps = append(bitmaps, rasterpkg.BitmapGlyph{
			Image: bitmap.Image, PPEM: bitmap.PPEM,
			OriginOffsetX: bitmap.OriginOffsetX, OriginOffsetY: bitmap.OriginOffsetY, Format: bitmap.Format,
		})
	}
	glyph, ok := rasterpkg.RasterizeShapedBitmaps(bitmaps, rasterShapeGlyphs(shaped), rasterMetrics(b), cellSpan)
	return rasterGlyphToRoot(glyph), ok
}

func hasVisibleRGBA(img *image.RGBA) bool { return rasterpkg.HasVisibleRGBA(img) }
