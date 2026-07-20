//go:build glfw && windows && cgo

package glfwgl

/*
#include <windows.h>
#include <stdint.h>
#include <UIAutomationCore.h>

extern uintptr_t cervtermGoUIATextRangeFromPoint(uintptr_t self, uint64_t x_bits, uint64_t y_bits, uintptr_t output);
extern uintptr_t cervtermGoUIATextFindAttribute(uintptr_t self, int32_t attribute, uintptr_t backward, uintptr_t output);

static HRESULT STDMETHODCALLTYPE cervtermUIATextRangeFromPoint(void *self, struct UiaPoint point, void **output) {
    union { double value; uint64_t bits; } x_value = { point.x }, y_value = { point.y };
    return (HRESULT)cervtermGoUIATextRangeFromPoint((uintptr_t)self, x_value.bits, y_value.bits, (uintptr_t)output);
}

static HRESULT STDMETHODCALLTYPE cervtermUIATextFindAttribute(void *self, int32_t attribute, VARIANT value, BOOL backward, void **output) {
    (void)value;
    return (HRESULT)cervtermGoUIATextFindAttribute((uintptr_t)self, attribute, (uintptr_t)backward, (uintptr_t)output);
}

static uintptr_t cervtermUIATextRangeFromPointCallback(void) { return (uintptr_t)&cervtermUIATextRangeFromPoint; }
static uintptr_t cervtermUIATextFindAttributeCallback(void) { return (uintptr_t)&cervtermUIATextFindAttribute; }

static HRESULT cervtermTestUIATextRangeFromPoint(uintptr_t callback, void *self, double x, double y, void **output) {
    typedef HRESULT (STDMETHODCALLTYPE *callback_fn)(void *, struct UiaPoint, void **);
    struct UiaPoint point = { x, y };
    return ((callback_fn)callback)(self, point, output);
}

static HRESULT cervtermTestUIATextFindAttribute(uintptr_t callback, void *self, int32_t attribute, BOOL backward, void **output) {
    typedef HRESULT (STDMETHODCALLTYPE *callback_fn)(void *, int32_t, VARIANT, BOOL, void **);
    VARIANT value = {0};
    return ((callback_fn)callback)(self, attribute, value, backward, output);
}
*/
import "C"
import "unsafe"

//export cervtermGoUIATextRangeFromPoint
func cervtermGoUIATextRangeFromPoint(self C.uintptr_t, xBits C.uint64_t, yBits C.uint64_t, output C.uintptr_t) C.uintptr_t {
	return C.uintptr_t(nativeUIATextRangeFromPointBits(uintptr(self), uint64(xBits), uint64(yBits), uintptr(output)))
}

//export cervtermGoUIATextFindAttribute
func cervtermGoUIATextFindAttribute(self C.uintptr_t, attribute C.int32_t, backward C.uintptr_t, output C.uintptr_t) C.uintptr_t {
	return C.uintptr_t(nativeUIATextRangeFindAttribute(uintptr(self), uintptr(uint32(attribute)), uintptr(backward), uintptr(output)))
}

func nativeUIATextRangeFromPointCallback() uintptr {
	return uintptr(C.cervtermUIATextRangeFromPointCallback())
}
func nativeUIATextFindAttributeCallback() uintptr {
	return uintptr(C.cervtermUIATextFindAttributeCallback())
}

func testNativeUIATextRangeFromPoint(callback, self uintptr, x, y float64) (uintptr, uintptr) {
	var output unsafe.Pointer
	hr := C.cervtermTestUIATextRangeFromPoint(C.uintptr_t(callback), unsafe.Pointer(self), C.double(x), C.double(y), &output)
	return uintptr(uint32(hr)), uintptr(output)
}

func testNativeUIATextFindAttribute(callback, self uintptr) (uintptr, uintptr) {
	var output unsafe.Pointer
	hr := C.cervtermTestUIATextFindAttribute(C.uintptr_t(callback), unsafe.Pointer(self), 1, 0, &output)
	return uintptr(uint32(hr)), uintptr(output)
}
