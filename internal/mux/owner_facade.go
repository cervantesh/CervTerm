package mux

import (
	"runtime"

	"cervterm/internal/core"
	"cervterm/internal/layoutrestore"
)

func (o *Owner) begin() (mutationScope, error) {
	if o == nil {
		return mutationScope{}, ErrWrongOwner
	}
	return o.enter(o.mux)
}

func (o *Owner) Drain(limit int) ([]Event, error) {
	scope, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(scope)
	return o.mux.drain(scope, limit), nil
}
func (o *Owner) SetPaletteBase(base core.PaletteBase) error {
	stamp, err := o.begin()
	if err != nil {
		return err
	}
	defer o.leave(stamp)
	return o.mux.setPaletteBase(stamp, base)
}

func (o *Owner) PrepareRestore(blueprint layoutrestore.Blueprint, geometries []RestoreWindowGeometry) (*RestoreCandidate, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.prepareRestore(stamp, blueprint, geometries)
}
func (o *Owner) CommitRestore(candidate *RestoreCandidate) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.commitRestore(stamp, candidate)
}
func (o *Owner) AbortRestore(candidate *RestoreCandidate) error {
	stamp, err := o.begin()
	if err != nil {
		return err
	}
	defer o.leave(stamp)
	return o.mux.abortRestoreOwned(stamp, candidate)
}
func (o *Owner) RestoreWindowIDs(candidate *RestoreCandidate) ([]WindowID, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.RestoreWindowIDs(candidate)
}

func (o *Owner) CreateWindow(spec SpawnSpec, content PixelRect, metrics CellMetrics, title string) (WindowView, []Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return WindowView{}, nil, err
	}
	defer o.leave(stamp)
	return o.mux.createWindow(stamp, spec, content, metrics, title)
}
func (o *Owner) ActivateWindow(id WindowID) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.activateWindow(stamp, id)
}
func (o *Owner) CloseWindow(id WindowID) (CloseWindowResult, []Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return CloseWindowResult{}, nil, err
	}
	defer o.leave(stamp)
	return o.mux.closeWindow(stamp, id)
}
func (o *Owner) RollbackWindow(id WindowID) error {
	stamp, err := o.begin()
	if err != nil {
		return err
	}
	defer o.leave(stamp)
	return o.mux.rollbackWindow(stamp, id)
}
func (o *Owner) TransferPaneBetweenWindows(req PaneTransferRequest) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.transferPaneBetweenWindows(stamp, req)
}
func (o *Owner) TransferTabBetweenWindows(req TabTransferRequest) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.transferTabBetweenWindows(stamp, req)
}

func (o *Owner) CreateWorkspace(name string) (WorkspaceView, []Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return WorkspaceView{}, nil, err
	}
	defer o.leave(stamp)
	return o.mux.createWorkspace(stamp, name)
}
func (o *Owner) RenameWorkspace(id WorkspaceID, name string) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.renameWorkspace(stamp, id, name)
}
func (o *Owner) SwitchWorkspace(id WorkspaceID) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.switchWorkspace(stamp, id)
}
func (o *Owner) MoveWindowToWorkspace(window WindowID, target WorkspaceID) ([]Event, error) {
	stamp, err := o.begin()
	if err != nil {
		return nil, err
	}
	defer o.leave(stamp)
	return o.mux.moveWindowToWorkspace(stamp, window, target)
}

func (o *Owner) SetScrollbackCapacity(capacity int) error {
	stamp, err := o.begin()
	if err != nil {
		return err
	}
	defer o.leave(stamp)
	return o.mux.setScrollbackCapacity(stamp, capacity)
}
func (o *Owner) SetHideCursorWhenScrolled(hide bool) error {
	stamp, err := o.begin()
	if err != nil {
		return err
	}
	defer o.leave(stamp)
	return o.mux.setHideCursorWhenScrolled(stamp, hide)
}

func (o *Owner) Shutdown() error {
	if o == nil {
		return ErrWrongOwner
	}
	if o.state.closed.Load() {
		if currentOwnerThreadID() == o.threadID && o.threadLocked.CompareAndSwap(true, false) {
			runtime.UnlockOSThread()
		}
		return o.state.closeErr
	}
	scope, err := o.begin()
	if err != nil {
		return err
	}
	closeErr := o.mux.shutdown(scope)
	if !o.mux.shutdownCommitted() {
		o.leave(scope)
		return closeErr
	}
	o.publishClosed(closeErr)
	o.leave(scope)
	if o.threadLocked.CompareAndSwap(true, false) {
		runtime.UnlockOSThread()
	}
	return closeErr
}
func (o *Owner) Close() error { return o.Shutdown() }

// Detached read projections retain their existing value semantics. They do not
// enter the mutation gate and callers must not invoke them concurrently with a
// mutation; returned values remain detached and safe to retain.
func (o *Owner) Windows() []WindowView                    { return o.mux.Windows() }
func (o *Owner) WindowForPane(id PaneID) (WindowID, bool) { return o.mux.WindowForPane(id) }
func (o *Owner) WindowForTab(id TabID) (WindowID, bool)   { return o.mux.WindowForTab(id) }
func (o *Owner) WorkspaceForWindow(id WindowID) (WorkspaceID, bool) {
	return o.mux.WorkspaceForWindow(id)
}
func (o *Owner) Workspaces() []WorkspaceView    { return o.mux.Workspaces() }
func (o *Owner) ActiveWorkspace() WorkspaceView { return o.mux.ActiveWorkspace() }
func (o *Owner) FreshSessionSnapshot() (FreshSessionSnapshot, error) {
	return o.mux.FreshSessionSnapshot()
}
func (o *Owner) ResolveEventAddresses(events []Event) []Event {
	return o.mux.ResolveEventAddresses(events)
}
func (o *Owner) ImageSetupError() error { return o.mux.ImageSetupError() }
