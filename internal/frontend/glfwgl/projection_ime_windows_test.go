//go:build glfw && windows

package glfwgl

import (
	"errors"
	"testing"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func withFakeProjectionIME(t *testing.T, backend *fakeWndProcBackend, api *fakeIMMAPI) (*App, *compositionBeforeUnbind, func()) {
	t.Helper()
	oldHWND, oldComponents := projectionIMEWindowHWND, projectionIMEComponents
	projectionIMEWindowHWND = func(*glfw.Window) (uintptr, error) { return 5, nil }
	app := &App{cfg: config.Defaults()}
	app.cfg.IME.Enabled = true
	app.initCompositionCoordinator()
	projectionIMEComponents = func(app *App, hwnd uintptr, report func(error)) (*immDecoder, *windowsWndProcHost) {
		decoder := newDormantIMMDecoder(app, hwnd, api)
		return decoder, &windowsWndProcHost{backend: backend, decoder: decoder, hwnd: hwnd, report: report}
	}
	cleanup := func() {
		projectionIMEWindowHWND, projectionIMEComponents = oldHWND, oldComponents
	}
	return app, newCompositionBeforeUnbind(app), cleanup
}

func TestProjectionIMEDisabledDoesNotTouchNativeSeams(t *testing.T) {
	oldHWND := projectionIMEWindowHWND
	defer func() { projectionIMEWindowHWND = oldHWND }()
	calls := 0
	projectionIMEWindowHWND = func(*glfw.Window) (uintptr, error) { calls++; return 0, errors.New("unexpected") }
	app := &App{cfg: config.Defaults()}
	activation := app.activateProjectionIME(nil, newCompositionBeforeUnbind(app))
	if activation.State != imeActivationDisabled || calls != 0 || app.notice != "" {
		t.Fatalf("activation=%#v calls=%d notice=%q", activation, calls, app.notice)
	}
}

func TestProjectionIMEOptInPublishesAndTearsDownAtomically(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9}
	api := &fakeIMMAPI{data: map[uint32][]byte{}}
	app, before, cleanup := withFakeProjectionIME(t, backend, api)
	defer cleanup()
	activation := app.activateProjectionIME(nil, before)
	if activation.State != imeActivationActive || activation.Err != nil || backend.current != 100 || app.candidateGeometry.publish == nil || app.candidateGeometry.clear == nil {
		t.Fatalf("activation=%#v current=%d candidate=%#v", activation, backend.current, app.candidateGeometry)
	}
	if err := app.candidateGeometry.publishChanged(nativeCandidateRect{X: 1, Y: 2, Width: 3, Height: 4}); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	if backend.current != 9 || backend.registered != 0 || app.candidateGeometry.publish != nil || app.candidateGeometry.clear != nil {
		t.Fatalf("teardown current=%d registered=%d candidate=%#v", backend.current, backend.registered, app.candidateGeometry)
	}
	if len(api.log) < 6 || api.log[len(api.log)-3] != "acquire" || api.log[len(api.log)-2] != "candidate:clear" || api.log[len(api.log)-1] != "release" {
		t.Fatalf("candidate lifecycle=%v", api.log)
	}
}

func TestProjectionIMEInstallFailureFallsBackWithoutFailingWindow(t *testing.T) {
	installErr := errors.New("get WndProc")
	backend := &fakeWndProcBackend{current: 9, getErr: installErr}
	app, before, cleanup := withFakeProjectionIME(t, backend, &fakeIMMAPI{})
	defer cleanup()
	activation := app.activateProjectionIME(nil, before)
	if activation.State != imeActivationFallback || !errors.Is(activation.Err, installErr) || app.notice == "" {
		t.Fatalf("activation=%#v notice=%q", activation, app.notice)
	}
	if app.candidateGeometry.publish != nil || backend.current != 9 {
		t.Fatalf("fallback leaked native state: candidate=%#v current=%d", app.candidateGeometry, backend.current)
	}
	if err := before.close(); err != nil || backend.registered != 0 {
		t.Fatalf("fallback cleanup err=%v registered=%d", err, backend.registered)
	}
}

func TestProjectionIMECandidateClearFailureStillDetachesOwnership(t *testing.T) {
	clearErr := errors.New("clear")
	app := &App{}
	app.initCompositionCoordinator()
	before := newCompositionBeforeUnbind(app)
	if err := attachIMECandidateCleanup(app, before); err != nil {
		t.Fatal(err)
	}
	if err := app.setCandidateGeometryCallbacks(func(nativeCandidateRect) error { return nil }, func() error { return clearErr }); err != nil {
		t.Fatal(err)
	}
	if err := app.candidateGeometry.publishChanged(nativeCandidateRect{Width: 1, Height: 1}); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); !errors.Is(err, clearErr) {
		t.Fatalf("cleanup err=%v", err)
	}
	if app.candidateGeometry.publish != nil || app.candidateGeometry.clear != nil || app.candidateGeometry.wasVisible {
		t.Fatalf("failed clear retained ownership: %#v", app.candidateGeometry)
	}
}

func TestInitialProjectionAdoptionFailureCleansIMEOwnership(t *testing.T) {
	backend := &fakeWndProcBackend{current: 9}
	app, _, cleanup := withFakeProjectionIME(t, backend, &fakeIMMAPI{})
	defer cleanup()
	var window *glfw.Window
	app.controller = &windowController{windows: map[termmux.WindowID]*windowProjection{
		termmux.WindowID(initialWindowID): {id: termmux.WindowID(initialWindowID), host: window, app: app, bundle: &nativeProjectionBundle{}},
	}}
	if err := app.adoptInitialProjection(window); !errors.Is(err, errWindowProjectionExists) {
		t.Fatalf("adoption err=%v", err)
	}
	if backend.current != 9 || backend.registered != 0 || app.candidateGeometry.publish != nil || app.candidateGeometry.clear != nil {
		t.Fatalf("adoption rollback leaked IME ownership: current=%d registered=%d candidate=%#v", backend.current, backend.registered, app.candidateGeometry)
	}
}

func TestProjectionIMEAmbiguousInstallFallbackRetainsOwnershipUntilTeardown(t *testing.T) {
	rollbackErr := errors.New("rollback")
	backend := &fakeWndProcBackend{current: 9, setReturns: []uintptr{8, 100}, setErrors: []error{nil, rollbackErr}}
	app, before, cleanup := withFakeProjectionIME(t, backend, &fakeIMMAPI{})
	defer cleanup()
	activation := app.activateProjectionIME(nil, before)
	if activation.State != imeActivationFallback || !errors.Is(activation.Err, rollbackErr) || backend.registered != 5 {
		t.Fatalf("activation=%#v registered=%d", activation, backend.registered)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	if backend.current != 8 || backend.registered != 0 {
		t.Fatalf("ambiguous fallback cleanup current=%d registered=%d", backend.current, backend.registered)
	}
}
