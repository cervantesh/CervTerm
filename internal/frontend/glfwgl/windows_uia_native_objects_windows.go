//go:build glfw && windows

package glfwgl

import (
	"math"
	"unsafe"

	"cervterm/internal/accessibility"
)

func (provider *nativeUIAProvider) Close() {
	if provider == nil {
		return
	}
	provider.closeOnce.Do(func() {
		provider.root.Disconnect()
		provider.root.Release()
	})
}

func (provider *nativeUIAProvider) finalize() {
	if provider == nil {
		return
	}
	provider.finalizeOnce.Do(func() {
		provider.root.nativePointer.CompareAndSwap(provider.pointer, 0)
		provider.mu.Lock()
		objects := provider.objects
		provider.objects = nil
		provider.rootObject = nil
		provider.mu.Unlock()
		for _, object := range objects {
			freeNativeUIAObject(object)
		}
	})
}

func (provider *nativeUIAProvider) object(id accessibility.NodeID) *nativeUIAObject {
	if provider == nil || !id.Valid() {
		return nil
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.objects == nil {
		return nil
	}
	return provider.objectLocked(id)
}

func (provider *nativeUIAProvider) objectLocked(id accessibility.NodeID) *nativeUIAObject {
	if object := provider.objects[id]; object != nil {
		return object
	}
	frame, ok := provider.root.publication.SnapshotFrame()
	if !ok {
		return nil
	}
	node, ok := frame.document.Node(id)
	if !ok {
		return nil
	}
	provider.pruneObjectsLocked(frame)
	if len(provider.objects) >= maxNativeUIAObjects {
		return nil
	}
	fragment := allocateNativeUIAInterface(uintptr(unsafe.Pointer(&uiaFragmentVTable)))
	if fragment == 0 {
		return nil
	}
	simple := allocateNativeUIAInterface(uintptr(unsafe.Pointer(&uiaSimpleVTable)))
	if simple == 0 {
		uiaGlobalFree.Call(fragment)
		return nil
	}
	object := &nativeUIAObject{fragment: fragment, simple: simple, provider: provider, node: id, textCapable: nativeUIATextCapable(node)}
	provider.objects[id] = object
	nativeUIAInterfaces.Store(fragment, object)
	nativeUIAInterfaces.Store(simple, object)
	return object
}

func (provider *nativeUIAProvider) pruneObjectsLocked(frame *uiaPublishedDocument) {
	for id, object := range provider.objects {
		if object == provider.rootObject || object.refs.Load() != 0 {
			continue
		}
		if _, current := frame.document.Node(id); current {
			continue
		}
		delete(provider.objects, id)
		freeNativeUIAObject(object)
	}
}

func freeNativeUIAObject(object *nativeUIAObject) {
	if object == nil {
		return
	}
	for _, pointer := range []uintptr{object.fragment, object.simple, object.fragmentRoot, object.text} {
		if pointer != 0 {
			nativeUIAInterfaces.Delete(pointer)
			uiaGlobalFree.Call(pointer)
		}
	}
}

func allocateNativeUIAInterface(vtable uintptr) uintptr {
	pointer, _, _ := uiaGlobalAlloc.Call(0x40, unsafe.Sizeof(nativeUIAInterface{}))
	if pointer != 0 {
		(*nativeUIAInterface)(unsafe.Pointer(pointer)).VTable = vtable
	}
	return pointer
}

func nativeUIAObjectFromThis(this uintptr) *nativeUIAObject {
	if this == 0 {
		return nil
	}
	value, ok := nativeUIAInterfaces.Load(this)
	if !ok {
		return nil
	}
	object, _ := value.(*nativeUIAObject)
	return object
}

func nativeUIAAvailableObject(this uintptr) *nativeUIAObject {
	object := nativeUIAObjectFromThis(this)
	if object == nil || object.provider == nil || !object.provider.root.available() {
		return nil
	}
	if _, _, ok := object.provider.root.frameNode(object.node); !ok {
		return nil
	}
	return object
}

func nativeUIARetain(object *nativeUIAObject) uint32 {
	if object == nil || object.provider == nil {
		return 0
	}
	for {
		current := object.refs.Load()
		if current == math.MaxUint32 {
			return current
		}
		if object.provider.root.AddRef() == 0 {
			return 0
		}
		if object.refs.CompareAndSwap(current, current+1) {
			return current + 1
		}
		object.provider.root.Release()
	}
}

func nativeUIAReleaseObject(object *nativeUIAObject) uint32 {
	if object == nil || object.provider == nil {
		return 0
	}
	for {
		current := object.refs.Load()
		if current == 0 || current == math.MaxUint32 {
			return current
		}
		if object.refs.CompareAndSwap(current, current-1) {
			object.provider.root.Release()
			return current - 1
		}
	}
}

func nativeUIARectSafeArray(rect accessibility.Rect) (uintptr, uiaHRESULT) {
	return nativeUIADoubleSafeArray([]float64{rect.X, rect.Y, rect.Width, rect.Height})
}

func nativeUIADoubleSafeArray(values []float64) (uintptr, uiaHRESULT) {
	array, _, _ := uiaSafeArrayCreateVector.Call(uiaVTR8, 0, uintptr(len(values)))
	if array == 0 {
		return 0, uiaEOutOfMemory
	}
	if len(values) == 0 {
		return array, uiaSOK
	}
	var data uintptr
	hr, _, _ := uiaSafeArrayAccessData.Call(array, uintptr(unsafe.Pointer(&data)))
	if uiaHRESULT(int32(hr)) != uiaSOK || data == 0 {
		uiaSafeArrayDestroy.Call(array)
		if uiaHRESULT(int32(hr)) != uiaSOK {
			return 0, uiaHRESULT(int32(hr))
		}
		return 0, uiaEElementNotAvailable
	}
	copy(unsafe.Slice((*float64)(unsafe.Pointer(data)), len(values)), values)
	unaccess, _, _ := uiaSafeArrayUnaccessData.Call(array)
	if uiaHRESULT(int32(unaccess)) != uiaSOK {
		uiaSafeArrayDestroy.Call(array)
		return 0, uiaHRESULT(int32(unaccess))
	}
	return array, uiaSOK
}
