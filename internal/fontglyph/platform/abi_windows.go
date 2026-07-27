//go:build windows

package platform

import "unsafe"

// ABIInfo reports the DirectWrite layouts pinned by the native bridge.
type ABIInfo struct {
	ScriptAnalysisSize uintptr
	ScriptOffset       uintptr
	ShapesOffset       uintptr
	GlyphOffsetSize    uintptr
}

// ABI returns the native layout projection without exposing package-private COM types.
func ABI() ABIInfo {
	return ABIInfo{
		ScriptAnalysisSize: unsafe.Sizeof(dwriteScriptAnalysis{}),
		ScriptOffset:       unsafe.Offsetof(dwriteScriptAnalysis{}.Script),
		ShapesOffset:       unsafe.Offsetof(dwriteScriptAnalysis{}.Shapes),
		GlyphOffsetSize:    unsafe.Sizeof(dwriteGlyphOffset{}),
	}
}
