//go:build glfw && windows

package glfwgl

import (
	"math"
	"unsafe"
)

//go:nocheckptr
func nativeUIAEmbeddedRoots(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	if nativeUIAAvailableObject(this) == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIASetFocus(this uintptr) uintptr {
	if nativeUIAAvailableObject(this) == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	return uiaHRESULTResult(uiaENotSupported)
}

//go:nocheckptr
func nativeUIAFragmentRoot(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil || object.provider.rootObject == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if nativeUIARetain(object.provider.rootObject) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = object.provider.rootObject.fragment
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIAElementFromPointBits(this uintptr, xBits, yBits uint64, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	x, y := math.Float64frombits(xBits), math.Float64frombits(yBits)
	frame, ok := object.provider.root.publication.SnapshotFrame()
	if !ok || !frame.screenSpace {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	nodes := frame.document.Nodes()
	var target *nativeUIAObject
	for index := len(nodes) - 1; index >= 0; index-- {
		rect := uiaFrameNodeBounds(frame, nodes[index].ID)
		if rect.Width > 0 && rect.Height > 0 && x >= rect.X && x < rect.X+rect.Width && y >= rect.Y && y < rect.Y+rect.Height {
			target = object.provider.object(nodes[index].ID)
			break
		}
	}
	if target == nil {
		return uiaHRESULTResult(uiaSOK)
	}
	if nativeUIARetain(target) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = target.fragment
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIAGetFocus(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	object := nativeUIAAvailableObject(this)
	if object == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	id, hr := object.provider.root.Focus()
	if hr != uiaSOK || !id.Valid() {
		return uiaHRESULTResult(hr)
	}
	target := object.provider.object(id)
	if target == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if nativeUIARetain(target) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = target.fragment
	return uiaHRESULTResult(uiaSOK)
}
