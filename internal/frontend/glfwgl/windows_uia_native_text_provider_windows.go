//go:build glfw && windows

package glfwgl

import (
	"math"
	"unsafe"

	"cervterm/internal/accessibility"
)

//go:nocheckptr
func nativeUIATextGetSelection(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	owner := nativeUIAAvailableObject(this)
	frame, node, ok := nativeUIATextFrameNode(owner)
	if !ok {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	var ranges []*nativeUIATextRange
	span := accessibility.Span{}
	hasRange := false
	switch {
	case node.HasSelect:
		span, hasRange = node.Selection, true
	case node.HasCaret:
		span, hasRange = accessibility.Span{Start: node.Caret, End: node.Caret}, true
	}
	if hasRange {
		value, err := accessibility.NewRange(frame.document, owner.node, span.Start, span.End)
		if err != nil {
			return uiaHRESULTResult(uiaEElementNotAvailable)
		}
		rangeObject := newNativeUIATextRange(owner, value)
		if rangeObject == nil {
			return uiaHRESULTResult(uiaEOutOfMemory)
		}
		ranges = append(ranges, rangeObject)
	}
	array, hr := nativeUIAUnknownSafeArray(ranges)
	if hr == uiaSOK {
		*(*uintptr)(unsafe.Pointer(output)) = array
	}
	return uiaHRESULTResult(hr)
}

//go:nocheckptr
func nativeUIATextGetVisibleRanges(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	owner := nativeUIAAvailableObject(this)
	value, hr := newNativeUIATextDocumentRange(owner)
	if hr != uiaSOK {
		return uiaHRESULTResult(hr)
	}
	array, hr := nativeUIAUnknownSafeArray([]*nativeUIATextRange{value})
	if hr == uiaSOK {
		*(*uintptr)(unsafe.Pointer(output)) = array
	}
	return uiaHRESULTResult(hr)
}

//go:nocheckptr
func nativeUIATextRangeFromChild(this, _ uintptr, output uintptr) uintptr {
	return nativeUIATextUnsupportedRange(this, output)
}

//go:nocheckptr
func nativeUIATextRangeFromAnnotation(this, _ uintptr, output uintptr) uintptr {
	return nativeUIATextUnsupportedRange(this, output)
}

func nativeUIATextUnsupportedRange(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	if nativeUIAAvailableObject(this) == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	return uiaHRESULTResult(uiaENotSupported)
}

//go:nocheckptr
func nativeUIATextDocumentRange(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	value, hr := newNativeUIATextDocumentRange(nativeUIAAvailableObject(this))
	if hr == uiaSOK {
		*(*uintptr)(unsafe.Pointer(output)) = value.pointer
	}
	return uiaHRESULTResult(hr)
}

//go:nocheckptr
func nativeUIATextSupportedSelection(this, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	if nativeUIAAvailableObject(this) == nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	*(*int32)(unsafe.Pointer(output)) = 1
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextGetCaretRange(this, activeOutput, rangeOutput uintptr) uintptr {
	if activeOutput == 0 || rangeOutput == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*int32)(unsafe.Pointer(activeOutput)) = 0
	*(*uintptr)(unsafe.Pointer(rangeOutput)) = 0
	owner := nativeUIAAvailableObject(this)
	frame, node, ok := nativeUIATextFrameNode(owner)
	if !ok {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	if !node.HasCaret {
		return uiaHRESULTResult(uiaSOK)
	}
	value, err := accessibility.NewRange(frame.document, owner.node, node.Caret, node.Caret)
	if err != nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	rangeObject := newNativeUIATextRange(owner, value)
	if rangeObject == nil {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	if frame.document.Focus() == owner.node {
		*(*int32)(unsafe.Pointer(activeOutput)) = 1
	}
	*(*uintptr)(unsafe.Pointer(rangeOutput)) = rangeObject.pointer
	return uiaHRESULTResult(uiaSOK)
}

//go:nocheckptr
func nativeUIATextRangeFromPointBits(this uintptr, xBits, yBits uint64, output uintptr) uintptr {
	if output == 0 {
		return uiaHRESULTResult(uiaEPointer)
	}
	*(*uintptr)(unsafe.Pointer(output)) = 0
	owner := nativeUIAAvailableObject(this)
	frame, node, ok := nativeUIATextFrameNode(owner)
	if !ok || !frame.screenSpace {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	x, y := math.Float64frombits(xBits), math.Float64frombits(yBits)
	position, found := nativeUIATextPositionFromPoint(frame, node, x, y)
	if !found {
		return uiaHRESULTResult(uiaSOK)
	}
	value, err := accessibility.NewRange(frame.document, owner.node, position, position)
	if err != nil {
		return uiaHRESULTResult(uiaEElementNotAvailable)
	}
	rangeObject := newNativeUIATextRange(owner, value)
	if rangeObject == nil {
		return uiaHRESULTResult(uiaEOutOfMemory)
	}
	*(*uintptr)(unsafe.Pointer(output)) = rangeObject.pointer
	return uiaHRESULTResult(uiaSOK)
}

func nativeUIATextPositionFromPoint(frame *uiaPublishedDocument, node accessibility.NodeSnapshot, x, y float64) (int, bool) {
	bestPosition, bestDistance, found := 0, math.Inf(1), false
	for _, row := range node.Rows {
		for index, bounds := range row.Bounds {
			bounds.X += frame.screenX
			bounds.Y += frame.screenY
			nearestX := max(bounds.X, min(x, bounds.X+bounds.Width))
			nearestY := max(bounds.Y, min(y, bounds.Y+bounds.Height))
			distance := math.Hypot(x-nearestX, y-nearestY)
			position := row.StartGrapheme + index
			if x >= bounds.X+bounds.Width/2 {
				position++
			}
			if distance < bestDistance {
				bestPosition, bestDistance, found = position, distance, true
			}
		}
	}
	return bestPosition, found
}
