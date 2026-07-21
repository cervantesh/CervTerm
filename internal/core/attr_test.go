package core

import (
	"testing"
	"unsafe"
)

func TestAttrExplicitColorsUseLogicalKind(t *testing.T) {
	if (Attr{BG: DefaultColor()}).HasExplicitBG() {
		t.Fatal("default background should not count as explicit")
	}
	if (Attr{FG: DefaultColor()}).HasExplicitFG() {
		t.Fatal("default foreground should not count as explicit")
	}
	if !(Attr{BG: IndexedColor(0)}).HasExplicitBG() {
		t.Fatal("indexed background should count as explicit")
	}
	if !(Attr{FG: RGBColor(DefaultFG)}).HasExplicitFG() {
		t.Fatal("RGB literal equal to physical default foreground must remain explicit")
	}
	if !(Attr{BG: RGBColor(DefaultBG)}).HasExplicitBG() {
		t.Fatal("RGB literal equal to physical default background must remain explicit")
	}
}

func TestCellSizeInvariant(t *testing.T) {
	const want = uintptr(32)
	if got := unsafe.Sizeof(Cell{}); got != want {
		t.Fatalf("Cell size = %d bytes, want exactly %d", got, want)
	}
}
