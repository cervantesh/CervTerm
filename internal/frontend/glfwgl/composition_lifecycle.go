//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/ime"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
)

func (a *App) openModal(mode modal.Mode, pane modal.PaneIdentity, focus modal.FocusIdentity, entries []modal.Entry) bool {
	if !a.modal.Open(mode, pane, focus, entries) {
		return false
	}
	_ = a.cancelComposition(ime.CancelModalChanged)
	return true
}

func (a *App) replaceModal(mode modal.Mode, entries []modal.Entry) bool {
	if !a.modal.Replace(mode, entries) {
		return false
	}
	_ = a.cancelComposition(ime.CancelModalChanged)
	return true
}

func (a *App) closeModal() []modal.Intent {
	if !a.modal.Active() {
		return nil
	}
	intents := a.modal.Close()
	_ = a.cancelComposition(ime.CancelModalChanged)
	return intents
}

func (a *App) compositionNativeFocusChanged(focused bool) {
	if !focused {
		_ = a.cancelComposition(ime.CancelFocusLost)
		a.charSuppression.clearEcho()
	}
}

func (a *App) cancelCompositionForMuxEvent(event termmux.Event) {
	snapshot := a.composition.snapshot()
	if !snapshot.Active {
		return
	}
	switch event.Kind {
	case termmux.PaneFocused:
		if snapshot.Target.ID != uint64(event.Pane) {
			_ = a.cancelComposition(ime.CancelTargetChanged)
		}
	case termmux.PaneClosed, termmux.PaneTransferred:
		if snapshot.Target.ID == uint64(event.Pane) {
			_ = a.cancelComposition(ime.CancelTargetChanged)
		}
	case termmux.WindowTabsEmpty:
		_ = a.cancelComposition(ime.CancelTargetChanged)
	}
}

func (a *App) compositionTargetsPane(pane termmux.PaneID) bool {
	snapshot := a.composition.snapshot()
	return snapshot.Active && snapshot.Target.ID == uint64(pane)
}

func (a *App) compositionTargetsTab(tab termmux.TabID) bool {
	snapshot := a.composition.snapshot()
	if !snapshot.Active || a.mux == nil {
		return false
	}
	targetTab, ok := a.mux.TabForPane(termmux.PaneID(snapshot.Target.ID))
	return ok && targetTab == tab
}

type compositionBeforeUnbind struct {
	cancel     func() error
	deactivate func() error
	restore    func() error
	release    func() error
	done       bool
}

func (coordinator *compositionBeforeUnbind) close() error {
	if coordinator == nil || coordinator.done {
		return nil
	}
	coordinator.done = true
	// Teardown is deliberately at-most-once. Even when a native cleanup step
	// reports an error, retrying restored callbacks or released contexts is unsafe;
	// callers continue through unbind, resource closure, and HWND destruction.
	var joined error
	for _, step := range []func() error{coordinator.cancel, coordinator.deactivate, coordinator.restore, coordinator.release} {
		if step != nil {
			joined = errors.Join(joined, step())
		}
	}
	return joined
}

func newCompositionBeforeUnbind(app *App) *compositionBeforeUnbind {
	if app == nil {
		return nil
	}
	return &compositionBeforeUnbind{
		cancel: func() error { return app.cancelComposition(ime.CancelTeardown) },
		deactivate: func() error {
			app.composition.deactivateDelivery()
			app.charSuppression.clear()
			return nil
		},
	}
}
