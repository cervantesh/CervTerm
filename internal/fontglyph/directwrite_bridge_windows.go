//go:build windows

package fontglyph

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	dwriteFactoryTypeShared              = 0
	dwriteFactoryTypeIsolated            = 1
	dwriteFontFaceTypeTrueType           = 1
	dwriteFontFaceTypeOpenTypeCollection = 2
)

var (
	dwriteDLL               = syscall.NewLazyDLL("dwrite.dll")
	procDWriteCreateFactory = dwriteDLL.NewProc("DWriteCreateFactory")
	iidIDWriteFactory       = syscall.GUID{Data1: 0xb859ee5a, Data2: 0xd838, Data3: 0x4b5b, Data4: [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
)

type iUnknownVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
}

type iUnknown struct {
	lpVtbl *iUnknownVtbl
}

type iWriteTextAnalyzer struct {
	lpVtbl *iWriteTextAnalyzerVtbl
}

type iWriteTextAnalyzerVtbl struct {
	queryInterface                  uintptr
	addRef                          uintptr
	release                         uintptr
	analyzeScript                   uintptr
	analyzeBidi                     uintptr
	analyzeNumberSubstitution       uintptr
	analyzeLineBreakpoints          uintptr
	getGlyphs                       uintptr
	getGlyphPlacements              uintptr
	getGdiCompatibleGlyphPlacements uintptr
}

type iWriteFactory struct {
	lpVtbl *iWriteFactoryVtbl
}

type iWriteFactoryVtbl struct {
	queryInterface                 uintptr
	addRef                         uintptr
	release                        uintptr
	getSystemFontCollection        uintptr
	createCustomFontCollection     uintptr
	registerFontCollectionLoader   uintptr
	unregisterFontCollectionLoader uintptr
	createFontFileReference        uintptr
	createCustomFontFileReference  uintptr
	createFontFace                 uintptr
	createRenderingParams          uintptr
	createMonitorRenderingParams   uintptr
	createCustomRenderingParams    uintptr
	registerFontFileLoader         uintptr
	unregisterFontFileLoader       uintptr
	createTextFormat               uintptr
	createTypography               uintptr
	getGdiInterop                  uintptr
	createTextLayout               uintptr
	createGdiCompatibleTextLayout  uintptr
	createEllipsisTrimmingSign     uintptr
	createTextAnalyzer             uintptr
	createNumberSubstitution       uintptr
	createGlyphRunAnalysis         uintptr
}

func newDirectWriteFactory() (*iWriteFactory, error) {
	return newDirectWriteFactoryOfType(dwriteFactoryTypeShared)
}

// newDirectWriteFactoryOfType exists because the shared factory's font-file
// cache keeps referenced font files locked for the process lifetime; callers
// that must release file locks on Close (the glyph rasterizer) use an
// isolated factory instead.
func newDirectWriteFactoryOfType(factoryType uintptr) (*iWriteFactory, error) {
	if err := dwriteDLL.Load(); err != nil {
		return nil, err
	}
	var factory *iWriteFactory
	hr, _, err := procDWriteCreateFactory.Call(
		factoryType,
		uintptr(unsafe.Pointer(&iidIDWriteFactory)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if failedHRESULT(hr) {
		return nil, fmt.Errorf("DWriteCreateFactory: HRESULT 0x%08x (%v)", uint32(hr), err)
	}
	if factory == nil {
		return nil, fmt.Errorf("DWriteCreateFactory returned nil factory")
	}
	return factory, nil
}

func (f *iWriteFactory) release() {
	if f != nil && f.lpVtbl != nil && f.lpVtbl.release != 0 {
		syscall.SyscallN(f.lpVtbl.release, uintptr(unsafe.Pointer(f)))
	}
}

func (f *iWriteFactory) createTextAnalyzer() (*iWriteTextAnalyzer, error) {
	if f == nil || f.lpVtbl == nil || f.lpVtbl.createTextAnalyzer == 0 {
		return nil, fmt.Errorf("IDWriteFactory::CreateTextAnalyzer unavailable")
	}
	var analyzer *iWriteTextAnalyzer
	hr, _, err := syscall.SyscallN(f.lpVtbl.createTextAnalyzer, uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(&analyzer)))
	if failedHRESULT(hr) {
		return nil, fmt.Errorf("IDWriteFactory::CreateTextAnalyzer: HRESULT 0x%08x (%v)", uint32(hr), err)
	}
	if analyzer == nil {
		return nil, fmt.Errorf("IDWriteFactory::CreateTextAnalyzer returned nil analyzer")
	}
	return analyzer, nil
}

func (a *iWriteTextAnalyzer) release() {
	if a != nil && a.lpVtbl != nil && a.lpVtbl.release != 0 {
		syscall.SyscallN(a.lpVtbl.release, uintptr(unsafe.Pointer(a)))
	}
}

func (a *iWriteTextAnalyzer) hasGlyphShapingMethods() bool {
	return a != nil && a.lpVtbl != nil && a.lpVtbl.analyzeScript != 0 && a.lpVtbl.getGlyphs != 0 && a.lpVtbl.getGlyphPlacements != 0
}

type dwriteScriptAnalysis struct {
	Script uint16
	_      uint16
	Shapes uint32
}

type dwriteGlyphOffset struct {
	AdvanceOffset  float32
	AscenderOffset float32
}

func (a *iWriteTextAnalyzer) shapeText(text string, fontFace *iUnknown, ppem uint16) ([]ShapedGlyph, bool, error) {
	if !a.hasGlyphShapingMethods() {
		return nil, false, fmt.Errorf("IDWriteTextAnalyzer shaping methods unavailable")
	}
	if text == "" || fontFace == nil {
		return nil, false, nil
	}
	utf16Text := utf16.Encode([]rune(text))
	if len(utf16Text) == 0 {
		return nil, false, nil
	}
	textLength := uint32(len(utf16Text))
	maxGlyphCount := uint32(len(utf16Text)*3 + 16)
	clusterMap := make([]uint16, textLength)
	textProps := make([]uint16, textLength)
	glyphIndices := make([]uint16, maxGlyphCount)
	glyphProps := make([]uint16, maxGlyphCount)
	var actualGlyphCount uint32
	script, ok, err := a.analyzeScript(utf16Text)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	localeName, err := syscall.UTF16PtrFromString("en-us")
	if err != nil {
		return nil, false, err
	}
	hr, _, callErr := syscall.Syscall18(
		a.lpVtbl.getGlyphs, 18,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&utf16Text[0])),
		uintptr(textLength),
		uintptr(unsafe.Pointer(fontFace)),
		0, // isSideways
		0, // isRightToLeft
		uintptr(unsafe.Pointer(&script)),
		uintptr(unsafe.Pointer(localeName)), // localeName
		0,                                   // numberSubstitution
		0,                                   // features
		0,                                   // featureRangeLengths
		0,                                   // featureRanges
		uintptr(maxGlyphCount),
		uintptr(unsafe.Pointer(&clusterMap[0])),
		uintptr(unsafe.Pointer(&textProps[0])),
		uintptr(unsafe.Pointer(&glyphIndices[0])),
		uintptr(unsafe.Pointer(&glyphProps[0])),
		uintptr(unsafe.Pointer(&actualGlyphCount)),
	)
	if failedHRESULT(hr) {
		return nil, false, fmt.Errorf("IDWriteTextAnalyzer::GetGlyphs: HRESULT 0x%08x (%v)", uint32(hr), callErr)
	}
	if actualGlyphCount == 0 || actualGlyphCount > maxGlyphCount {
		return nil, false, nil
	}

	glyphAdvances := make([]float32, actualGlyphCount)
	glyphOffsets := make([]dwriteGlyphOffset, actualGlyphCount)
	hr, _, callErr = syscall.SyscallN(
		a.lpVtbl.getGlyphPlacements,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&utf16Text[0])),
		uintptr(unsafe.Pointer(&clusterMap[0])),
		uintptr(unsafe.Pointer(&textProps[0])),
		uintptr(textLength),
		uintptr(unsafe.Pointer(&glyphIndices[0])),
		uintptr(unsafe.Pointer(&glyphProps[0])),
		uintptr(actualGlyphCount),
		uintptr(unsafe.Pointer(fontFace)),
		uintptr(math32bits(float32(ppem))),
		0, // isSideways
		0, // isRightToLeft
		uintptr(unsafe.Pointer(&script)),
		uintptr(unsafe.Pointer(localeName)), // localeName
		0,                                   // features
		0,                                   // featureRangeLengths
		0,                                   // featureRanges
		uintptr(unsafe.Pointer(&glyphAdvances[0])),
		uintptr(unsafe.Pointer(&glyphOffsets[0])),
	)
	if failedHRESULT(hr) {
		return nil, false, fmt.Errorf("IDWriteTextAnalyzer::GetGlyphPlacements: HRESULT 0x%08x (%v)", uint32(hr), callErr)
	}

	shaped := make([]ShapedGlyph, 0, actualGlyphCount)
	for i := uint32(0); i < actualGlyphCount; i++ {
		glyphID := glyphIndices[i]
		if glyphID == 0 {
			continue
		}
		shaped = append(shaped, ShapedGlyph{
			GlyphID:  glyphID,
			XOffset:  float64(glyphOffsets[i].AdvanceOffset),
			YOffset:  -float64(glyphOffsets[i].AscenderOffset),
			XAdvance: float64(glyphAdvances[i]),
		})
	}
	if len(shaped) == 0 {
		return nil, false, nil
	}
	return shaped, true, nil
}

func math32bits(f float32) uint32 {
	return *(*uint32)(unsafe.Pointer(&f))
}

func (f *iWriteFactory) createFontFaceFromPath(path string) (*iUnknown, error) {
	if f == nil || f.lpVtbl == nil || f.lpVtbl.createFontFileReference == 0 || f.lpVtbl.createFontFace == 0 {
		return nil, fmt.Errorf("IDWriteFactory font-face APIs unavailable")
	}
	path16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var fontFile *iUnknown
	hr, _, callErr := syscall.SyscallN(
		f.lpVtbl.createFontFileReference,
		uintptr(unsafe.Pointer(f)),
		uintptr(unsafe.Pointer(path16)),
		0,
		uintptr(unsafe.Pointer(&fontFile)),
	)
	if failedHRESULT(hr) {
		return nil, fmt.Errorf("IDWriteFactory::CreateFontFileReference(%s): HRESULT 0x%08x (%v)", path, uint32(hr), callErr)
	}
	if fontFile == nil {
		return nil, fmt.Errorf("IDWriteFactory::CreateFontFileReference(%s) returned nil font file", path)
	}
	// Keep fontFile alive with the returned fontFace. Some DirectWrite builds keep
	// references to the file loader data while shaping; releasing it here caused
	// GetGlyphs to reject otherwise valid arguments in the Go COM bridge.

	fontFaceType := uintptr(dwriteFontFaceTypeTrueType)
	if strings.EqualFold(filepath.Ext(path), ".ttc") {
		fontFaceType = dwriteFontFaceTypeOpenTypeCollection
	}
	fontFiles := []*iUnknown{fontFile}
	var fontFace *iUnknown
	hr, _, callErr = syscall.SyscallN(
		f.lpVtbl.createFontFace,
		uintptr(unsafe.Pointer(f)),
		fontFaceType,
		1,
		uintptr(unsafe.Pointer(&fontFiles[0])),
		0,
		0,
		uintptr(unsafe.Pointer(&fontFace)),
	)
	if failedHRESULT(hr) {
		return nil, fmt.Errorf("IDWriteFactory::CreateFontFace(%s): HRESULT 0x%08x (%v)", path, uint32(hr), callErr)
	}
	if fontFace == nil {
		return nil, fmt.Errorf("IDWriteFactory::CreateFontFace(%s) returned nil font face", path)
	}
	return fontFace, nil
}

func (u *iUnknown) release() {
	if u != nil && u.lpVtbl != nil && u.lpVtbl.release != 0 {
		syscall.SyscallN(u.lpVtbl.release, uintptr(unsafe.Pointer(u)))
	}
}

func directWriteTextAnalyzerAvailable() bool {
	factory, err := newDirectWriteFactory()
	if err != nil {
		return false
	}
	defer factory.release()
	analyzer, err := factory.createTextAnalyzer()
	if err != nil {
		return false
	}
	analyzer.release()
	return true
}

func failedHRESULT(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}
