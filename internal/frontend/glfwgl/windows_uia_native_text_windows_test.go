//go:build glfw && windows

package glfwgl

import (
	"testing"
	"unicode/utf16"
	"unsafe"

	"cervterm/internal/accessibility"
)

func TestNativeUIATextABIAndPatternIdentity(t *testing.T) {
	pointer := unsafe.Sizeof(uintptr(0))
	if unsafe.Sizeof(nativeUIATextProvider2VTable{}) != 11*pointer || unsafe.Sizeof(nativeUIATextRangeProvider2VTable{}) != 22*pointer {
		t.Fatalf("pointer=%d provider=%d range=%d", pointer, unsafe.Sizeof(nativeUIATextProvider2VTable{}), unsafe.Sizeof(nativeUIATextRangeProvider2VTable{}))
	}
	if unsafe.Offsetof(nativeUIATextProvider2VTable{}.GetCaretRange) != 10*pointer || unsafe.Offsetof(nativeUIATextRangeProvider2VTable{}.ShowContextMenu) != 21*pointer {
		t.Fatal("unexpected text vtable offsets")
	}
	if uiaIIDTextProvider.Data1 != 0x3589c92c || uiaIIDTextProvider2.Data1 != 0x0dc5e6ed || uiaIIDTextRangeProvider.Data1 != 0x5347ad7b || uiaIIDTextRangeProvider2.Data1 != 0x9bbce42c {
		t.Fatal("unexpected text IIDs")
	}
	root, publication, _, rootID, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	pane := native.object(paneID)
	var text uintptr
	if hr := nativeUIAGetPatternProvider(pane.simple, uiaTextPatternID, uintptr(unsafe.Pointer(&text))); hr != uiaHRESULTResult(uiaSOK) || text == 0 || text != pane.text {
		t.Fatalf("text pattern hr=%#x text=%#x", hr, text)
	}
	var output uintptr
	iid := uiaIIDTextProvider2
	if hr := nativeUIAQueryInterface(text, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != text {
		t.Fatalf("provider2 QI hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	iid = uiaIIDUnknown
	output = 0
	if hr := nativeUIAQueryInterface(text, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != pane.fragment {
		t.Fatalf("canonical IUnknown hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	output = 99
	if hr := nativeUIAGetPatternProvider(native.rootObject.simple, uiaTextPattern2ID, uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != 0 {
		t.Fatalf("non-text pattern hr=%#x output=%#x", hr, output)
	}
	next, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 2, Focus: paneID, Nodes: []accessibility.NodeDraft{
		{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm", Rows: []accessibility.RowDraft{{Text: "title", Bounds: []accessibility.Rect{{Width: 8, Height: 16}, {X: 8, Width: 8, Height: 16}, {X: 16, Width: 8, Height: 16}, {X: 24, Width: 8, Height: 16}, {X: 32, Width: 8, Height: 16}}}}},
		{ID: paneID, Parent: rootID, Role: accessibility.RoleTerminal, Name: "terminal"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.PublishScreen(next, 100, 200, testUIAWindowBounds()); err != nil {
		t.Fatal(err)
	}
	iid = uiaIIDTextProvider
	output = 0
	if hr := nativeUIAQueryInterface(pane.fragment, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != text {
		t.Fatalf("text interface disappeared hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	output = 99
	if hr := nativeUIAQueryInterface(native.rootObject.fragment, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaENoInterface) || output != 0 {
		t.Fatalf("text interface appeared hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(text)
	native.Close()
	root.Release()
}

func TestNativeUIATextRangesMovementTextBoundsAndStaleness(t *testing.T) {
	root, publication, _, _, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	pane := native.object(paneID)
	text := pane.provider.textInterface(pane)
	var documentRange uintptr
	if hr := nativeUIATextDocumentRange(text, uintptr(unsafe.Pointer(&documentRange))); hr != uiaHRESULTResult(uiaSOK) || documentRange == 0 {
		t.Fatalf("document range hr=%#x range=%#x", hr, documentRange)
	}
	if got := testNativeUIATextRangeString(t, documentRange, -1); got != "ok" {
		t.Fatalf("text=%q", got)
	}
	if got := testNativeUIATextRangeString(t, documentRange, 1); got != "o" {
		t.Fatalf("limited text=%q", got)
	}
	needle := testNativeUIAAllocBSTR(t, "O")
	var found uintptr
	if hr := nativeUIATextRangeFindText(documentRange, needle, 0, 1, uintptr(unsafe.Pointer(&found))); hr != uiaHRESULTResult(uiaSOK) || found == 0 || testNativeUIATextRangeString(t, found, -1) != "o" {
		t.Fatalf("find text hr=%#x found=%#x", hr, found)
	}
	uiaSysFreeString.Call(needle)
	nativeUIATextRangeRelease(found)
	if hr, output := testNativeUIATextFindAttribute(uiaTextRangeVTable.FindAttribute, documentRange); hr != uiaHRESULTResult(uiaSOK) || output != 0 {
		t.Fatalf("find attribute hr=%#x output=%#x", hr, output)
	}
	var attribute nativeUIAVariant
	if hr := nativeUIATextRangeGetAttribute(documentRange, 1, uintptr(unsafe.Pointer(&attribute))); hr != uiaHRESULTResult(uiaSOK) || attribute.VT != uiaVTUnknown || attribute.Value == 0 {
		t.Fatalf("attribute hr=%#x value=%#v", hr, attribute)
	}
	uiaVariantClear.Call(uintptr(unsafe.Pointer(&attribute)))
	var clone uintptr
	if hr := nativeUIATextRangeClone(documentRange, uintptr(unsafe.Pointer(&clone))); hr != uiaHRESULTResult(uiaSOK) || clone == 0 {
		t.Fatalf("clone hr=%#x clone=%#x", hr, clone)
	}
	var equal int32
	if hr := nativeUIATextRangeCompare(documentRange, clone, uintptr(unsafe.Pointer(&equal))); hr != uiaHRESULTResult(uiaSOK) || equal != 1 {
		t.Fatalf("compare hr=%#x equal=%d", hr, equal)
	}
	if hr := nativeUIATextRangeExpand(clone, uintptr(0)); hr != uiaHRESULTResult(uiaSOK) || testNativeUIATextRangeString(t, clone, -1) != "o" {
		t.Fatalf("expand hr=%#x", hr)
	}
	var moved int32
	if hr := nativeUIATextRangeMoveEndpointByUnit(clone, 1, uintptr(0), 1, uintptr(unsafe.Pointer(&moved))); hr != uiaHRESULTResult(uiaSOK) || moved != 1 || testNativeUIATextRangeString(t, clone, -1) != "ok" {
		t.Fatalf("move endpoint hr=%#x moved=%d", hr, moved)
	}
	var rectangles uintptr
	if hr := nativeUIATextRangeGetBoundingRectangles(documentRange, uintptr(unsafe.Pointer(&rectangles))); hr != uiaHRESULTResult(uiaSOK) || rectangles == 0 {
		t.Fatalf("rectangles hr=%#x array=%#x", hr, rectangles)
	}
	values := testNativeUIADoubleArray(t, rectangles, 8)
	want := []float64{110, 220, 8, 16, 118, 220, 8, 16}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("rectangles=%v", values)
		}
	}
	uiaSafeArrayDestroy.Call(rectangles)
	var enclosing uintptr
	if hr := nativeUIATextRangeGetEnclosingElement(documentRange, uintptr(unsafe.Pointer(&enclosing))); hr != uiaHRESULTResult(uiaSOK) || enclosing != pane.simple {
		t.Fatalf("enclosing hr=%#x output=%#x", hr, enclosing)
	}
	nativeUIARelease(enclosing)
	for _, hr := range []uintptr{nativeUIATextRangeUnsupported(documentRange), nativeUIATextRangeScrollUnsupported(documentRange, 1)} {
		if hr != uiaHRESULTResult(uiaENotSupported) {
			t.Fatalf("mutation hr=%#x", hr)
		}
	}
	next, _, _ := uiaTestDocument(t, 2)
	if err := publication.PublishScreen(next, 100, 200, testUIAWindowBounds()); err != nil {
		t.Fatal(err)
	}
	var staleText uintptr
	if hr := nativeUIATextRangeGetText(documentRange, ^uintptr(0), uintptr(unsafe.Pointer(&staleText))); hr != uiaHRESULTResult(uiaEElementNotAvailable) || staleText != 0 {
		t.Fatalf("stale hr=%#x output=%#x", hr, staleText)
	}
	nativeUIATextRangeRelease(clone)
	nativeUIATextRangeRelease(documentRange)
	native.Close()
	root.Release()
}

func TestNativeUIATextSelectionCaretVisiblePointAndArrayOwnership(t *testing.T) {
	root, _, _, _, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	pane := native.object(paneID)
	text := pane.provider.textInterface(pane)
	for _, callback := range []func(uintptr, uintptr) uintptr{nativeUIATextGetSelection, nativeUIATextGetVisibleRanges} {
		var array uintptr
		if hr := callback(text, uintptr(unsafe.Pointer(&array))); hr != uiaHRESULTResult(uiaSOK) || array == 0 {
			t.Fatalf("array hr=%#x array=%#x", hr, array)
		}
		if native.textRanges.Load() != 1 {
			t.Fatalf("range count=%d", native.textRanges.Load())
		}
		uiaSafeArrayDestroy.Call(array)
		if native.textRanges.Load() != 0 {
			t.Fatalf("array did not release range: %d", native.textRanges.Load())
		}
	}
	var active int32
	var caret uintptr
	if hr := nativeUIATextGetCaretRange(text, uintptr(unsafe.Pointer(&active)), uintptr(unsafe.Pointer(&caret))); hr != uiaHRESULTResult(uiaSOK) || active != 1 || caret == 0 {
		t.Fatalf("caret hr=%#x active=%d range=%#x", hr, active, caret)
	}
	if span := nativeUIATextRangeFromThis(caret).value.Span(); span.Start != 1 || span.End != 1 {
		t.Fatalf("caret span=%#v", span)
	}
	nativeUIATextRangeRelease(caret)
	hr, pointRange := testNativeUIATextRangeFromPoint(uiaTextProviderVTable.RangeFromPoint, text, 111, 221)
	if hr != uiaHRESULTResult(uiaSOK) || pointRange == 0 || nativeUIATextRangeFromThis(pointRange).value.Span().Start != 0 {
		t.Fatalf("point hr=%#x range=%#x", hr, pointRange)
	}
	nativeUIATextRangeRelease(pointRange)
	native.Close()
	root.Release()
}

func TestNativeUIATextUTF16LimitAndExplicitSelection(t *testing.T) {
	rootID := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 9, Object: 1}
	paneID := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 9, Object: 2, Activation: 1}
	caret := 2
	selection := accessibility.Span{Start: 1, End: 2}
	document, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 9, Generation: 1, Focus: paneID, Nodes: []accessibility.NodeDraft{
		{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm"},
		{ID: paneID, Parent: rootID, Role: accessibility.RoleTerminal, Name: "terminal", Rows: []accessibility.RowDraft{{Text: "A👩‍💻B", Bounds: []accessibility.Rect{{X: 0, Y: 0, Width: 8, Height: 16}, {X: 8, Y: 0, Width: 16, Height: 16}, {X: 24, Y: 0, Width: 8, Height: 16}}}}, Caret: &caret, Selection: &selection},
	}})
	if err != nil {
		t.Fatal(err)
	}
	publication := &uiaPublication{}
	if err := publication.PublishScreen(document, 0, 0, accessibility.Rect{Width: 80, Height: 24}); err != nil {
		t.Fatal(err)
	}
	api := &fakeUIANativeAPI{host: 88, hostHR: uiaSOK, result: 99}
	root, err := newDormantUIARootProvider(publication, api, 55)
	if err != nil {
		t.Fatal(err)
	}
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	pane := native.object(paneID)
	text := pane.provider.textInterface(pane)
	var whole uintptr
	if hr := nativeUIATextDocumentRange(text, uintptr(unsafe.Pointer(&whole))); hr != uiaHRESULTResult(uiaSOK) {
		t.Fatalf("document hr=%#x", hr)
	}
	if whole == 0 || nativeUIATextRangeFromThis(whole) == nil {
		t.Fatalf("invalid document range=%#x", whole)
	}
	if raw, rawErr := nativeUIATextRangeFromThis(whole).value.Text(document); rawErr != nil || raw != "A👩‍💻B" {
		t.Fatalf("raw text=%q err=%v", raw, rawErr)
	}
	if got := testNativeUIATextRangeString(t, whole, 2); got != "A" {
		t.Fatalf("surrogate-split text=%q", got)
	}
	var array uintptr
	if hr := nativeUIATextGetSelection(text, uintptr(unsafe.Pointer(&array))); hr != uiaHRESULTResult(uiaSOK) || array == 0 {
		t.Fatalf("selection hr=%#x array=%#x", hr, array)
	}
	var data uintptr
	if hr, _, _ := uiaSafeArrayAccessData.Call(array, uintptr(unsafe.Pointer(&data))); uiaHRESULT(int32(hr)) != uiaSOK || data == 0 {
		t.Fatalf("selection data hr=%#x", hr)
	}
	selected := *(*uintptr)(unsafe.Pointer(data))
	uiaSafeArrayUnaccessData.Call(array)
	if got := testNativeUIATextRangeString(t, selected, -1); got != "👩‍💻" {
		t.Fatalf("selection=%q", got)
	}
	uiaSafeArrayDestroy.Call(array)
	nativeUIATextRangeRelease(whole)
	native.Close()
	root.Release()
}

func testNativeUIATextRangeString(t *testing.T, value uintptr, limit int32) string {
	t.Helper()
	storage, _, _ := uiaGlobalAlloc.Call(0x40, unsafe.Sizeof(uintptr(0)))
	if storage == 0 {
		t.Fatal("output allocation failed")
	}
	defer uiaGlobalFree.Call(storage)
	hr := nativeUIATextRangeGetText(value, uintptr(uint32(limit)), storage)
	bstr := *(*uintptr)(unsafe.Pointer(storage))
	if hr != uiaHRESULTResult(uiaSOK) || bstr == 0 {
		t.Fatalf("get text hr=%#x bstr=%#x", hr, bstr)
	}
	result := nativeUIABSTRString(bstr)
	uiaSysFreeString.Call(bstr)
	return result
}

func testNativeUIADoubleArray(t *testing.T, array uintptr, count int) []float64 {
	t.Helper()
	var data uintptr
	if hr, _, _ := uiaSafeArrayAccessData.Call(array, uintptr(unsafe.Pointer(&data))); uiaHRESULT(int32(hr)) != uiaSOK || data == 0 {
		t.Fatalf("array data hr=%#x data=%#x", hr, data)
	}
	result := append([]float64(nil), unsafe.Slice((*float64)(unsafe.Pointer(data)), count)...)
	uiaSafeArrayUnaccessData.Call(array)
	return result
}

func testNativeUIAAllocBSTR(t *testing.T, value string) uintptr {
	t.Helper()
	units := utf16.Encode([]rune(value))
	var source uintptr
	if len(units) != 0 {
		source = uintptr(unsafe.Pointer(&units[0]))
	}
	bstr, _, _ := uiaSysAllocStringLen.Call(source, uintptr(len(units)))
	if bstr == 0 {
		t.Fatal("BSTR allocation failed")
	}
	return bstr
}

func testUIAWindowBounds() accessibility.Rect {
	return accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}
}
