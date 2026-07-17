//go:build windows

package fontglyph

import (
	"syscall"

	"cervterm/internal/unicodecluster"
)

type DirectWriteShaper struct {
	Fallback Shaper
}

func (s DirectWriteShaper) Shape(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if !directWriteTextAnalyzerAvailable() {
		return fallbackShape(s.Fallback, cluster, face, ppem)
	}
	if isSimpleShapeableCluster(cluster) && !unicodecluster.IsEmojiString(cluster) {
		return fallbackShape(s.Fallback, cluster, face, ppem)
	}
	if face.sourcePath == "" {
		return nil, false
	}
	shaped, ok := shapeWithDirectWrite(cluster, face.sourcePath, face.faceIndex, ppem)
	if ok {
		return shaped, true
	}
	return nil, false
}

func (s DirectWriteShaper) Available() bool {
	return directWriteTextAnalyzerAvailable()
}

func fallbackShape(fallback Shaper, cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool) {
	if fallback == nil {
		return nil, false
	}
	return fallback.Shape(cluster, face, ppem)
}

func shapeWithDirectWrite(cluster string, fontPath string, faceIndex int, ppem uint16) ([]ShapedGlyph, bool) {
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
	shaped, ok, err := analyzer.shapeText(cluster, fontFace, ppem)
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
