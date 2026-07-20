//go:build glfw && windows

package glfwgl

import (
	"errors"
	"testing"

	"cervterm/internal/accessibility"
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestDormantProjectionAccessibilityPreparePublishAndTeardown(t *testing.T) {
	document, _, _ := uiaTestDocument(t, 1)
	api := &fakeUIANativeAPI{host: 88, hostHR: uiaSOK, result: 99}
	dispatcher := newUIAProviderDispatcher()
	backend := &fakeWndProcBackend{current: 9, callbackPtr: 77}
	host := &windowsWndProcHost{backend: backend, hwnd: 55}
	before := newCompositionBeforeUnbind(&App{})
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: document, ScreenX: 100, ScreenY: 200, Bounds: accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600},
		HWND: 55, API: api, Dispatcher: dispatcher, Host: host,
	}, before)
	if err != nil {
		t.Fatal(err)
	}
	if err := before.attachBeforeNative(lifecycle.Close); err != nil {
		t.Fatal(err)
	}
	root, native := lifecycle.root, lifecycle.native
	if lifecycle.token == 0 || lifecycle.handlerID == 0 || !host.installed || root.refs.Load() != 3 {
		t.Fatalf("token=%d handler=%d installed=%v refs=%d", lifecycle.token, lifecycle.handlerID, host.installed, root.refs.Load())
	}
	if handled, result, err := (&uiaWndProcHandler{provider: root}).handleWndProcMessage(55, wmGetObject, 0, ^uintptr(24)); !handled || result != 99 || err != nil {
		t.Fatalf("forward handled=%v result=%d err=%v", handled, result, err)
	}
	if err := lifecycle.Publish(document, 100, 200, accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}); err == nil {
		t.Fatal("duplicate generation publication succeeded")
	}
	if snapshot, ok := lifecycle.publication.Snapshot(); !ok || snapshot.Generation() != 1 {
		t.Fatal("failed publication replaced the last immutable document")
	}
	next, _, _ := uiaTestDocument(t, 2)
	eventErr := errors.New("native event")
	lifecycle.eventSink = func(accessibility.Document) error { return eventErr }
	if err := lifecycle.Publish(next, 100, 200, accessibility.Rect{X: 100, Y: 200, Width: 800, Height: 600}); !errors.Is(err, eventErr) {
		t.Fatalf("event err=%v", err)
	}
	if snapshot, ok := lifecycle.publication.Snapshot(); !ok || snapshot.Generation() != 2 {
		t.Fatal("native-event failure rolled back the published document")
	}
	captureErr := errors.New("capture")
	if err := lifecycle.CaptureAndPublish(func() (projectionAccessibilitySnapshot, error) { return projectionAccessibilitySnapshot{}, captureErr }); !errors.Is(err, captureErr) {
		t.Fatalf("capture err=%v", err)
	}
	if snapshot, ok := lifecycle.publication.Snapshot(); !ok || snapshot.Generation() != 2 {
		t.Fatal("capture failure replaced the last immutable document")
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	if root.refs.Load() != 0 || native.objects != nil || host.installed || !host.released || backend.current != 9 {
		t.Fatalf("refs=%d objects=%v installed=%v released=%v current=%d", root.refs.Load(), native.objects != nil, host.installed, host.released, backend.current)
	}
	if _, ok := dispatcher.Provider(1); ok {
		t.Fatal("dispatcher token survived teardown")
	}
}

func TestProjectionAccessibilityRefreshCoalescesAndPublishesSemanticEvents(t *testing.T) {
	document, rootID, paneID := uiaTestDocument(t, 1)
	backend := &fakeWndProcBackend{current: 9, callbackPtr: 77}
	before := newCompositionBeforeUnbind(&App{})
	var events []accessibility.SemanticEvent
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: document, Bounds: accessibility.Rect{Width: 800, Height: 600}, HWND: 55,
		API: &fakeUIANativeAPI{host: 88, hostHR: uiaSOK}, Dispatcher: newUIAProviderDispatcher(),
		Host:              &windowsWndProcHost{backend: backend, hwnd: 55},
		SemanticEventSink: func(event accessibility.SemanticEvent) error { events = append(events, event); return nil },
	}, before)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lifecycle.Close(); _ = before.close() }()
	caret := 2
	next, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 2, Focus: paneID, Nodes: []accessibility.NodeDraft{
		{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm"},
		{ID: paneID, Parent: rootID, Role: accessibility.RoleTerminal, Name: "terminal", Rows: []accessibility.RowDraft{{Text: "next", Bounds: []accessibility.Rect{{Width: 8, Height: 16}, {X: 8, Width: 8, Height: 16}, {X: 16, Width: 8, Height: 16}, {X: 24, Width: 8, Height: 16}}}}, Caret: &caret},
	}})
	if err != nil {
		t.Fatal(err)
	}
	captures := 0
	lifecycle.capture = func(uint64) (projectionAccessibilitySnapshot, error) {
		captures++
		return projectionAccessibilitySnapshot{Document: next, Bounds: accessibility.Rect{Width: 800, Height: 600}}, nil
	}
	lifecycle.generation = 1
	lifecycle.Invalidate(accessibility.IntentText | accessibility.IntentCaret)
	lifecycle.Invalidate(accessibility.IntentText)
	if err := lifecycle.Refresh(); err != nil {
		t.Fatal(err)
	}
	if captures != 1 || lifecycle.generation != 2 || len(events) != 2 || events[0].Kind != accessibility.EventTextChanged || events[1].Kind != accessibility.EventCaretChanged {
		t.Fatalf("captures=%d generation=%d events=%#v", captures, lifecycle.generation, events)
	}
	if err := lifecycle.Refresh(); err != nil || captures != 1 {
		t.Fatalf("idle refresh captures=%d err=%v", captures, err)
	}
	announcementDocument, _, _ := uiaTestDocument(t, 3)
	lifecycle.capture = func(uint64) (projectionAccessibilitySnapshot, error) {
		captures++
		return projectionAccessibilitySnapshot{Document: announcementDocument, Bounds: accessibility.Rect{Width: 800, Height: 600}}, nil
	}
	if err := lifecycle.Announce(accessibility.AnnouncementBell); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("announcement was not coalesced: events=%#v", events)
	}
	if err := lifecycle.Refresh(); err != nil {
		t.Fatal(err)
	}
	if captures != 2 || len(events) <= 2 || events[len(events)-1].Kind != accessibility.EventAnnouncement || events[len(events)-1].Announcement != accessibility.AnnouncementBell {
		t.Fatalf("captures=%d announcement events=%#v", captures, events)
	}
	beforeHidden := len(events)
	hidden, err := accessibility.NewDocument(accessibility.DocumentDraft{ProviderID: 7, Generation: 4, Focus: rootID, Nodes: []accessibility.NodeDraft{{ID: rootID, Role: accessibility.RoleWindow, Name: "CervTerm"}}})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.capture = func(uint64) (projectionAccessibilitySnapshot, error) {
		return projectionAccessibilitySnapshot{Document: hidden, Bounds: accessibility.Rect{Width: 800, Height: 600}}, nil
	}
	if err := lifecycle.Announce(accessibility.AnnouncementNotification); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Refresh(); err != nil {
		t.Fatal(err)
	}
	for _, event := range events[beforeHidden:] {
		if event.Kind == accessibility.EventAnnouncement {
			t.Fatalf("hidden announcement leaked: %#v", events)
		}
	}
}

func TestDormantProjectionAccessibilityInstallFailureRollsBack(t *testing.T) {
	document, _, _ := uiaTestDocument(t, 1)
	injected := errors.New("callback")
	backend := &fakeWndProcBackend{current: 9, callbackPtr: 77, newErr: injected}
	host := &windowsWndProcHost{backend: backend, hwnd: 55}
	before := newCompositionBeforeUnbind(&App{})
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: document, Bounds: accessibility.Rect{Width: 800, Height: 600}, HWND: 55,
		API: &fakeUIANativeAPI{host: 88, hostHR: uiaSOK}, Dispatcher: newUIAProviderDispatcher(), Host: host,
	}, before)
	if !errors.Is(err, injected) || lifecycle == nil {
		t.Fatalf("lifecycle=%p err=%v", lifecycle, err)
	}
	root, native := lifecycle.root, lifecycle.native
	if err := lifecycle.Close(); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	if root.refs.Load() != 0 || native.objects != nil || !host.released {
		t.Fatalf("refs=%d objects=%v released=%v", root.refs.Load(), native.objects != nil, host.released)
	}
}

func TestDormantProjectionAccessibilityReusesInstalledIMEHost(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9}
	app, before, cleanup := withFakeProjectionIME(t, backend, &fakeIMMAPI{data: map[uint32][]byte{}})
	defer cleanup()
	if activation := app.activateProjectionIME(nil, before); activation.State != imeActivationActive {
		t.Fatalf("activation=%#v", activation)
	}
	document, _, _ := uiaTestDocument(t, 1)
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: document, Bounds: accessibility.Rect{Width: 800, Height: 600}, HWND: 5,
		API: &fakeUIANativeAPI{host: 88, hostHR: uiaSOK}, Dispatcher: newUIAProviderDispatcher(),
	}, before)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.host != before.wndProcHost || len(lifecycle.host.handlers) != 2 || backend.registered == 0 {
		t.Fatalf("host=%p shared=%p handlers=%d callback=%d", lifecycle.host, before.wndProcHost, len(lifecycle.host.handlers), backend.registered)
	}
	if err := before.attachBeforeNative(lifecycle.Close); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	if backend.current != 9 || backend.registered != 0 {
		t.Fatalf("current=%d callback=%d", backend.current, backend.registered)
	}
}

func TestDormantProjectionAccessibilityDoesNotRetryFallbackIMEHost(t *testing.T) {
	injected := errors.New("install")
	backend := &fakeWndProcBackend{current: 9, getErr: injected}
	app, before, cleanup := withFakeProjectionIME(t, backend, &fakeIMMAPI{})
	defer cleanup()
	if activation := app.activateProjectionIME(nil, before); activation.State != imeActivationFallback {
		t.Fatalf("activation=%#v", activation)
	}
	document, _, _ := uiaTestDocument(t, 1)
	lifecycle, err := prepareDormantProjectionAccessibility(projectionAccessibilityPreparation{
		Document: document, Bounds: accessibility.Rect{Width: 800, Height: 600}, HWND: 5,
		API: &fakeUIANativeAPI{host: 88, hostHR: uiaSOK}, Dispatcher: newUIAProviderDispatcher(),
	}, before)
	if !errors.Is(err, errWndProcHostInvalid) || lifecycle == nil {
		t.Fatalf("lifecycle=%p err=%v", lifecycle, err)
	}
	if backend.registered != 0 || before.wndProcHost.installed || before.wndProcHost.active {
		t.Fatalf("callback=%d installed=%v active=%v", backend.registered, before.wndProcHost.installed, before.wndProcHost.active)
	}
	if err := lifecycle.Close(); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); !errors.Is(err, injected) && err != nil {
		t.Fatalf("cleanup err=%v", err)
	}
}

func TestInitialProjectionTransfersDormantAccessibilityOwnership(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	var log []string
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		log = append(log, "prepare")
		return &fakeProjectionAccessibilityLifecycle{log: &log}, nil
	}
	window := new(glfw.Window)
	app := &App{cfg: config.Defaults()}
	app.cfg.Accessibility.Enabled = true
	app.controller = &windowController{windows: map[termmux.WindowID]*windowProjection{
		termmux.WindowID(initialWindowID): {id: termmux.WindowID(initialWindowID), host: window, app: app},
	}}
	if err := app.adoptInitialProjection(window); err != nil {
		t.Fatal(err)
	}
	bundle := app.controller.windows[termmux.WindowID(initialWindowID)].bundle
	if bundle == nil || bundle.beforeUnbind == nil || len(log) != 1 || log[0] != "prepare" {
		t.Fatalf("bundle=%p log=%v", bundle, log)
	}
	if err := bundle.beforeUnbind.close(); err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[1] != "accessibility" {
		t.Fatalf("teardown=%v", log)
	}
}
