//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/accessibility"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type fakeProjectionAccessibilityLifecycle struct {
	log           *[]string
	err           error
	invalidations int
	refreshes     int
}

func (lifecycle *fakeProjectionAccessibilityLifecycle) Close() error {
	*lifecycle.log = append(*lifecycle.log, "accessibility")
	return lifecycle.err
}

func (lifecycle *fakeProjectionAccessibilityLifecycle) Invalidate(intent accessibility.SemanticIntent) {
	if intent != accessibility.IntentNone {
		lifecycle.invalidations++
	}
}

func (lifecycle *fakeProjectionAccessibilityLifecycle) Refresh() error {
	lifecycle.refreshes++
	return lifecycle.err
}

func (*fakeProjectionAccessibilityLifecycle) Announce(accessibility.AnnouncementKind) error {
	return nil
}

func TestProjectionAccessibilityFactoryDormantTransferAndOrder(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	var log []string
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		log = append(log, "prepare")
		return &fakeProjectionAccessibilityLifecycle{log: &log}, nil
	}
	before := &compositionBeforeUnbind{
		cancel:     func() error { log = append(log, "cancel"); return nil },
		deactivate: func() error { log = append(log, "deactivate"); return nil },
		restore:    func() error { log = append(log, "restore"); return nil },
		release:    func() error { log = append(log, "release"); return nil },
	}
	app := &App{}
	app.cfg.Accessibility.Enabled = true
	if err := prepareProjectionAccessibility(app, new(glfw.Window), before); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "cancel", "accessibility", "deactivate", "restore", "release"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("order=%v want=%v", log, want)
	}
}

func TestProjectionAccessibilityFactoryFailureClosesPartial(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	injected := errors.New("prepare")
	var log []string
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		return &fakeProjectionAccessibilityLifecycle{log: &log}, injected
	}
	app := &App{}
	app.cfg.Accessibility.Enabled = true
	if err := prepareProjectionAccessibility(app, new(glfw.Window), &compositionBeforeUnbind{}); err != nil || app.accessibilityActivation.State != accessibilityActivationFallback || !errors.Is(app.accessibilityActivation.Err, injected) {
		t.Fatalf("activation=%#v err=%v", app.accessibilityActivation, err)
	}
	if !reflect.DeepEqual(log, []string{"accessibility"}) {
		t.Fatalf("cleanup=%v", log)
	}
}

func TestProjectionAccessibilityDisabledAndUnsupportedAreNonFatal(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	calls := 0
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		calls++
		return nil, errors.New("unexpected")
	}
	disabled := &App{}
	if err := prepareProjectionAccessibility(disabled, nil, nil); err != nil || calls != 0 || disabled.accessibilityActivation.State != accessibilityActivationDisabled {
		t.Fatalf("disabled activation=%#v calls=%d err=%v", disabled.accessibilityActivation, calls, err)
	}
	projectionAccessibilityFactory = nil
	unsupported := &App{}
	unsupported.cfg.Accessibility.Enabled = true
	if err := prepareProjectionAccessibility(unsupported, new(glfw.Window), &compositionBeforeUnbind{}); err != nil || unsupported.accessibilityActivation.State != accessibilityActivationUnsupported {
		t.Fatalf("unsupported activation=%#v err=%v", unsupported.accessibilityActivation, err)
	}
}

func TestProjectionAccessibilitySuppressesRepaintOnlyMuxEvents(t *testing.T) {
	app := newMuxTestApp(t, 8, 2)
	log := []string{}
	runtime := &fakeProjectionAccessibilityLifecycle{log: &log}
	app.accessibilityRuntime = runtime
	if !app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneDirty, Pane: app.focusedPane}}) {
		t.Fatal("pane dirty event was not consumed")
	}
	if runtime.invalidations != 0 || runtime.refreshes != 0 {
		t.Fatalf("repaint-only event invalidations=%d refreshes=%d", runtime.invalidations, runtime.refreshes)
	}
	app.applyMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: app.focusedPane, Data: []byte("x")}})
	if runtime.invalidations != 1 {
		t.Fatalf("semantic output invalidations=%d", runtime.invalidations)
	}
}

func TestProjectionAccessibilityRefreshFailureDisconnectsProvider(t *testing.T) {
	injected := errors.New("refresh")
	log := []string{}
	runtime := &fakeProjectionAccessibilityLifecycle{log: &log, err: injected}
	app := &App{accessibilityRuntime: runtime}
	app.refreshAccessibilityProjection()
	if app.accessibilityRuntime != nil || app.accessibilityActivation.State != accessibilityActivationFallback || !errors.Is(app.accessibilityActivation.Err, injected) {
		t.Fatalf("runtime=%v activation=%#v", app.accessibilityRuntime, app.accessibilityActivation)
	}
	if runtime.invalidations != 1 || runtime.refreshes != 1 || !reflect.DeepEqual(log, []string{"accessibility"}) {
		t.Fatalf("invalidations=%d refreshes=%d log=%v", runtime.invalidations, runtime.refreshes, log)
	}
}
