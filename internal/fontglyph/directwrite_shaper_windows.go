//go:build windows

package fontglyph

import (
	"syscall"
	"unicode/utf8"

	"cervterm/internal/fontdesc"
	"cervterm/internal/unicodecluster"
)

type DirectWriteShaper struct {
	Fallback Shaper
}

func (s DirectWriteShaper) FeatureCapability() string {
	if directWriteTextAnalyzerAvailable() {
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
	if !directWriteTextAnalyzerAvailable() {
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
	return directWriteTextAnalyzerAvailable()
}

func fallbackShapeFeatures(fallback Shaper, cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	if fallback == nil {
		return nil, false
	}
	return shapeWithFeatures(fallback, cluster, face, ppem, features)
}

func shapeWithDirectWrite(cluster string, fontPath string, faceIndex int, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool) {
	factory, err := newDirectWriteFactory()
	if err != nil {
		return nil, false
	}
	defer factory.release()
	fontFace, err := factory.createFontFaceFromPathIndex(fontPath, faceIndex)
	if err != nil {
		return nil, false
	}
	defer fontFace.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		return nil, false
	}
	defer analyzer.release()
	shaped, ok, err := analyzer.shapeText(cluster, fontFace, ppem, features)
	if err != nil {
		return nil, false
	}
	return shaped, ok
}

func directWriteAvailable() bool {
	dll, err := syscall.LoadDLL("dwrite.dll")
	if err != nil {
		return false
	}
	defer dll.Release()
	proc, err := dll.FindProc("DWriteCreateFactory")
	return err == nil && proc != nil
}
