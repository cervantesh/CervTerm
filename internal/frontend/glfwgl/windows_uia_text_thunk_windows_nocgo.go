//go:build glfw && windows && !cgo

package glfwgl

import (
	"math"
	"unsafe"
)

func nativeUIATextRangeFromPointCallback() uintptr { return 0 }
func nativeUIATextFindAttributeCallback() uintptr  { return 0 }

//go:nocheckptr
func testNativeUIATextRangeFromPoint(_ uintptr, self uintptr, x, y float64) (uintptr, uintptr) {
	var output uintptr
	hr := nativeUIATextRangeFromPointBits(self, math.Float64bits(x), math.Float64bits(y), uintptr(unsafe.Pointer(&output)))
	return hr, output
}

//go:nocheckptr
func testNativeUIATextFindAttribute(_ uintptr, self uintptr) (uintptr, uintptr) {
	var output uintptr
	hr := nativeUIATextRangeFindAttribute(self, 1, 0, uintptr(unsafe.Pointer(&output)))
	return hr, output
}
