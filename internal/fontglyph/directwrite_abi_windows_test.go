//go:build windows

package fontglyph

import (
	"testing"
	"unsafe"
)

func TestDirectWriteStructLayouts(t *testing.T) {
	if got, want := unsafe.Sizeof(dwriteScriptAnalysis{}), uintptr(8); got != want {
		t.Fatalf("DWRITE_SCRIPT_ANALYSIS size = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(dwriteScriptAnalysis{}.Script), uintptr(0); got != want {
		t.Fatalf("DWRITE_SCRIPT_ANALYSIS.script offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(dwriteScriptAnalysis{}.Shapes), uintptr(4); got != want {
		t.Fatalf("DWRITE_SCRIPT_ANALYSIS.shapes offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(dwriteGlyphOffset{}), uintptr(8); got != want {
		t.Fatalf("DWRITE_GLYPH_OFFSET size = %d, want %d", got, want)
	}
}
