//go:build glfw && windows && cgo

package glfwgl

/*
#include <windows.h>
#include <stdint.h>
extern uintptr_t cervtermGoUIAElementFromPoint(uintptr_t self, uint64_t x_bits, uint64_t y_bits, uintptr_t output);

static HRESULT STDMETHODCALLTYPE cervtermUIAElementFromPoint(void *self, double x, double y, void **output) {
    union { double value; uint64_t bits; } x_value = { x }, y_value = { y };
    return (HRESULT)cervtermGoUIAElementFromPoint((uintptr_t)self, x_value.bits, y_value.bits, (uintptr_t)output);
}

static uintptr_t cervtermUIAElementFromPointCallback(void) {
	return (uintptr_t)&cervtermUIAElementFromPoint;
}

static HRESULT cervtermTestUIAElementFromPoint(uintptr_t callback, void *self, double x, double y, void **output) {
    typedef HRESULT (STDMETHODCALLTYPE *element_from_point_fn)(void *, double, double, void **);
    return ((element_from_point_fn)callback)(self, x, y, output);
}
*/
import "C"
import "unsafe"

//export cervtermGoUIAElementFromPoint
func cervtermGoUIAElementFromPoint(self C.uintptr_t, xBits C.uint64_t, yBits C.uint64_t, output C.uintptr_t) C.uintptr_t {
	return C.uintptr_t(nativeUIAElementFromPointBits(uintptr(self), uint64(xBits), uint64(yBits), uintptr(output)))
}

func nativeUIAElementFromPointCallback() uintptr {
	return uintptr(C.cervtermUIAElementFromPointCallback())
}

func testNativeUIAElementFromPoint(callback, self uintptr, x, y float64) (uintptr, uintptr) {
	var output unsafe.Pointer
	hr := C.cervtermTestUIAElementFromPoint(C.uintptr_t(callback), unsafe.Pointer(self), C.double(x), C.double(y), &output)
	return uintptr(uint32(hr)), uintptr(output)
}
