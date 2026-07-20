//go:build glfw

package glfwgl

import (
	"errors"

	"cervterm/internal/ime"
)

var errCompositionDeliveryInactive = errors.New("composition callback delivery is inactive")

type compositionCoordinator struct {
	model          ime.Controller
	capture        func() (ime.Target, error)
	route          func(ime.Target, string) error
	deliveryActive bool
	changed        func(ime.Snapshot)
}

func (coordinator *compositionCoordinator) bind(capture func() (ime.Target, error), route func(ime.Target, string) error) {
	coordinator.capture = capture
	coordinator.route = route
	coordinator.deliveryActive = capture != nil && route != nil
}

func (coordinator *compositionCoordinator) bindPresentation(changed func(ime.Snapshot)) {
	coordinator.changed = changed
}

func (coordinator *compositionCoordinator) notifyChanged() {
	if coordinator.changed != nil {
		coordinator.changed(coordinator.model.Snapshot())
	}
}

func (coordinator *compositionCoordinator) start() (uint64, error) {
	if !coordinator.deliveryActive || coordinator.capture == nil {
		return 0, errCompositionDeliveryInactive
	}
	target, err := coordinator.capture()
	if err != nil {
		return 0, err
	}
	generation, err := coordinator.model.Start(target)
	if err == nil {
		coordinator.notifyChanged()
	}
	return generation, err
}

func (coordinator *compositionCoordinator) update(generation uint64, update ime.NativeUpdate) error {
	if !coordinator.deliveryActive {
		return errCompositionDeliveryInactive
	}
	if err := coordinator.model.Update(generation, update); err != nil {
		if malformedCompositionError(err) {
			_ = coordinator.cancel(ime.CancelMalformed)
		}
		return err
	}
	coordinator.notifyChanged()
	return nil
}

func (coordinator *compositionCoordinator) commit(generation uint64, units []uint16) error {
	if !coordinator.deliveryActive || coordinator.route == nil {
		return errCompositionDeliveryInactive
	}
	commit, err := coordinator.model.Commit(generation, units)
	if err != nil {
		if malformedCompositionError(err) {
			_ = coordinator.cancel(ime.CancelMalformed)
		}
		return err
	}
	coordinator.notifyChanged()
	return coordinator.route(commit.Target, commit.Text)
}

func (coordinator *compositionCoordinator) cancel(reason ime.CancelReason) error {
	if !reason.Valid() {
		return ime.ErrInvalidCancelReason
	}
	snapshot := coordinator.model.Snapshot()
	if !snapshot.Active {
		return nil
	}
	err := coordinator.model.Cancel(snapshot.Generation, reason)
	if err == nil {
		coordinator.notifyChanged()
	}
	return err
}

func (coordinator *compositionCoordinator) reconcile(target ime.Target, targetErr error, reason ime.CancelReason) error {
	snapshot := coordinator.model.Snapshot()
	if !snapshot.Active {
		return nil
	}
	if targetErr == nil && target == snapshot.Target {
		return nil
	}
	return coordinator.cancel(reason)
}

func (coordinator *compositionCoordinator) deactivateDelivery() {
	coordinator.deliveryActive = false
}

func (coordinator *compositionCoordinator) snapshot() ime.Snapshot {
	return coordinator.model.Snapshot()
}

func malformedCompositionError(err error) bool {
	return errors.Is(err, ime.ErrInvalidUTF16) ||
		errors.Is(err, ime.ErrInvalidCursor) ||
		errors.Is(err, ime.ErrInvalidAttributes) ||
		errors.Is(err, ime.ErrPreeditLimit) ||
		errors.Is(err, ime.ErrCommitLimit) ||
		errors.Is(err, ime.ErrEmptyCommit)
}

func (a *App) initCompositionCoordinator() {
	a.composition.bind(a.captureCommittedTextTarget, a.routeCommittedText)
	a.composition.bindPresentation(func(ime.Snapshot) { a.requestRedraw() })
}

func (a *App) cancelComposition(reason ime.CancelReason) error {
	return a.composition.cancel(reason)
}

func (a *App) reconcileComposition(reason ime.CancelReason) error {
	target, err := a.currentCommittedTextTarget()
	return a.composition.reconcile(target, err, reason)
}
