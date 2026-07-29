package mux

import (
	"time"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

// WindowAttestor is evaluated immediately before every dispatch. Production
// frontends must prove the exact native window/context and incarnation current.
type WindowAttestor func(WindowIdentity) bool

// WindowOwner is a stable projection capability. It carries only one WindowID;
// every call revalidates that ID and its pane/tab targets against live topology.
type WindowOwner struct {
	owner    *Owner
	identity WindowIdentity
	attest   WindowAttestor
}

func (o *Owner) ForWindow(window WindowID, attest WindowAttestor) (*WindowOwner, error) {
	if o == nil || window == 0 || attest == nil {
		return nil, ErrWrongOwner
	}
	if currentOwnerThreadID() != o.threadID {
		return nil, ErrWrongOwnerThread
	}
	state := o.mux.model.windowByID(window)
	if state == nil {
		return nil, ErrWindowNotFound
	}
	identity := WindowIdentity{ID: state.id, Incarnation: state.incarnation}
	return &WindowOwner{owner: o, identity: identity, attest: attest}, nil
}

func (w *WindowOwner) WindowID() WindowID {
	if w == nil {
		return 0
	}
	return w.identity.ID
}

// Identity returns the exact non-reusable window object identity.
func (w *WindowOwner) Identity() WindowIdentity {
	if w == nil {
		return WindowIdentity{}
	}
	return w.identity
}

func (w *WindowOwner) validate() error {
	if w == nil || w.owner == nil || w.identity.ID == 0 || w.identity.Incarnation == 0 || w.attest == nil {
		return ErrWrongOwner
	}
	if currentOwnerThreadID() != w.owner.threadID {
		return ErrWrongOwnerThread
	}
	if !w.attest(w.identity) {
		return ErrWrongOwnerThread
	}
	if w.owner.state.closed.Load() {
		return ErrOwnerClosed
	}
	state := w.owner.mux.model.windowByID(w.identity.ID)
	if state == nil || state.incarnation != w.identity.Incarnation {
		return ErrWrongOrigin
	}
	return nil
}

// beginWindowDispatch samples the native owner thread once, attests the exact
// current window/context on that thread, then creates the ephemeral scope from
// the same identity. Owner-internal work performs final live topology
// revalidation immediately before mutation.
func (w *WindowOwner) beginWindowDispatch() (mutationScope, error) {
	if w == nil || w.owner == nil || w.identity.ID == 0 || w.identity.Incarnation == 0 || w.attest == nil {
		return mutationScope{}, ErrWrongOwner
	}
	threadID := currentOwnerThreadID()
	if threadID == 0 || threadID != w.owner.threadID {
		return mutationScope{}, ErrWrongOwnerThread
	}
	if !w.attest(w.identity) {
		return mutationScope{}, ErrWrongOwnerThread
	}
	scope, err := w.owner.enterAttested(w.owner.mux, threadID)
	if err != nil {
		return mutationScope{}, err
	}
	scope.origin = w.identity
	return scope, nil
}

func (w *WindowOwner) validateActive() error {
	if err := w.validate(); err != nil {
		return err
	}
	if w.owner.mux.model.activeWindow != w.identity.ID {
		return ErrWrongOrigin
	}
	return nil
}

func (w *WindowOwner) validatePane(id PaneID) error {
	if err := w.validate(); err != nil {
		return err
	}
	window, ok := w.owner.mux.WindowForPane(id)
	if !ok || window != w.identity.ID {
		return ErrWrongOrigin
	}
	return nil
}

func (w *WindowOwner) validateTab(id TabID) error {
	if err := w.validate(); err != nil {
		return err
	}
	window, ok := w.owner.mux.WindowForTab(id)
	if !ok || window != w.identity.ID {
		return ErrWrongOrigin
	}
	return nil
}

func (w *WindowOwner) Bootstrap(spec SpawnSpec, content PixelRect, metrics CellMetrics) (TabID, PaneID, []Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return 0, 0, nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.bootstrap(scope, spec, content, metrics)
}

func (w *WindowOwner) SpawnSplit(origin PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return 0, nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.spawnSplit(scope, origin, axis, spec)
}

func (w *WindowOwner) Split(origin PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	return w.SpawnSplit(origin, axis, spec)
}

func (w *WindowOwner) FocusPane(id PaneID) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.focusPane(scope, id)
}

func (w *WindowOwner) FocusDirection(direction Direction) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.focusDirection(scope, direction)
}

func (w *WindowOwner) FocusNext(reverse bool) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.focusNext(scope, reverse)
}

func (w *WindowOwner) Write(id PaneID, data []byte) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.write(scope, id, data)
}

func (w *WindowOwner) FeedFallback(id PaneID, data []byte) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.feedFallbackOwned(scope, w.identity.ID, id, data)
}

func (w *WindowOwner) ClosePane(id PaneID) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.closePane(scope, id)
}

func (w *WindowOwner) SpawnTab(spec SpawnSpec, metrics CellMetrics, title string) (TabID, PaneID, []Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return 0, 0, nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.spawnTab(scope, spec, metrics, title)
}

func (w *WindowOwner) ActivateTab(id TabID) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.activateTab(scope, id)
}

func (w *WindowOwner) RenameTab(id TabID, title string) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.renameTab(scope, id, title)
}

func (w *WindowOwner) MoveTab(id TabID, position int) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.moveTab(scope, id, position)
}

func (w *WindowOwner) CloseTab(id TabID) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.closeTab(scope, id)
}

func (w *WindowOwner) TransferPane(pane PaneID, destinationTab TabID, destinationPane PaneID, axis SplitAxis) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.transferPane(scope, pane, destinationTab, destinationPane, axis)
}

func (w *WindowOwner) ResizeCurrentPane(direction Direction, delta int) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.resizeCurrentPane(scope, direction, delta)
}

func (w *WindowOwner) SwapCurrentPane(direction Direction) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.swapCurrentPane(scope, direction)
}

func (w *WindowOwner) MoveCurrentPane(direction Direction) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.moveCurrentPane(scope, direction)
}

func (w *WindowOwner) SetSplitRatio(split SplitID, ratio SplitRatio) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.setSplitRatio(scope, split, ratio)
}

func (w *WindowOwner) Resize(content PixelRect, metrics CellMetrics) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.resizeOwned(scope, w.identity.ID, content, metrics)
}

func (w *WindowOwner) ResizeGrid(content PixelRect, metrics CellMetrics) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.resizeGrid(scope, content, metrics)
}

func (w *WindowOwner) ResizeBounds(content PixelRect) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.resizeBounds(scope, content)
}

func (w *WindowOwner) ResizePaneGrid(id PaneID, metrics CellMetrics) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.resizePaneGrid(scope, id, metrics)
}

func (w *WindowOwner) ApplyResize(id PaneID) ([]Event, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return nil, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.applyResize(scope, id)
}

func (w *WindowOwner) ScrollViewport(id PaneID, lines int) (bool, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return false, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.scrollViewport(scope, id, lines)
}

func (w *WindowOwner) ScrollViewportToGlobalRow(id PaneID, row int) (bool, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return false, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.scrollViewportToGlobalRow(scope, id, row)
}

func (w *WindowOwner) SetTitle(id PaneID, title string) (bool, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return false, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.setTitle(scope, id, title)
}

func (w *WindowOwner) FocusedPane() (PaneID, bool) {
	if w.validate() != nil {
		return 0, false
	}
	return w.owner.mux.FocusedPane()
}
func (w *WindowOwner) PaneIDs() []PaneID {
	if w.validate() != nil {
		return nil
	}
	return w.owner.mux.PaneIDs()
}
func (w *WindowOwner) Layout() (Layout, error) {
	if err := w.validate(); err != nil {
		return Layout{}, err
	}
	return w.owner.mux.Layout()
}
func (w *WindowOwner) PaneView(id PaneID) (PaneView, bool) {
	if w.validatePane(id) != nil {
		return PaneView{}, false
	}
	return w.owner.mux.PaneView(id)
}
func (w *WindowOwner) Tabs() []TabView {
	if w.validate() != nil {
		return nil
	}
	return w.owner.mux.Tabs()
}
func (w *WindowOwner) ActiveTab() TabID {
	if w.validate() != nil {
		return 0
	}
	return w.owner.mux.ActiveTab()
}
func (w *WindowOwner) TabForPane(id PaneID) (TabID, bool) {
	if w.validatePane(id) != nil {
		return 0, false
	}
	return w.owner.mux.TabForPane(id)
}
func (w *WindowOwner) AcquireImageResource(id PaneID, ref termimage.ResourceRef) (termimage.DetachedResource, bool) {
	if w.validatePane(id) != nil {
		return termimage.DetachedResource{}, false
	}
	return w.owner.mux.AcquireImageResource(id, ref)
}
func (w *WindowOwner) SearchUpward(id PaneID, query string, hasPrev bool, prevRow int) (int, int, bool, error) {
	scope, err := w.beginWindowDispatch()
	if err != nil {
		return 0, 0, false, err
	}
	defer w.owner.leave(scope)
	return w.owner.mux.searchUpward(scope, id, query, hasPrev, prevRow)
}
func (w *WindowOwner) GlobalRowToViewport(id PaneID, row int) (int, bool) {
	if w.validatePane(id) != nil {
		return 0, false
	}
	return w.owner.mux.GlobalRowToViewport(id, row)
}
func (w *WindowOwner) Line(id PaneID, row int) (string, bool) {
	if w.validatePane(id) != nil {
		return "", false
	}
	return w.owner.mux.Line(id, row)
}
func (w *WindowOwner) LineWrapped(id PaneID, row int) (bool, bool) {
	if w.validatePane(id) != nil {
		return false, false
	}
	return w.owner.mux.LineWrapped(id, row)
}
func (w *WindowOwner) QuickSelectSnapshot(id PaneID, rowLimit, cellLimit int) (QuickSelectSnapshot, bool) {
	if w.validatePane(id) != nil {
		return QuickSelectSnapshot{}, false
	}
	return w.owner.mux.QuickSelectSnapshot(id, rowLimit, cellLimit)
}
func (w *WindowOwner) QuickSelectSnapshotCurrent(snapshot QuickSelectSnapshot) bool {
	if w.validatePane(snapshot.PaneID) != nil {
		return false
	}
	return w.owner.mux.QuickSelectSnapshotCurrent(snapshot)
}
func (w *WindowOwner) SemanticSnapshot(id PaneID) (SemanticSnapshot, bool) {
	if w.validatePane(id) != nil {
		return SemanticSnapshot{}, false
	}
	return w.owner.mux.SemanticSnapshot(id)
}
func (w *WindowOwner) SemanticSnapshotCurrent(snapshot SemanticSnapshot) bool {
	if w.validatePane(snapshot.PaneID) != nil {
		return false
	}
	return w.owner.mux.SemanticSnapshotCurrent(snapshot)
}
func (w *WindowOwner) ValidateSemanticRange(snapshot SemanticSnapshot, target core.SemanticRange) error {
	if err := w.validatePane(snapshot.PaneID); err != nil {
		return err
	}
	return w.owner.mux.ValidateSemanticRange(snapshot, target)
}
func (w *WindowOwner) SemanticRangeText(snapshot SemanticSnapshot, target core.SemanticRange) (string, error) {
	if err := w.validatePane(snapshot.PaneID); err != nil {
		return "", err
	}
	return w.owner.mux.SemanticRangeText(snapshot, target)
}
func (w *WindowOwner) ReplyCounters(id PaneID) (ReplyCounters, bool) {
	if w.validatePane(id) != nil {
		return ReplyCounters{}, false
	}
	return w.owner.mux.ReplyCounters(id)
}
func (w *WindowOwner) NextImageDeadline() (time.Time, bool) {
	if w.validate() != nil {
		return time.Time{}, false
	}
	return w.owner.mux.NextImageDeadline()
}

func (w *WindowOwner) ImageSetupError() error {
	if w == nil || w.owner == nil {
		return ErrWrongOwner
	}
	return w.owner.ImageSetupError()
}
