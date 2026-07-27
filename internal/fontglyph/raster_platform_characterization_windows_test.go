//go:build windows

package fontglyph

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestL402DirectWriteSelectionABIAndCloseCharacterization(t *testing.T) {
	if got := reflect.TypeOf(newDefaultShaper()).String(); got != "fontglyph.DirectWriteShaper" {
		t.Fatalf("default Windows shaper type = %q", got)
	}
	if got, want := unsafe.Sizeof(dwriteScriptAnalysis{}), uintptr(8); got != want {
		t.Fatalf("DWRITE_SCRIPT_ANALYSIS size = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(dwriteScriptAnalysis{}.Shapes), uintptr(4); got != want {
		t.Fatalf("DWRITE_SCRIPT_ANALYSIS.shapes offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Sizeof(dwriteGlyphOffset{}), uintptr(8); got != want {
		t.Fatalf("DWRITE_GLYPH_OFFSET size = %d, want %d", got, want)
	}

	backend, err := NewOpenTypeBackend(Spec{Family: "Go Mono", Size: 14, DPI: 96, TextRaster: "auto"})
	if err != nil {
		t.Fatalf("NewOpenTypeBackend: %v", err)
	}
	if backend.dwRaster == nil {
		t.Fatal("Windows auto raster did not select DirectWrite")
	}
	backend.Close()
	backend.Close()
	if backend.dwRaster != nil || backend.faces != nil || !backend.closed {
		t.Fatalf("closed backend retained native/source state: %#v", backend)
	}
}
