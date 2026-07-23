//go:build glfw && windows && !cgo

package glfwgl

import (
	"math"
	"unsafe"
)

func nativeUIAElementFromPointCallback() uintptr { return 0 }

//go:nocheckptr
func testNativeUIAElementFromPoint(_ uintptr, self uintptr, x, y float64) (uintptr, uintptr) {
	var output uintptr
	hr := nativeUIAElementFromPointBits(self, math.Float64bits(x), math.Float64bits(y), uintptr(unsafe.Pointer(&output)))
	return hr, output
}
