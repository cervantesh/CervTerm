//go:build glfw && windows

package glfwgl

import (
	"errors"
	"math"
	"unsafe"

	"cervterm/internal/accessibility"
)

func (provider *nativeUIAProvider) textInterface(object *nativeUIAObject) uintptr {
	if provider == nil || object == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.objects == nil {
		return 0
	}
	if object.text != 0 {
		return object.text
	}
	if !object.textCapable {
		return 0
	}
	pointer := allocateNativeUIAInterface(uintptr(unsafe.Pointer(&uiaTextProviderVTable)))
	if pointer == 0 {
		return 0
	}
	object.text = pointer
	nativeUIAInterfaces.Store(pointer, object)
	return pointer
}

func nativeUIATextCapable(node accessibility.NodeSnapshot) bool {
	return len(node.Rows) != 0 || node.HasCaret || node.HasSelect
}

func nativeUIANodeTextLength(node accessibility.NodeSnapshot) int {
	if len(node.Rows) == 0 {
		return 0
	}
	return node.Rows[len(node.Rows)-1].EndGrapheme
}

func newNativeUIATextRange(owner *nativeUIAObject, value accessibility.Range) *nativeUIATextRange {
	if owner == nil || owner.provider == nil {
		return nil
	}
	provider := owner.provider
	for {
		count := provider.textRanges.Load()
		if count >= maxNativeUIARanges || count < 0 {
			return nil
		}
		if provider.textRanges.CompareAndSwap(count, count+1) {
			break
		}
	}
	if nativeUIARetain(owner) == 0 {
		provider.textRanges.Add(-1)
		return nil
	}
	pointer := allocateNativeUIAInterface(uintptr(unsafe.Pointer(&uiaTextRangeVTable)))
	if pointer == 0 {
		nativeUIAReleaseObject(owner)
		provider.textRanges.Add(-1)
		return nil
	}
	result := &nativeUIATextRange{pointer: pointer, owner: owner, value: value}
	result.refs.Store(1)
	nativeUIATextRanges.Store(pointer, result)
	return result
}

func nativeUIATextRangeFromThis(this uintptr) *nativeUIATextRange {
	if this == 0 {
		return nil
	}
	value, ok := nativeUIATextRanges.Load(this)
	if !ok {
		return nil
	}
	result, _ := value.(*nativeUIATextRange)
	return result
}

func nativeUIATextRangeAddReference(value *nativeUIATextRange) uint32 {
	if value == nil {
		return 0
	}
	for {
		current := value.refs.Load()
		if current == 0 || current == math.MaxUint32 {
			return current
		}
		if value.refs.CompareAndSwap(current, current+1) {
			return current + 1
		}
	}
}

func nativeUIATextRangeReleaseReference(value *nativeUIATextRange) uint32 {
	if value == nil {
		return 0
	}
	for {
		current := value.refs.Load()
		if current == 0 || current == math.MaxUint32 {
			return current
		}
		if !value.refs.CompareAndSwap(current, current-1) {
			continue
		}
		if current != 1 {
			return current - 1
		}
		nativeUIATextRanges.Delete(value.pointer)
		uiaGlobalFree.Call(value.pointer)
		owner := value.owner
		value.owner = nil
		owner.provider.textRanges.Add(-1)
		nativeUIAReleaseObject(owner)
		return 0
	}
}

func newNativeUIATextDocumentRange(owner *nativeUIAObject) (*nativeUIATextRange, uiaHRESULT) {
	frame, node, ok := nativeUIATextFrameNode(owner)
	if !ok {
		return nil, uiaEElementNotAvailable
	}
	value, err := accessibility.NewRange(frame.document, owner.node, 0, nativeUIANodeTextLength(node))
	if err != nil {
		return nil, uiaEElementNotAvailable
	}
	result := newNativeUIATextRange(owner, value)
	if result == nil {
		return nil, uiaEOutOfMemory
	}
	return result, uiaSOK
}

func nativeUIATextFrameNode(owner *nativeUIAObject) (*uiaPublishedDocument, accessibility.NodeSnapshot, bool) {
	if owner == nil || owner.provider == nil || !owner.provider.root.available() {
		return nil, accessibility.NodeSnapshot{}, false
	}
	frame, ok := owner.provider.root.publication.SnapshotFrame()
	if !ok {
		return nil, accessibility.NodeSnapshot{}, false
	}
	node, ok := frame.document.Node(owner.node)
	return frame, node, ok && nativeUIATextCapable(node)
}

func nativeUIATextRangeHRESULT(err error) uiaHRESULT {
	switch {
	case err == nil:
		return uiaSOK
	case errors.Is(err, accessibility.ErrStaleRange):
		return uiaEElementNotAvailable
	default:
		return uiaEInvalidArg
	}
}

func nativeUIAUnknownSafeArray(ranges []*nativeUIATextRange) (uintptr, uiaHRESULT) {
	array, _, _ := uiaSafeArrayCreateVector.Call(uiaVTUnknown, 0, uintptr(len(ranges)))
	if array == 0 {
		for _, value := range ranges {
			nativeUIATextRangeReleaseReference(value)
		}
		return 0, uiaEOutOfMemory
	}
	if len(ranges) == 0 {
		return array, uiaSOK
	}
	var data uintptr
	hr, _, _ := uiaSafeArrayAccessData.Call(array, uintptr(unsafe.Pointer(&data)))
	if uiaHRESULT(int32(hr)) != uiaSOK || data == 0 {
		uiaSafeArrayDestroy.Call(array)
		for _, value := range ranges {
			nativeUIATextRangeReleaseReference(value)
		}
		if uiaHRESULT(int32(hr)) != uiaSOK {
			return 0, uiaHRESULT(int32(hr))
		}
		return 0, uiaEElementNotAvailable
	}
	pointers := unsafe.Slice((*uintptr)(unsafe.Pointer(data)), len(ranges))
	for index, value := range ranges {
		pointers[index] = value.pointer
	}
	unaccess, _, _ := uiaSafeArrayUnaccessData.Call(array)
	if uiaHRESULT(int32(unaccess)) != uiaSOK {
		uiaSafeArrayDestroy.Call(array)
		return 0, uiaHRESULT(int32(unaccess))
	}
	return array, uiaSOK
}
