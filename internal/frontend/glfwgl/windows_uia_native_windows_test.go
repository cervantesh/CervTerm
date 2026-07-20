//go:build glfw && windows

package glfwgl

import (
	"testing"
	"unsafe"

	"cervterm/internal/accessibility"
)

func TestNativeUIAABIConstantsAndVTableLayout(t *testing.T) {
	pointer := unsafe.Sizeof(uintptr(0))
	if unsafe.Sizeof(nativeUIASimpleVTable{}) != 7*pointer || unsafe.Sizeof(nativeUIAFragmentVTable{}) != 9*pointer || unsafe.Sizeof(nativeUIAFragmentRootVTable{}) != 5*pointer {
		t.Fatalf("pointer=%d simple=%d fragment=%d root=%d", pointer, unsafe.Sizeof(nativeUIASimpleVTable{}), unsafe.Sizeof(nativeUIAFragmentVTable{}), unsafe.Sizeof(nativeUIAFragmentRootVTable{}))
	}
	if unsafe.Offsetof(nativeUIASimpleVTable{}.ProviderOptions) != 3*pointer || unsafe.Offsetof(nativeUIAFragmentVTable{}.FragmentRoot) != 8*pointer || unsafe.Offsetof(nativeUIAFragmentRootVTable{}.GetFocus) != 4*pointer {
		t.Fatal("unexpected COM vtable offsets")
	}
	if uiaIIDRawElementProviderSimple.Data1 != 0xd6dd68d1 || uiaIIDRawElementProviderFragmentRoot.Data1 != 0x620ce2a5 || wmGetObject != 0x3d || uiaRootObjectID != -25 {
		t.Fatal("unexpected UIA ABI constants")
	}
	wantVariant := uintptr(16)
	if pointer == 8 {
		wantVariant = 24
	}
	if unsafe.Sizeof(nativeUIAVariant{}) != wantVariant {
		t.Fatalf("variant size=%d want=%d", unsafe.Sizeof(nativeUIAVariant{}), wantVariant)
	}
}

func TestNativeUIAInterfaceNavigationFocusAndOwnership(t *testing.T) {
	root, _, _, rootID, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	paneObject := native.object(paneID)
	if paneObject == nil || nativeUIARetain(paneObject) == 0 {
		t.Fatal("pane interface allocation failed")
	}
	this := native.rootObject.fragment
	var output uintptr
	iid := uiaIIDRawElementProviderSimple
	if hr := nativeUIAQueryInterface(this, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output == 0 {
		t.Fatalf("QI hr=%#x output=%#x", hr, output)
	}
	simple := output
	iid = uiaIIDUnknown
	output = 0
	if hr := nativeUIAQueryInterface(simple, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != this {
		t.Fatalf("canonical IUnknown hr=%#x output=%#x root=%#x", hr, output, this)
	}
	nativeUIARelease(output)
	iid = uiaGUID{Data1: 99}
	output = 77
	if hr := nativeUIAQueryInterface(simple, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaENoInterface) || output != 0 {
		t.Fatalf("unsupported QI hr=%#x output=%#x", hr, output)
	}
	iid = uiaIIDRawElementProviderFragmentRoot
	if hr := nativeUIAQueryInterface(this, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != native.rootObject.fragmentRoot {
		t.Fatalf("root interface QI hr=%#x output=%#x", hr, output)
	}
	rootInterface := output
	iid = uiaIIDUnknown
	output = 0
	if hr := nativeUIAQueryInterface(rootInterface, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != this {
		t.Fatalf("root canonical IUnknown hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	nativeUIARelease(rootInterface)
	iid = uiaIIDRawElementProviderFragmentRoot
	output = 77
	if hr := nativeUIAQueryInterface(paneObject.fragment, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaENoInterface) || output != 0 {
		t.Fatalf("child root QI hr=%#x output=%#x", hr, output)
	}
	output = simple
	if root.refs.Load() != 4 {
		t.Fatalf("refs=%d", root.refs.Load())
	}
	nativeUIARelease(output)
	output = 0
	if hr := nativeUIANavigate(this, uintptr(uiaNavigateFirstChild), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output == 0 || nativeUIAObjectFromThis(output).node != paneID {
		t.Fatalf("navigate hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	rootRuntimeID := uintptr(77)
	if hr := nativeUIAGetRuntimeID(this, uintptr(unsafe.Pointer(&rootRuntimeID))); hr != uiaHRESULTResult(uiaSOK) || rootRuntimeID != 0 {
		t.Fatalf("root runtime hr=%#x value=%#x", hr, rootRuntimeID)
	}
	var runtimeID uintptr
	if hr := nativeUIAGetRuntimeID(paneObject.fragment, uintptr(unsafe.Pointer(&runtimeID))); hr != uiaHRESULTResult(uiaSOK) || runtimeID == 0 {
		t.Fatalf("runtime ID hr=%#x value=%#x", hr, runtimeID)
	}
	var runtimeData uintptr
	if hr, _, _ := uiaSafeArrayAccessData.Call(runtimeID, uintptr(unsafe.Pointer(&runtimeData))); uiaHRESULT(int32(hr)) != uiaSOK || runtimeData == 0 {
		t.Fatalf("runtime data hr=%#x data=%#x", hr, runtimeData)
	}
	runtimeValues := unsafe.Slice((*int32)(unsafe.Pointer(runtimeData)), 8)
	if runtimeValues[0] != 3 || runtimeValues[3] != int32(paneID.Kind) || runtimeValues[4] != int32(paneID.Object) {
		t.Fatalf("runtime values=%v", runtimeValues)
	}
	uiaSafeArrayUnaccessData.Call(runtimeID)
	uiaSafeArrayDestroy.Call(runtimeID)
	var rect nativeUIARect
	if hr := nativeUIABoundingRectangle(paneObject.fragment, uintptr(unsafe.Pointer(&rect))); hr != uiaHRESULTResult(uiaSOK) || rect.Width != 16 || rect.Height != 16 {
		t.Fatalf("bounds hr=%#x rect=%#v", hr, rect)
	}
	var boundsVariant nativeUIAVariant
	if hr := nativeUIAGetPropertyValue(paneObject.simple, uintptr(uiaPropertyBoundingRectangle), uintptr(unsafe.Pointer(&boundsVariant))); hr != uiaHRESULTResult(uiaSOK) || boundsVariant.VT != uiaVTArray|uiaVTR8 || boundsVariant.Value == 0 {
		t.Fatalf("bounds variant hr=%#x value=%#v", hr, boundsVariant)
	}
	var boundsData uintptr
	if hr, _, _ := uiaSafeArrayAccessData.Call(boundsVariant.Value, uintptr(unsafe.Pointer(&boundsData))); uiaHRESULT(int32(hr)) != uiaSOK || boundsData == 0 {
		t.Fatalf("bounds data hr=%#x data=%#x", hr, boundsData)
	}
	boundsValues := unsafe.Slice((*float64)(unsafe.Pointer(boundsData)), 4)
	if boundsValues[0] != 110 || boundsValues[1] != 220 || boundsValues[2] != 16 || boundsValues[3] != 16 {
		t.Fatalf("bounds values=%v", boundsValues)
	}
	uiaSafeArrayUnaccessData.Call(boundsVariant.Value)
	uiaSafeArrayDestroy.Call(boundsVariant.Value)
	output = 0
	if hr := nativeUIAGetFocus(this, uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || nativeUIAObjectFromThis(output).node != paneID {
		t.Fatalf("focus hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	if native.rootObject.node != rootID || root.nativePointer.Load() != native.rootObject.simple {
		t.Fatalf("root=%#v pointer=%#x", native.rootObject.node, root.nativePointer.Load())
	}
	native.Close()
	if root.nativePointer.Load() != 0 {
		t.Fatal("native pointer retained after close")
	}
	output = 0
	iid = uiaIIDUnknown
	if hr := nativeUIAQueryInterface(this, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaSOK) || output != this {
		t.Fatalf("disconnected IUnknown hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	output = 1
	if hr := nativeUIAGetPatternProvider(this, 0, uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaEElementNotAvailable) || output != 0 {
		t.Fatalf("disconnected pattern hr=%#x output=%#x", hr, output)
	}
	if hr := nativeUIAGetRuntimeID(this, uintptr(unsafe.Pointer(&output))); hr != uiaHRESULTResult(uiaEElementNotAvailable) {
		t.Fatalf("disconnected runtime hr=%#x", hr)
	}
	nativeUIARelease(paneObject.fragment)
	root.Release()
	if native.objects != nil || nativeUIAObjectFromThis(this) != nil {
		t.Fatal("native COM storage survived final release")
	}
}

func TestNativeUIAElementFromPointAndHRESULTWidths(t *testing.T) {
	root, _, _, _, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { native.Close(); root.Release() }()
	this := native.rootObject.fragmentRoot
	hr, output := testNativeUIAElementFromPoint(uiaFragmentRootVTable.ElementProviderFromPoint, this, 119, 225)
	if hr != uiaHRESULTResult(uiaSOK) || output == 0 || nativeUIAObjectFromThis(output).node != paneID {
		t.Fatalf("point hr=%#x output=%#x", hr, output)
	}
	nativeUIARelease(output)
	if got := uiaHRESULTResult(uiaENoInterface); uint32(got) != 0x80004002 {
		t.Fatalf("HRESULT=%#x", got)
	}
}

func TestNativeUIAWndProcPassesRawCOMPointer(t *testing.T) {
	root, _, api, _, _ := newUIATestProvider(t)
	dispatcher := newUIAProviderDispatcher()
	token, err := dispatcher.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Unregister(token)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { native.Close(); root.Release() }()
	handler := &uiaWndProcHandler{provider: root}
	handled, result, err := handler.handleWndProcMessage(55, wmGetObject, 0, ^uintptr(24))
	if !handled || result != 99 || err != nil || len(api.calls) != 1 || api.calls[0][3] != root.nativePointer.Load() {
		t.Fatalf("handled=%v result=%d err=%v calls=%v", handled, result, err, api.calls)
	}
}

func TestNativeUIAWndProcRetainsCOMStorageAcrossConcurrentDisconnect(t *testing.T) {
	root, _, api, _, _ := newUIATestProvider(t)
	dispatcher := newUIAProviderDispatcher()
	token, err := dispatcher.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	api.returnEntered = make(chan struct{})
	api.returnRelease = make(chan struct{})
	type outcome struct {
		handled bool
		result  uintptr
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		handled, result, callErr := (&uiaWndProcHandler{provider: root}).handleWndProcMessage(55, wmGetObject, 0, ^uintptr(24))
		done <- outcome{handled: handled, result: result, err: callErr}
	}()
	<-api.returnEntered
	native.Close()
	if err := dispatcher.Unregister(token); err != nil {
		t.Fatal(err)
	}
	root.Release()
	if native.objects == nil || root.refs.Load() != 1 {
		t.Fatalf("storage was not pinned: objects=%v refs=%d", native.objects != nil, root.refs.Load())
	}
	close(api.returnRelease)
	call := <-done
	if !call.handled || call.result != 99 || call.err != nil {
		t.Fatalf("call=%#v", call)
	}
	if native.objects != nil || root.refs.Load() != 0 {
		t.Fatalf("storage survived forwarding: objects=%v refs=%d", native.objects != nil, root.refs.Load())
	}
}

func TestNativeUIALazilyPrunesUnreferencedHistoricalNodes(t *testing.T) {
	root, publication, _, rootID, paneID := newUIATestProvider(t)
	native, err := newNativeUIAProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	old := native.object(paneID)
	if old == nil {
		t.Fatal("old pane allocation failed")
	}
	oldPointer := old.fragment
	newPane := paneID
	newPane.Activation++
	caret := 1
	document, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 2, Focus: newPane, Nodes: []accessibility.NodeDraft{
		{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm"},
		{ID: newPane, Parent: rootID, Role: accessibility.RoleTerminal, Name: "terminal", Rows: []accessibility.RowDraft{{Text: "ok", Bounds: []accessibility.Rect{{X: 10, Y: 20, Width: 8, Height: 16}, {X: 18, Y: 20, Width: 8, Height: 16}}}}, Caret: &caret},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.PublishScreen(document, 100, 200, accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}); err != nil {
		t.Fatal(err)
	}
	if current := native.object(newPane); current == nil {
		t.Fatal("current pane allocation failed")
	}
	if native.objects[paneID] != nil || nativeUIAObjectFromThis(oldPointer) != nil {
		t.Fatal("unreferenced historical node survived lazy pruning")
	}
	native.Close()
	root.Release()
}
