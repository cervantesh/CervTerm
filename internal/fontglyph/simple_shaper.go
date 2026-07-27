package fontglyph

import (
	"cervterm/internal/fontdesc"
	faceleaf "cervterm/internal/fontglyph/internal/face"
	shapepkg "cervterm/internal/fontglyph/shape"

	"golang.org/x/image/font/sfnt"
)

// SimpleShaper remains the concrete root compatibility type.
type SimpleShaper struct{}

func (SimpleShaper) FeatureCapability() string { return (shapepkg.Simple{}).FeatureCapability() }

func (SimpleShaper) ShapeFeatures(cluster string, source loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	glyphs, ok := (shapepkg.Simple{}).ShapeFeatures(cluster, shapingFaceRef(source), ppem, features)
	return shapedGlyphsFromShape(glyphs), ok
}

func (SimpleShaper) Shape(cluster string, source loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	glyphs, ok := (shapepkg.Simple{}).Shape(cluster, shapingFaceRef(source), ppem)
	return shapedGlyphsFromShape(glyphs), ok
}

// shapeOneRune is retained only as the same-package concrete compatibility
// bridge used by injected shapers and tests.
func shapeOneRune(parsed *sfnt.Font, value rune, ppem uint16) ([]ShapedGlyph, bool) {
	glyphs, ok := shapepkg.ShapeOneRune(faceleaf.NewRef(parsed, "", 0), value, ppem)
	return shapedGlyphsFromShape(glyphs), ok
}

func isSimpleShapeableCluster(cluster string) bool { return shapepkg.IsSimpleCluster(cluster) }

func isComplexShapingRune(value rune) bool { return shapepkg.IsComplexRune(value) }
