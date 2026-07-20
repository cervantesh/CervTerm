//go:build windows

package fontglyph

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	dwriteReadingDirectionLeftToRight = 0
)

type dwriteTextAnalysisSourceVtbl struct {
	queryInterface        uintptr
	addRef                uintptr
	release               uintptr
	getTextAtPosition     uintptr
	getTextBeforePosition uintptr
	getParagraphDirection uintptr
	getLocaleName         uintptr
	getNumberSubstitution uintptr
}

type dwriteTextAnalysisSinkVtbl struct {
	queryInterface        uintptr
	addRef                uintptr
	release               uintptr
	setScriptAnalysis     uintptr
	setLineBreakpoints    uintptr
	setBidiLevel          uintptr
	setNumberSubstitution uintptr
}

type dwriteTextAnalysis struct {
	text         []uint16
	locale       []uint16
	script       dwriteScriptAnalysis
	scriptLength uint32
	hasScript    bool
	source       dwriteTextAnalysisSource
	sink         dwriteTextAnalysisSink
}

type dwriteTextAnalysisSource struct {
	lpVtbl *dwriteTextAnalysisSourceVtbl
	owner  *dwriteTextAnalysis
}

type dwriteTextAnalysisSink struct {
	lpVtbl *dwriteTextAnalysisSinkVtbl
	owner  *dwriteTextAnalysis
}

var (
	dwriteTextAnalysisSourceVTable = dwriteTextAnalysisSourceVtbl{
		queryInterface:        syscall.NewCallback(dwriteAnalysisQueryInterface),
		addRef:                syscall.NewCallback(dwriteAnalysisAddRef),
		release:               syscall.NewCallback(dwriteAnalysisRelease),
		getTextAtPosition:     syscall.NewCallback(dwriteGetTextAtPosition),
		getTextBeforePosition: syscall.NewCallback(dwriteGetTextBeforePosition),
		getParagraphDirection: syscall.NewCallback(dwriteGetParagraphDirection),
		getLocaleName:         syscall.NewCallback(dwriteGetLocaleName),
		getNumberSubstitution: syscall.NewCallback(dwriteGetNumberSubstitution),
	}
	dwriteTextAnalysisSinkVTable = dwriteTextAnalysisSinkVtbl{
		queryInterface:        syscall.NewCallback(dwriteAnalysisQueryInterface),
		addRef:                syscall.NewCallback(dwriteAnalysisAddRef),
		release:               syscall.NewCallback(dwriteAnalysisRelease),
		setScriptAnalysis:     syscall.NewCallback(dwriteSetScriptAnalysis),
		setLineBreakpoints:    syscall.NewCallback(dwriteSetLineBreakpoints),
		setBidiLevel:          syscall.NewCallback(dwriteSetBidiLevel),
		setNumberSubstitution: syscall.NewCallback(dwriteSetNumberSubstitution),
	}
)

func newDWriteTextAnalysis(text []uint16) *dwriteTextAnalysis {
	locale, _ := syscall.UTF16FromString("en-us")
	analysis := &dwriteTextAnalysis{text: text, locale: locale}
	analysis.source = dwriteTextAnalysisSource{lpVtbl: &dwriteTextAnalysisSourceVTable, owner: analysis}
	analysis.sink = dwriteTextAnalysisSink{lpVtbl: &dwriteTextAnalysisSinkVTable, owner: analysis}
	return analysis
}

func (a *iWriteTextAnalyzer) analyzeScript(text []uint16) (dwriteScriptAnalysis, bool, error) {
	if a == nil || a.lpVtbl == nil || a.lpVtbl.analyzeScript == 0 {
		return dwriteScriptAnalysis{}, false, fmt.Errorf("IDWriteTextAnalyzer::AnalyzeScript unavailable")
	}
	analysis := newDWriteTextAnalysis(text)
	hr, _, callErr := syscall.SyscallN(
		a.lpVtbl.analyzeScript,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&analysis.source)),
		0,
		uintptr(len(text)),
		uintptr(unsafe.Pointer(&analysis.sink)),
	)
	if failedHRESULT(hr) {
		return dwriteScriptAnalysis{}, false, fmt.Errorf("IDWriteTextAnalyzer::AnalyzeScript: HRESULT 0x%08x (%v)", uint32(hr), callErr)
	}
	return analysis.script, analysis.hasScript, nil
}

//go:nocheckptr
func dwriteAnalysisQueryInterface(this uintptr, iid uintptr, object uintptr) uintptr {
	if object != 0 {
		*(*uintptr)(unsafe.Pointer(object)) = 0
	}
	return 0x80004002 // E_NOINTERFACE
}

func dwriteAnalysisAddRef(this uintptr) uintptr  { return 1 }
func dwriteAnalysisRelease(this uintptr) uintptr { return 1 }

//go:nocheckptr
func dwriteGetTextAtPosition(this uintptr, textPosition uintptr, textString uintptr, textLength uintptr) uintptr {
	source := (*dwriteTextAnalysisSource)(unsafe.Pointer(this))
	text := source.owner.text
	pos := int(textPosition)
	if pos >= len(text) {
		*(*uintptr)(unsafe.Pointer(textString)) = 0
		*(*uint32)(unsafe.Pointer(textLength)) = 0
		return 0
	}
	*(*uintptr)(unsafe.Pointer(textString)) = uintptr(unsafe.Pointer(&text[pos]))
	*(*uint32)(unsafe.Pointer(textLength)) = uint32(len(text) - pos)
	return 0
}

//go:nocheckptr
func dwriteGetTextBeforePosition(this uintptr, textPosition uintptr, textString uintptr, textLength uintptr) uintptr {
	source := (*dwriteTextAnalysisSource)(unsafe.Pointer(this))
	text := source.owner.text
	pos := int(textPosition)
	if pos <= 0 || len(text) == 0 {
		*(*uintptr)(unsafe.Pointer(textString)) = 0
		*(*uint32)(unsafe.Pointer(textLength)) = 0
		return 0
	}
	if pos > len(text) {
		pos = len(text)
	}
	*(*uintptr)(unsafe.Pointer(textString)) = uintptr(unsafe.Pointer(&text[0]))
	*(*uint32)(unsafe.Pointer(textLength)) = uint32(pos)
	return 0
}

func dwriteGetParagraphDirection(this uintptr) uintptr {
	return dwriteReadingDirectionLeftToRight
}

//go:nocheckptr
func dwriteGetLocaleName(this uintptr, textPosition uintptr, textLength uintptr, localeName uintptr) uintptr {
	source := (*dwriteTextAnalysisSource)(unsafe.Pointer(this))
	remaining := len(source.owner.text) - int(textPosition)
	if remaining < 0 {
		remaining = 0
	}
	*(*uint32)(unsafe.Pointer(textLength)) = uint32(remaining)
	*(*uintptr)(unsafe.Pointer(localeName)) = uintptr(unsafe.Pointer(&source.owner.locale[0]))
	return 0
}

//go:nocheckptr
func dwriteGetNumberSubstitution(this uintptr, textPosition uintptr, textLength uintptr, numberSubstitution uintptr) uintptr {
	source := (*dwriteTextAnalysisSource)(unsafe.Pointer(this))
	remaining := len(source.owner.text) - int(textPosition)
	if remaining < 0 {
		remaining = 0
	}
	*(*uint32)(unsafe.Pointer(textLength)) = uint32(remaining)
	*(*uintptr)(unsafe.Pointer(numberSubstitution)) = 0
	return 0
}

//go:nocheckptr
func dwriteSetScriptAnalysis(this uintptr, textPosition uintptr, textLength uintptr, scriptAnalysis uintptr) uintptr {
	sink := (*dwriteTextAnalysisSink)(unsafe.Pointer(this))
	if scriptAnalysis != 0 {
		sink.owner.script = *(*dwriteScriptAnalysis)(unsafe.Pointer(scriptAnalysis))
		sink.owner.scriptLength = uint32(textLength)
		sink.owner.hasScript = true
	}
	return 0
}

func dwriteSetLineBreakpoints(this uintptr, textPosition uintptr, textLength uintptr, lineBreakpoints uintptr) uintptr {
	return 0
}

func dwriteSetBidiLevel(this uintptr, textPosition uintptr, textLength uintptr, explicitLevel uintptr, resolvedLevel uintptr) uintptr {
	return 0
}

func dwriteSetNumberSubstitution(this uintptr, textPosition uintptr, textLength uintptr, numberSubstitution uintptr) uintptr {
	return 0
}
