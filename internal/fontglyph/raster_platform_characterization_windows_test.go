//go:build windows

package fontglyph

import (
	"reflect"
	"testing"

	platformpkg "cervterm/internal/fontglyph/platform"
)

func TestL402DirectWriteSelectionABIAndCloseCharacterization(t *testing.T) {
	if got := reflect.TypeOf(newDefaultShaper()).String(); got != "fontglyph.DirectWriteShaper" {
		t.Fatalf("default Windows shaper type = %q", got)
	}
	abi := platformpkg.ABI()
	if abi.ScriptAnalysisSize != 8 || abi.ScriptOffset != 0 || abi.ShapesOffset != 4 || abi.GlyphOffsetSize != 8 {
		t.Fatalf("DirectWrite ABI layout = %#v", abi)
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
