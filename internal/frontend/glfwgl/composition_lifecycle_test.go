//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func bindStaticComposition(app *App, target ime.Target) {
	app.composition.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
}

func TestCompositionCancelsOnNativeAndProjectionFocusLoss(t *testing.T) {
	t.Run("native focus", func(t *testing.T) {
		app := &App{}
		bindStaticComposition(app, ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1})
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		app.compositionNativeFocusChanged(true)
		if !app.composition.snapshot().Active {
			t.Fatal("focus gain cancelled composition")
		}
		app.compositionNativeFocusChanged(false)
		if snapshot := app.composition.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelFocusLost {
			t.Fatalf("snapshot=%#v", snapshot)
		}
	})

	t.Run("projection focus", func(t *testing.T) {
		var log []string
		controller := newWindowController(processServices{}, fakeNativePump{log: &log})
		first, second := &App{}, &App{}
		bindStaticComposition(first, ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1})
		bindStaticComposition(second, ime.Target{Kind: ime.TargetPane, ID: 2, Activation: 1})
		if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, first, func([]termmux.Event) bool { return true }); err != nil {
			t.Fatal(err)
		}
		if err := controller.attachApp(2, &fakeNativeWindow{id: "two", log: &log}, second, func([]termmux.Event) bool { return true }); err != nil {
			t.Fatal(err)
		}
		if err := controller.startLoop(); err != nil {
			t.Fatal(err)
		}
		if _, err := first.composition.start(); err != nil {
			t.Fatal(err)
		}
		if err := controller.focus(2); err != nil {
			t.Fatal(err)
		}
		if snapshot := first.composition.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelFocusLost {
			t.Fatalf("first snapshot=%#v", snapshot)
		}
	})
}

func TestCompositionCancelsBeforeWorkspaceHide(t *testing.T) {
	muxApp := newRunningMuxTestApp(t)
	second, _, err := muxApp.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _, err := muxApp.mux.CreateWorkspace("work")
	if err != nil {
		t.Fatal(err)
	}
	moveEvents, err := muxApp.mux.MoveWindowToWorkspace(second.ID, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	firstApp, secondApp := &App{}, &App{}
	bindStaticComposition(firstApp, ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1})
	bindStaticComposition(secondApp, ime.Target{Kind: ime.TargetPane, ID: uint64(second.Tabs[0].Focused), Activation: 1})
	controller := newWindowController(processServices{mux: muxApp.mux}, fakeNativePump{log: &log})
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, firstApp, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(second.ID, &fakeNativeWindow{id: "two", log: &log}, secondApp, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(moveEvents)
	if _, err := firstApp.composition.start(); err != nil {
		t.Fatal(err)
	}
	switchEvents, err := muxApp.mux.SwitchWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller.dispatch(switchEvents)
	if snapshot := firstApp.composition.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelWindowHidden {
		t.Fatalf("snapshot=%#v log=%v", snapshot, log)
	}
}

func TestProjectionCloseCancelsAndDeactivatesComposition(t *testing.T) {
	var log []string
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	app := &App{}
	bindStaticComposition(app, ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1})
	host := &fakeNativeWindow{id: "close", log: &log}
	if err := controller.attachApp(1, host, app, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	bundle := &nativeProjectionBundle{host: host, app: app, handle: func([]termmux.Event) bool { return true }, beforeUnbind: newCompositionBeforeUnbind(app)}
	if err := controller.adoptProjectionBundle(1, bundle); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.composition.start(); err != nil {
		t.Fatal(err)
	}
	if err := controller.closeProjection(1); err != nil {
		t.Fatal(err)
	}
	snapshot := app.composition.snapshot()
	if snapshot.Active || snapshot.LastCancel != ime.CancelTeardown || app.composition.deliveryActive {
		t.Fatalf("snapshot=%#v delivery=%v", snapshot, app.composition.deliveryActive)
	}
	if _, err := app.composition.start(); err != errCompositionDeliveryInactive {
		t.Fatalf("post-close start err=%v", err)
	}
}
