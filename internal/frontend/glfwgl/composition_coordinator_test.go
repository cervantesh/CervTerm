//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
	"unicode/utf16"

	"cervterm/internal/ime"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
)

func utf16Text(text string) []uint16 { return utf16.Encode([]rune(text)) }

func TestCompositionCoordinatorCommitsOnceAndNeverWritesPreedit(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	app.initCompositionCoordinator()
	generation, err := app.composition.start()
	if err != nil {
		t.Fatal(err)
	}
	preedit := "にほん"
	if err := app.composition.update(generation, ime.NativeUpdate{UTF16: utf16Text(preedit), CursorUTF16: len(utf16Text(preedit))}); err != nil {
		t.Fatal(err)
	}
	if got := factory.sessions[0].text(); got != "" {
		t.Fatalf("preedit reached PTY: %q", got)
	}
	if err := app.composition.commit(generation, utf16Text("日本語😀")); err != nil {
		t.Fatal(err)
	}
	if got := factory.sessions[0].text(); got != "日本語😀" {
		t.Fatalf("commit=%q", got)
	}
	if err := app.composition.commit(generation, utf16Text("duplicate")); !errors.Is(err, ime.ErrInactive) || factory.sessions[0].text() != "日本語😀" {
		t.Fatalf("duplicate err=%v text=%q", err, factory.sessions[0].text())
	}
}

func TestCompositionCoordinatorMalformedPayloadCancelsAtomically(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	app.initCompositionCoordinator()
	generation, err := app.composition.start()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.composition.update(generation, ime.NativeUpdate{UTF16: []uint16{0xd800}, CursorUTF16: 1}); !errors.Is(err, ime.ErrInvalidUTF16) {
		t.Fatalf("malformed err=%v", err)
	}
	snapshot := app.composition.snapshot()
	if snapshot.Active || snapshot.LastCancel != ime.CancelMalformed || factory.sessions[0].text() != "" {
		t.Fatalf("snapshot=%#v text=%q", snapshot, factory.sessions[0].text())
	}
}

func TestCompositionCoordinatorReconcileAndDeliveryGate(t *testing.T) {
	target := ime.Target{Kind: ime.TargetPane, ID: 7, Activation: 9}
	var routed []string
	var coordinator compositionCoordinator
	coordinator.bind(func() (ime.Target, error) { return target, nil }, func(_ ime.Target, text string) error {
		routed = append(routed, text)
		return nil
	})
	generation, err := coordinator.start()
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.reconcile(target, nil, ime.CancelTargetChanged); err != nil || !coordinator.snapshot().Active {
		t.Fatalf("unchanged reconcile: snapshot=%#v err=%v", coordinator.snapshot(), err)
	}
	changed := target
	changed.Activation++
	if err := coordinator.reconcile(changed, nil, ime.CancelTargetChanged); err != nil {
		t.Fatal(err)
	}
	if snapshot := coordinator.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelTargetChanged {
		t.Fatalf("changed reconcile=%#v", snapshot)
	}
	coordinator.deactivateDelivery()
	if _, err := coordinator.start(); !errors.Is(err, errCompositionDeliveryInactive) {
		t.Fatalf("inactive start err=%v", err)
	}
	if err := coordinator.update(generation, ime.NativeUpdate{}); !errors.Is(err, errCompositionDeliveryInactive) {
		t.Fatalf("inactive update err=%v", err)
	}
	if len(routed) != 0 {
		t.Fatalf("inactive delivery routed %#v", routed)
	}
}

func TestCompositionCommitReportsRouteFailureWithoutRetry(t *testing.T) {
	routeErr := errors.New("route failed")
	calls := 0
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	var coordinator compositionCoordinator
	coordinator.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error {
		calls++
		return routeErr
	})
	generation, err := coordinator.start()
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.commit(generation, utf16Text("日本語")); !errors.Is(err, routeErr) {
		t.Fatalf("commit err=%v", err)
	}
	if snapshot := coordinator.snapshot(); snapshot.Active || calls != 1 {
		t.Fatalf("snapshot=%#v calls=%d", snapshot, calls)
	}
	if err := coordinator.commit(generation, utf16Text("retry")); !errors.Is(err, ime.ErrInactive) || calls != 1 {
		t.Fatalf("retry err=%v calls=%d", err, calls)
	}
}

func TestCompositionLifecycleModalSearchFocusAndMuxCancellation(t *testing.T) {
	newApp := func(t *testing.T) *App {
		t.Helper()
		app, _ := newRecordingActionApp(t)
		app.initCompositionCoordinator()
		return app
	}
	assertReason := func(t *testing.T, app *App, reason ime.CancelReason) {
		t.Helper()
		snapshot := app.composition.snapshot()
		if snapshot.Active || snapshot.LastCancel != reason {
			t.Fatalf("snapshot=%#v want reason=%v", snapshot, reason)
		}
	}

	t.Run("modal open", func(t *testing.T) {
		app := newApp(t)
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		if !app.openModal(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "x", Label: "x"}}) {
			t.Fatal("open failed")
		}
		assertReason(t, app, ime.CancelModalChanged)
	})

	t.Run("modal close", func(t *testing.T) {
		app := newApp(t)
		if !app.openModal(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "x", Label: "x"}}) {
			t.Fatal("open failed")
		}
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		app.closeModal()
		assertReason(t, app, ime.CancelModalChanged)
	})

	t.Run("modal replace", func(t *testing.T) {
		app := newApp(t)
		if !app.openModal(modal.ModeCommandPalette, modal.PaneIdentity(app.focusedPane), modal.FocusIdentity(app.focusedPane), []modal.Entry{{ID: "x", Label: "x"}}) {
			t.Fatal("open failed")
		}
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		if !app.replaceModal(modal.ModeLaunchMenu, []modal.Entry{{ID: "y", Label: "y"}}) {
			t.Fatal("replace failed")
		}
		assertReason(t, app, ime.CancelModalChanged)
	})

	t.Run("search open", func(t *testing.T) {
		app := newApp(t)
		app.search.redraw = func() {}
		app.search.bindActivationChange(func() { _ = app.cancelComposition(ime.CancelTargetChanged) })
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		if !app.search.open() {
			t.Fatal("search open failed")
		}
		assertReason(t, app, ime.CancelTargetChanged)
	})

	t.Run("search close", func(t *testing.T) {
		app := newApp(t)
		app.search.redraw = func() {}
		app.search.bindActivationChange(func() { _ = app.cancelComposition(ime.CancelTargetChanged) })
		if !app.search.open() {
			t.Fatal("search open failed")
		}
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		app.search.close()
		assertReason(t, app, ime.CancelTargetChanged)
	})

	t.Run("pane focus", func(t *testing.T) {
		app := newApp(t)
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		app.setFocusedPane(app.focusedPane + 1)
		assertReason(t, app, ime.CancelTargetChanged)
	})

	for _, test := range []struct {
		name  string
		event termmux.Event
	}{
		{name: "pane close", event: termmux.Event{Kind: termmux.PaneClosed}},
		{name: "pane transfer", event: termmux.Event{Kind: termmux.PaneTransferred}},
		{name: "window empty", event: termmux.Event{Kind: termmux.WindowTabsEmpty}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := newApp(t)
			if _, err := app.composition.start(); err != nil {
				t.Fatal(err)
			}
			event := test.event
			event.Pane = app.focusedPane
			app.cancelCompositionForMuxEvent(event)
			assertReason(t, app, ime.CancelTargetChanged)
		})
	}

	t.Run("unrelated lifecycle preserves composition", func(t *testing.T) {
		app := newApp(t)
		if _, err := app.composition.start(); err != nil {
			t.Fatal(err)
		}
		for _, event := range []termmux.Event{
			{Kind: termmux.TabClosed},
			{Kind: termmux.TabEmpty},
			{Kind: termmux.PaneTransferred, Pane: app.focusedPane + 1},
		} {
			app.cancelCompositionForMuxEvent(event)
		}
		if snapshot := app.composition.snapshot(); !snapshot.Active {
			t.Fatalf("unrelated event cancelled composition: %#v", snapshot)
		}
	})
}

func TestCompositionBeforeUnbindRunsExactOrderOnce(t *testing.T) {
	log := []string{}
	step := func(name string) func() error {
		return func() error { log = append(log, name); return nil }
	}
	host := &fakeNativeWindow{id: "ime", log: &log}
	bundle := &nativeProjectionBundle{
		host: host,
		beforeUnbind: &compositionBeforeUnbind{
			cancel: step("cancel"), deactivate: step("deactivate"), restore: step("restore"), release: step("release"),
		},
		unbind: step("unbind"),
		resources: []projectionResource{
			projectionResourceFunc(step("resource-1")),
			projectionResourceFunc(step("resource-2")),
		},
	}
	if err := bundle.close(); err != nil {
		t.Fatal(err)
	}
	if err := bundle.close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"cancel", "deactivate", "restore", "release", "unbind", "resource-2", "resource-1", "destroy:ime"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("order=%#v want=%#v", log, want)
	}
}

func TestCompositionBeforeUnbindSurvivesRestoreStyleEarlyUnbind(t *testing.T) {
	log := []string{}
	step := func(name string) func() error { return func() error { log = append(log, name); return nil } }
	host := &fakeNativeWindow{id: "restore", log: &log}
	bundle := &nativeProjectionBundle{
		host:         host,
		beforeUnbind: &compositionBeforeUnbind{cancel: step("cancel"), deactivate: step("deactivate")},
		unbind:       step("unbind"),
		resources:    []projectionResource{projectionResourceFunc(step("resource"))},
	}
	if err := bundle.unbindProjection(); err != nil {
		t.Fatal(err)
	}
	if err := bundle.close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"cancel", "deactivate", "unbind", "resource", "destroy:restore"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("order=%#v want=%#v", log, want)
	}
}
