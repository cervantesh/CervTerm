//go:build glfw && windows

package glfwgl

import (
	"runtime"
	"unicode/utf16"
	"unsafe"

	"cervterm/internal/accessibility"
)

//go:nocheckptr
func nativeUIATextRangeQueryInterface(this, iidPointer, output uintptr) uintptr {
	if iidPointer == 0 || output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value := nativeUIATextRangeFromThis(this)
	if value == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	iid := *(*uiaGUID)(unsafe.Pointer(iidPointer))
	if iid != uiaIIDUnknown && iid != uiaIIDTextRangeProvider && iid != uiaIIDTextRangeProvider2 {
		return uiaHRESULTResult(uiaENoInterface)
	}
	if nativeUIATextRangeAddReference(value) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = value.pointer
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIATextRangeAddRef(this uintptr) uintptr {
	return uintptr(nativeUIATextRangeAddReference(nativeUIATextRangeFromThis(this)))
}

func nativeUIATextRangeRelease(this uintptr) uintptr {
	return uintptr(nativeUIATextRangeReleaseReference(nativeUIATextRangeFromThis(this)))
}

//go:nocheckptr
func nativeUIATextRangeClone(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value := nativeUIATextRangeFromThis(this)
	document, current, hr := nativeUIATextRangeSnapshot(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	if _, err := current.Text(document); err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	clone := newNativeUIATextRange(value.owner, current.Clone())
	if clone == nil {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	*(*uintptr)(unsafe.Pointer(output)) = clone.pointer
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeCompare(this, targetPointer, output uintptr) uintptr {
	if output == 0 || targetPointer == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*int32)(unsafe.Pointer(output)) = 0
	left := nativeUIATextRangeFromThis(this)
	right := nativeUIATextRangeFromThis(targetPointer)
	document, leftValue, rightValue, hr := nativeUIATextRangePair(left, right)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	equal, err := leftValue.Equal(document, rightValue)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	if equal {
		*(*int32)(unsafe.Pointer(output)) = 1
	}
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeCompareEndpoints(this, endpoint, targetPointer, targetEndpoint, output uintptr) uintptr {
	if output == 0 || targetPointer == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*int32)(unsafe.Pointer(output)) = 0
	if endpoint > 1 || targetEndpoint > 1 {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	document, leftValue, rightValue, hr := nativeUIATextRangePair(nativeUIATextRangeFromThis(this), nativeUIATextRangeFromThis(targetPointer))
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	comparison, err := accessibility.CompareEndpoints(document, leftValue, endpoint == 0, rightValue, targetEndpoint == 0)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	*(*int32)(unsafe.Pointer(output)) = int32(comparison)
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIATextRangeExpand(this, unit uintptr) uintptr {
	textUnit, ok := nativeUIATextUnit(unit)
	if !ok {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	value := nativeUIATextRangeFromThis(this)
	if value == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	document, hr := nativeUIATextRangeDocument(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	expanded, err := value.value.Expand(document, textUnit)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	value.value = expanded
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeFindAttribute(this, _ uintptr, _ uintptr, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	if _, _, hr := nativeUIATextRangeSnapshot(nativeUIATextRangeFromThis(this)); hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeFindText(this, bstr, backward, ignoreCase, output uintptr) uintptr {
	if output == 0 || bstr == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value := nativeUIATextRangeFromThis(this)
	document, current, hr := nativeUIATextRangeSnapshot(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	needle := nativeUIABSTRString(bstr)
	found, ok, err := current.FindText(document, needle, backward != 0, ignoreCase != 0)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	if !ok {
		return uiaHRESULTResult(uiaSOK)
	}
	result := newNativeUIATextRange(value.owner, found)
	if result == nil {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	*(*uintptr)(unsafe.Pointer(output)) = result.pointer
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeGetAttribute(this, _ uintptr, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	variant := (*nativeUIAVariant)(unsafe.Pointer(output))
	*variant = nativeUIAVariant{}
	if _, _, hr := nativeUIATextRangeSnapshot(nativeUIATextRangeFromThis(this)); hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	var reserved uintptr
	hr, _, _ := uiaReservedNotSupported.Call(uintptr(unsafe.Pointer(&reserved)))
	if uiaHRESULT(int32(hr)) != uiaSOK {
		return uintptr(uint32(hr))
	}
	variant.VT, variant.Value = uiaVTUnknown, reserved
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeGetBoundingRectangles(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value := nativeUIATextRangeFromThis(this)
	_, current, hr := nativeUIATextRangeSnapshot(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	frame, ok := value.owner.provider.root.publication.SnapshotFrame()
	if !ok || !frame.screenSpace {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	rectangles, err := current.Rectangles(frame.document)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	values := make([]float64, 0, len(rectangles)*4)
	for _, rect := range rectangles {
		values = append(values, rect.X+frame.screenX, rect.Y+frame.screenY, rect.Width, rect.Height)
	}
	array, arrayHR := nativeUIADoubleSafeArray(values)
	if arrayHR == uiaSOK {
		*(*uintptr)(unsafe.Pointer(output)) = array
	}
	return uiaHRESULTResult(arrayHR)
}

//go:nocheckptr
func nativeUIATextRangeGetEnclosingElement(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value := nativeUIATextRangeFromThis(this)
	if _, _, hr := nativeUIATextRangeSnapshot(value); hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	if nativeUIARetain(value.owner) == 0 {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*uintptr)(unsafe.Pointer(output)) = value.owner.simple
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeGetText(this, maxLength, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	limit := int32(maxLength)
	if limit < -1 {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	document, current, hr := nativeUIATextRangeSnapshot(nativeUIATextRangeFromThis(this))
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	text, err := current.Text(document)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	units := utf16.Encode([]rune(text))
	if limit >= 0 && len(units) > int(limit) {
		units = units[:limit]
		if len(units) != 0 && units[len(units)-1] >= 0xd800 && units[len(units)-1] <= 0xdbff {
			units = units[:len(units)-1]
		}
	}
	units = append([]uint16(nil), units...)
	var source uintptr
	if len(units) != 0 {
		source = uintptr(unsafe.Pointer(&units[0]))
	}
	bstr, _, _ := uiaSysAllocStringLen.Call(source, uintptr(len(units)))
	runtime.KeepAlive(units)
	if bstr == 0 {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	*(*uintptr)(unsafe.Pointer(output)) = bstr
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeMove(this, unit, count, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*int32)(unsafe.Pointer(output)) = 0
	textUnit, ok := nativeUIATextUnit(unit)
	if !ok {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	value := nativeUIATextRangeFromThis(this)
	if value == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	document, hr := nativeUIATextRangeDocument(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	moved, actual, err := value.value.Move(document, textUnit, int(int32(count)))
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	value.value = moved
	*(*int32)(unsafe.Pointer(output)) = int32(actual)
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeMoveEndpointByUnit(this, endpoint, unit, count, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*int32)(unsafe.Pointer(output)) = 0
	if endpoint > 1 {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	textUnit, ok := nativeUIATextUnit(unit)
	if !ok {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	value := nativeUIATextRangeFromThis(this)
	if value == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	document, hr := nativeUIATextRangeDocument(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	moved, actual, err := value.value.MoveEndpoint(document, endpoint == 0, textUnit, int(int32(count)))
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	value.value = moved
	*(*int32)(unsafe.Pointer(output)) = int32(actual)
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIATextRangeMoveEndpointByRange(this, endpoint, targetPointer, targetEndpoint uintptr) uintptr {
	if endpoint > 1 || targetEndpoint > 1 || targetPointer == 0 {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	value := nativeUIATextRangeFromThis(this)
	target := nativeUIATextRangeFromThis(targetPointer)
	if value == nil || target == nil || value.owner.provider != target.owner.provider {
		return uiaHRESULTResult(uiaEInvalidArg)
	}
	target.mu.Lock()
	targetValue := target.value
	target.mu.Unlock()
	value.mu.Lock()
	defer value.mu.Unlock()
	document, hr := nativeUIATextRangeDocument(value)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	moved, err := value.value.MoveEndpointTo(document, endpoint == 0, targetValue, targetEndpoint == 0)
	if err != nil {
		return uiaHRESULTResult(nativeUIATextRangeHRESULT(err))
	}
	value.value = moved
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIATextRangeUnsupported(this uintptr) uintptr {
	if _, _, hr := nativeUIATextRangeSnapshot(nativeUIATextRangeFromThis(this)); hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	return uiaHRESULTResult(uiaENotSupported)
}

func nativeUIATextRangeScrollUnsupported(this, _ uintptr) uintptr {
	return nativeUIATextRangeUnsupported(this)
}

//go:nocheckptr
func nativeUIATextRangeGetChildren(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	if _, _, hr := nativeUIATextRangeSnapshot(nativeUIATextRangeFromThis(this)); hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	array, hr := nativeUIAUnknownSafeArray(nil)
	if hr == uiaSOK {
		*(*uintptr)(unsafe.Pointer(output)) = array
	}
	return uiaHRESULTResult(hr)
}

func nativeUIATextRangeSnapshot(value *nativeUIATextRange) (accessibility.Document, accessibility.Range, uiaHRESULT) {
	if value == nil || value.owner == nil {
		return accessibility.Document{}, accessibility.Range{}, uiaEElementNotAvailable
	}
	value.mu.Lock()
	current := value.value
	value.mu.Unlock()
	document, hr := nativeUIATextRangeDocument(value)
	if hr != uiaSOK {
		return accessibility.Document{}, accessibility.Range{}, hr
	}
	if _, err := current.Text(document); err != nil {
		return accessibility.Document{}, accessibility.Range{}, nativeUIATextRangeHRESULT(err)
	}
	return document, current, uiaSOK
}

func nativeUIATextRangeDocument(value *nativeUIATextRange) (accessibility.Document, uiaHRESULT) {
	if value == nil || value.owner == nil || value.owner.provider == nil || !value.owner.provider.root.available() {
		return accessibility.Document{}, uiaEElementNotAvailable
	}
	frame, ok := value.owner.provider.root.publication.SnapshotFrame()
	if !ok {
		return accessibility.Document{}, uiaEElementNotAvailable
	}
	return frame.document, uiaSOK
}

func nativeUIATextRangePair(left, right *nativeUIATextRange) (accessibility.Document, accessibility.Range, accessibility.Range, uiaHRESULT) {
	if left == nil || right == nil || left.owner == nil || right.owner == nil || left.owner.provider != right.owner.provider {
		return accessibility.Document{}, accessibility.Range{}, accessibility.Range{}, uiaEInvalidArg
	}
	_, leftValue, hr := nativeUIATextRangeSnapshot(left)
	if hr != uiaSOK {
		return accessibility.Document{}, accessibility.Range{}, accessibility.Range{}, hr
	}
	document, rightValue, hr := nativeUIATextRangeSnapshot(right)
	return document, leftValue, rightValue, hr
}

func nativeUIABSTRString(value uintptr) string {
	if value == 0 {
		return ""
	}
	length, _, _ := uiaSysStringLen.Call(value)
	units := unsafe.Slice((*uint16)(unsafe.Pointer(value)), int(length))
	return string(utf16.Decode(units))
}

func nativeUIATextUnit(value uintptr) (accessibility.TextUnit, bool) {
	signed := int32(value)
	if signed < int32(accessibility.TextUnitCharacter) || signed > int32(accessibility.TextUnitDocument) {
		return 0, false
	}
	return accessibility.TextUnit(signed), true
}
