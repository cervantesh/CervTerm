//go:build glfw

package glfwgl

import (
	"errors"
	"math"
	"sync"
	"testing"

	"cervterm/internal/accessibility"
)

type fakeUIANativeAPI struct {
	host          uintptr
	hostHR        uiaHRESULT
	result        uintptr
	disconnects   []uintptr
	calls         [][4]uintptr
	panicReturn   bool
	returnEntered chan struct{}
	returnRelease chan struct{}
}

func (api *fakeUIANativeAPI) HostProviderFromHWND(uintptr) (uintptr, uiaHRESULT) {
	return api.host, api.hostHR
}
func (api *fakeUIANativeAPI) ReturnRawElementProvider(hwnd, wParam, lParam, provider uintptr) uintptr {
	if api.panicReturn {
		panic("uia")
	}
	api.calls = append(api.calls, [4]uintptr{hwnd, wParam, lParam, provider})
	if api.returnEntered != nil {
		close(api.returnEntered)
		<-api.returnRelease
	}
	return api.result
}

func (api *fakeUIANativeAPI) DisconnectProvider(provider uintptr) uiaHRESULT {
	api.disconnects = append(api.disconnects, provider)
	return uiaSOK
}

func uiaTestDocument(t *testing.T, generation uint64) (accessibility.Document, accessibility.NodeID, accessibility.NodeID) {
	t.Helper()
	root := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: 1, Object: 1}
	pane := accessibility.NodeID{Kind: accessibility.NodeKindPane, Projection: 1, Object: 2, Activation: 1}
	caret := 1
	document, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: generation, Focus: pane, Nodes: []accessibility.NodeDraft{
		{ID: root, Role: accessibility.RoleWindow, Name: "CervTerm"},
		{ID: pane, Parent: root, Role: accessibility.RoleTerminal, Name: "terminal", Rows: []accessibility.RowDraft{{Text: "ok", Bounds: []accessibility.Rect{{X: 10, Y: 20, Width: 8, Height: 16}, {X: 18, Y: 20, Width: 8, Height: 16}}}}, Caret: &caret},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return document, root, pane
}

func newUIATestProvider(t *testing.T) (*uiaRootProvider, *uiaPublication, *fakeUIANativeAPI, accessibility.NodeID, accessibility.NodeID) {
	t.Helper()
	document, root, pane := uiaTestDocument(t, 1)
	publication := &uiaPublication{}
	if err := publication.PublishScreen(document, 100, 200, accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}); err != nil {
		t.Fatal(err)
	}
	api := &fakeUIANativeAPI{host: 88, hostHR: uiaSOK, result: 99}
	provider, err := newDormantUIARootProvider(publication, api, 55)
	if err != nil {
		t.Fatal(err)
	}
	return provider, publication, api, root, pane
}

func TestDormantUIAProviderInterfaceMatrixAndRefcountBounds(t *testing.T) {
	provider, _, _, _, _ := newUIATestProvider(t)
	for _, test := range []struct {
		iid  uiaGUID
		want uiaInterface
	}{
		{uiaIIDUnknown, uiaInterfaceUnknown}, {uiaIIDRawElementProviderSimple, uiaInterfaceSimple},
		{uiaIIDRawElementProviderFragment, uiaInterfaceFragment}, {uiaIIDRawElementProviderFragmentRoot, uiaInterfaceFragmentRoot},
	} {
		if got, hr := provider.QueryInterface(test.iid); got != test.want || hr != uiaSOK {
			t.Fatalf("iid=%#v got=%v hr=%d", test.iid, got, hr)
		}
	}
	if got, hr := provider.QueryInterface(uiaGUID{Data1: 99}); got != uiaInterfaceNone || hr != uiaENoInterface {
		t.Fatalf("unknown got=%v hr=%d", got, hr)
	}
	for range 4 {
		provider.Release()
	}
	if provider.refs.Load() != 1 {
		t.Fatalf("refs=%d", provider.refs.Load())
	}
	provider.refs.Store(math.MaxUint32)
	if provider.AddRef() != math.MaxUint32 || provider.Release() != math.MaxUint32 {
		t.Fatal("refcount saturation changed")
	}
	provider.refs.Store(1)
	if provider.Release() != 0 || provider.Release() != 0 || !provider.disconnected.Load() {
		t.Fatalf("release refs=%d disconnected=%v", provider.refs.Load(), provider.disconnected.Load())
	}
}

func TestDormantUIAProviderPropertiesNavigationFocusAndDisconnect(t *testing.T) {
	provider, _, api, root, pane := newUIATestProvider(t)
	if options, hr := provider.ProviderOptions(); options != uiaProviderServer || hr != uiaSOK {
		t.Fatalf("options=%d hr=%d", options, hr)
	}
	if host, hr := provider.HostProvider(); host != 88 || hr != uiaSOK {
		t.Fatalf("host=%d hr=%d", host, hr)
	}
	if value, hr := provider.Property(pane, uiaPropertyName); hr != uiaSOK || value.String != "terminal" {
		t.Fatalf("name=%#v hr=%d", value, hr)
	}
	if value, _ := provider.Property(pane, uiaPropertyHasKeyboardFocus); !value.Bool {
		t.Fatalf("focus property=%#v", value)
	}
	if value, _ := provider.Property(pane, uiaPropertyIsKeyboardFocusable); !value.Bool {
		t.Fatalf("focusable property=%#v", value)
	}
	if value, _ := provider.Property(root, uiaPropertyIsKeyboardFocusable); value.Bool {
		t.Fatalf("root focusable property=%#v", value)
	}
	if value, _ := provider.Property(pane, uiaPropertyBoundingRectangle); value.Rect != (accessibility.Rect{X: 110, Y: 220, Width: 16, Height: 16}) {
		t.Fatalf("bounds=%#v", value.Rect)
	}
	if child, hr := provider.Navigate(root, uiaNavigateFirstChild); child != pane || hr != uiaSOK {
		t.Fatalf("child=%#v hr=%d", child, hr)
	}
	if parent, hr := provider.Navigate(pane, uiaNavigateParent); parent != root || hr != uiaSOK {
		t.Fatalf("parent=%#v hr=%d", parent, hr)
	}
	if focus, hr := provider.Focus(); focus != pane || hr != uiaSOK {
		t.Fatalf("focus=%#v hr=%d", focus, hr)
	}
	provider.nativePointer.Store(123)
	provider.Disconnect()
	if _, hr := provider.Focus(); hr != uiaEElementNotAvailable {
		t.Fatalf("disconnected hr=%d", hr)
	}
	if len(api.disconnects) != 1 || api.disconnects[0] != 123 {
		t.Fatalf("disconnects=%v", api.disconnects)
	}
	if result, hr := provider.QueryInterface(uiaIIDUnknown); result != uiaInterfaceUnknown || hr != uiaSOK {
		t.Fatalf("static QueryInterface result=%v hr=%d", result, hr)
	}
	provider.Release()
	if len(api.calls) != 0 {
		t.Fatalf("unexpected native calls=%v", api.calls)
	}
}

func TestDormantUIADispatcherBoundDuplicateStaleAndOwnership(t *testing.T) {
	dispatcher := newUIAProviderDispatcher()
	providers := make([]*uiaRootProvider, maxUIAProviders)
	for index := range providers {
		provider, _, _, _, _ := newUIATestProvider(t)
		providers[index] = provider
		token, err := dispatcher.Register(provider)
		if err != nil || token == 0 || provider.refs.Load() != 2 {
			t.Fatalf("register %d token=%d refs=%d err=%v", index, token, provider.refs.Load(), err)
		}
	}
	if _, err := dispatcher.Register(providers[0]); !errors.Is(err, errUIAProviderDuplicate) {
		t.Fatalf("duplicate err=%v", err)
	}
	extra, _, _, _, _ := newUIATestProvider(t)
	if _, err := dispatcher.Register(extra); !errors.Is(err, errUIAProviderLimit) {
		t.Fatalf("limit err=%v", err)
	}
	if err := dispatcher.Unregister(1); err != nil || providers[0].refs.Load() != 1 {
		t.Fatalf("unregister refs=%d err=%v", providers[0].refs.Load(), err)
	}
	if err := dispatcher.Unregister(1); !errors.Is(err, errUIAProviderMissing) {
		t.Fatalf("stale err=%v", err)
	}
}

func TestDormantUIAWndProcHandlerConsumesOnlyRootObject(t *testing.T) {
	provider, _, api, _, _ := newUIATestProvider(t)
	dispatcher := newUIAProviderDispatcher()
	token, err := dispatcher.Register(provider)
	if err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Unregister(token)
	provider.nativePointer.Store(123)
	handler := &uiaWndProcHandler{provider: provider}
	if handled, result, err := handler.handleWndProcMessage(55, wmGetObject, 3, ^uintptr(24)); !handled || result != 99 || err != nil {
		t.Fatalf("root handled=%v result=%d err=%v", handled, result, err)
	}
	if len(api.calls) != 1 || api.calls[0][3] != 123 {
		t.Fatalf("calls=%v token=%d", api.calls, token)
	}
	for _, message := range []struct {
		hwnd   uintptr
		msg    uint32
		lParam uintptr
	}{{55, 42, 0}, {55, wmGetObject, 0}, {99, wmGetObject, ^uintptr(24)}} {
		if handled, _, _ := handler.handleWndProcMessage(message.hwnd, message.msg, 0, message.lParam); handled {
			t.Fatalf("consumed %#v", message)
		}
	}
}

func TestDormantUIAProviderConcurrentReadsAndDisconnect(t *testing.T) {
	provider, _, _, _, pane := newUIATestProvider(t)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				provider.Property(pane, uiaPropertyName)
				provider.Focus()
			}
		}()
	}
	provider.Disconnect()
	wait.Wait()
	if provider.available() {
		t.Fatal("provider remained available")
	}
}

func TestDormantUIAWndProcHostConsumeChainAndPanicContainment(t *testing.T) {
	provider, _, api, _, _ := newUIATestProvider(t)
	dispatcher := newUIAProviderDispatcher()
	token, err := dispatcher.Register(provider)
	if err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Unregister(token)
	provider.nativePointer.Store(123)
	backend := &fakeWndProcBackend{current: 9, chainResult: 77}
	reports := 0
	host := &windowsWndProcHost{backend: backend, hwnd: 55, report: func(error) { reports++ }}
	if _, err := host.registerHandler(&uiaWndProcHandler{provider: provider}); err != nil {
		t.Fatal(err)
	}
	if err := host.install(); err != nil {
		t.Fatal(err)
	}
	if got := backend.callback(55, wmGetObject, 3, ^uintptr(24)); got != 99 || len(backend.chainCalls) != 0 {
		t.Fatalf("root got=%d chain=%v", got, backend.chainCalls)
	}
	if got := backend.callback(55, wmGetObject, 3, 0); got != 77 || len(backend.chainCalls) != 1 {
		t.Fatalf("other got=%d chain=%v", got, backend.chainCalls)
	}
	api.panicReturn = true
	if got := backend.callback(55, wmGetObject, 3, ^uintptr(24)); got != 77 || reports != 1 || len(backend.chainCalls) != 2 {
		t.Fatalf("panic got=%d reports=%d chain=%v", got, reports, backend.chainCalls)
	}
}

func TestUIAPublicationRejectsRollbackIdentityChangeAndReconnect(t *testing.T) {
	first, root, _ := uiaTestDocument(t, 1)
	second, _, _ := uiaTestDocument(t, 2)
	publication := &uiaPublication{}
	if err := publication.PublishScreen(first, 10, 20, accessibility.Rect{X: 10, Y: 20, Width: 100, Height: 100}); err != nil {
		t.Fatal(err)
	}
	if err := publication.Publish(first); !errors.Is(err, errUIAPublicationStale) {
		t.Fatalf("rollback err=%v", err)
	}
	if err := publication.PublishScreen(second, 10, 20, accessibility.Rect{X: 10, Y: 20, Width: 100, Height: 100}); err != nil {
		t.Fatal(err)
	}
	changedRoot := accessibility.NodeID{Kind: accessibility.NodeKindWindow, Projection: root.Projection, Object: 99}
	changed, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 3, Nodes: []accessibility.NodeDraft{{ID: changedRoot, Role: accessibility.RoleWindow}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.PublishScreen(changed, 10, 20, accessibility.Rect{X: 10, Y: 20, Width: 100, Height: 100}); !errors.Is(err, errUIAPublicationStale) {
		t.Fatalf("identity err=%v", err)
	}
	publication.Disconnect()
	if err := publication.PublishScreen(second, 10, 20, accessibility.Rect{X: 10, Y: 20, Width: 100, Height: 100}); !errors.Is(err, errUIAPublicationClosed) {
		t.Fatalf("reconnect err=%v", err)
	}
}
