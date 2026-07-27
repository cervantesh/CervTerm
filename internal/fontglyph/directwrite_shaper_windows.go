//go:build windows

package fontglyph

import (
	"unicode/utf8"

	"cervterm/internal/fontdesc"
	platformpkg "cervterm/internal/fontglyph/platform"
	"cervterm/internal/unicodecluster"
)

type DirectWriteShaper struct {
	Fallback Shaper
}

func (s DirectWriteShaper) FeatureCapability() string {
	if platformpkg.Available() {
		return "directwrite"
	}
	if reporter, ok := s.Fallback.(featureCapabilityReporter); ok {
		return reporter.FeatureCapability()
	}
	return "unsupported"
}

func (s DirectWriteShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	return s.ShapeFeatures(cluster, face, ppem, fontdesc.FeatureSet{})
}

func (s DirectWriteShaper) ShapeFeatures(cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	if !platformpkg.Available() {
		return fallbackShapeFeatures(s.Fallback, cluster, face, ppem, features)
	}
	if isSimpleShapeableCluster(cluster) && !unicodecluster.IsEmojiString(cluster) {
		runeCount := utf8.RuneCountInString(cluster)
		if features.IsZero() || (runeCount <= 1 && !features.EnablesSingleGlyphSubstitution()) || (runeCount > 1 && !features.RequiresRunShaping()) {
			return fallbackShapeFeatures(s.Fallback, cluster, face, ppem, features)
		}
	}
	if face.sourcePath == "" {
		return nil, false
	}
	shaped, ok := shapeWithDirectWrite(cluster, face.sourcePath, face.faceIndex, ppem, features)
	if ok {
		return shaped, true
	}
	return nil, false
}

func (s DirectWriteShaper) Available() bool {
	return platformpkg.Available()
}

func fallbackShapeFeatures(fallback Shaper, cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	if fallback == nil {
		return nil, false
	}
	return shapeWithFeatures(fallback, cluster, face, ppem, features)
}

func shapeWithDirectWrite(cluster string, fontPath string, faceIndex int, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	return platformpkg.ShapeInto(platformpkg.ShapeRequest{
		Text: cluster,
		Face: platformpkg.FaceSource{
			Path:  fontPath,
			Index: faceIndex,
		},
		PPEM:     ppem,
		Features: features,
	}, allocateRootShapedGlyphs, assignRootShapedGlyph)
}

func allocateRootShapedGlyphs(count int) []ShapedGlyph { return make([]ShapedGlyph, count) }

func assignRootShapedGlyph(target *ShapedGlyph, glyph platformpkg.ShapeGlyph) {
	*target = ShapedGlyph{
		GlyphID:  glyph.GlyphID,
		XOffset:  glyph.XOffset,
		YOffset:  glyph.YOffset,
		XAdvance: glyph.XAdvance,
	}
}

func directWriteAvailable() bool { return platformpkg.Available() }
