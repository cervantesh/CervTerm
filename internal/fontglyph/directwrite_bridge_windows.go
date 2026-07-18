//go:build windows

package fontglyph

import (
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"cervterm/internal/fontdesc"
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

type dwriteFontFeature struct {
	Name      uint32
	Parameter uint32
}

type dwriteTypographicFeatures struct {
	Features     *dwriteFontFeature
	FeatureCount uint32
}

type dwriteFeatureArguments struct {
	entries      []dwriteFontFeature
	typography   []dwriteTypographicFeatures
	pointers     []*dwriteTypographicFeatures
	rangeLengths []uint32
}

func newDirectWriteFeatureArguments(features fontdesc.FeatureSet, textLength uint32) dwriteFeatureArguments {
	entries := features.Entries()
	if len(entries) == 0 || textLength == 0 {
		return dwriteFeatureArguments{}
	}
	arguments := dwriteFeatureArguments{entries: make([]dwriteFontFeature, len(entries)), typography: make([]dwriteTypographicFeatures, 1), pointers: make([]*dwriteTypographicFeatures, 1), rangeLengths: []uint32{textLength}}
	for index, feature := range entries {
		arguments.entries[index] = dwriteFontFeature{Name: directWriteFeatureTag(feature.Tag), Parameter: uint32(feature.Value)}
	}
	arguments.typography[0] = dwriteTypographicFeatures{Features: &arguments.entries[0], FeatureCount: uint32(len(arguments.entries))}
	arguments.pointers[0] = &arguments.typography[0]
	return arguments
}

func directWriteFeatureTag(tag string) uint32 {
	if len(tag) != 4 {
		return 0
	}
	return uint32(tag[0]) | uint32(tag[1])<<8 | uint32(tag[2])<<16 | uint32(tag[3])<<24
}

func (a *dwriteFeatureArguments) callPointers() (features, lengths uintptr, ranges uintptr) {
	if len(a.pointers) == 0 {
		return 0, 0, 0
	}
	return uintptr(unsafe.Pointer(&a.pointers[0])), uintptr(unsafe.Pointer(&a.rangeLengths[0])), uintptr(len(a.pointers))
}

func (a *iWriteTextAnalyzer) shapeText(text string, fontFace *iUnknown, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool, error) {
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
	featureArguments := newDirectWriteFeatureArguments(features, textLength)
	featurePointer, featureRangeLengths, featureRanges := featureArguments.callPointers()
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
		featurePointer,
		featureRangeLengths,
		featureRanges,
		uintptr(maxGlyphCount),
		uintptr(unsafe.Pointer(&clusterMap[0])),
		uintptr(unsafe.Pointer(&textProps[0])),
		uintptr(unsafe.Pointer(&glyphIndices[0])),
		uintptr(unsafe.Pointer(&glyphProps[0])),
		uintptr(unsafe.Pointer(&actualGlyphCount)),
	)
	runtime.KeepAlive(featureArguments)
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
		featurePointer,
		featureRangeLengths,
		featureRanges,
		uintptr(unsafe.Pointer(&glyphAdvances[0])),
		uintptr(unsafe.Pointer(&glyphOffsets[0])),
	)
	runtime.KeepAlive(featureArguments)
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
	return f.createFontFaceFromPathIndex(path, 0)
}

func (f *iWriteFactory) createFontFaceFromPathIndex(path string, faceIndex int) (*iUnknown, error) {
	if f == nil || f.lpVtbl == nil || f.lpVtbl.createFontFileReference == 0 || f.lpVtbl.createFontFace == 0 {
		return nil, fmt.Errorf("IDWriteFactory font-face APIs unavailable")
	}
	if faceIndex < 0 || faceIndex >= fontdesc.MaxFacesPerFile {
		return nil, fmt.Errorf("font face index %d is outside 0..%d", faceIndex, fontdesc.MaxFacesPerFile-1)
	}
	fontFile, faceType, err := f.openFontFile(path)
	if err != nil {
		return nil, err
	}
	defer fontFile.release()
	return f.createAnalyzedFontFace(fontFile, faceType, faceIndex)
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
