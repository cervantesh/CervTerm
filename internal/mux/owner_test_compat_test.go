package mux

import (
	"cervterm/internal/core"
	"cervterm/internal/itermimage"
	"cervterm/internal/kitty"
	"cervterm/internal/layoutrestore"
	"cervterm/internal/pty"
	"cervterm/internal/sixel"
	"cervterm/internal/workscheduler"
	"runtime"
	"time"
)

// New is package-test-only. It returns the concrete mux for white-box reads,
// while every mutation below still acquires the real ephemeral Owner scope.
func New(factory SessionFactory, options Options) *Mux { return NewOwner(factory, options).mux }

func testOwnerForMux(m *Mux) *Owner {
	if m == nil {
		return nil
	}
	if m.owner != nil && m.owner.owner != nil {
		return m.owner.owner
	}
	runtime.LockOSThread()
	owner := &Owner{mux: m, generation: 1, threadID: currentOwnerThreadID()}
	owner.threadLocked.Store(true)
	owner.state.owner = owner
	owner.state.generation.Store(1)
	m.owner = &owner.state
	return owner
}

func testWindowOwnerForOwner(owner *Owner) *WindowOwner {
	window, err := owner.ForWindow(1, func(identity WindowIdentity) bool {
		return identity.ID == 1 && identity.Incarnation != 0
	})
	if err != nil {
		panic(err)
	}
	return window
}

func testMuxScope(m *Mux) (*Owner, mutationScope, error) {
	o := testOwnerForMux(m)
	if o == nil {
		return nil, mutationScope{}, ErrWrongOwner
	}
	scope, err := o.enter(m)
	return o, scope, err
}

func (s *imageDecodeScheduler) submitKitty(work kittyDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.job == nil {
		return errKittyDecodeInvalidWork
	}
	if s.owner != nil && work.owner.muxOwner == (ownerStamp{}) {
		work.owner.muxOwner = s.owner.currentOwnerStamp()
	}
	adapter := &kittyDecodeJobAdapter{job: work.job}
	return s.inner.Submit(workscheduler.Work[PaneID, imageDecodeOwner, workscheduler.Result]{Key: work.owner.paneID, Owner: imageDecodeOwner{protocol: imageDecodeKitty, value: work.owner}, Job: adapter})
}
func (s *imageDecodeScheduler) submit(work kittyDecodeWork) error { return s.submitKitty(work) }
func (s *imageDecodeScheduler) submitSixel(work sixelDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.job == nil {
		return errKittyDecodeInvalidWork
	}
	if s.owner != nil && work.owner.muxOwner == (ownerStamp{}) {
		work.owner.muxOwner = s.owner.currentOwnerStamp()
	}
	adapter := &sixelDecodeJobAdapter{job: work.job}
	return s.inner.Submit(workscheduler.Work[PaneID, imageDecodeOwner, workscheduler.Result]{Key: work.owner.paneID, Owner: imageDecodeOwner{protocol: imageDecodeSixel, value: work.owner}, Job: adapter})
}
func (s *imageDecodeScheduler) submitITerm(work itermDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.job == nil {
		return errKittyDecodeInvalidWork
	}
	if s.owner != nil && work.owner.muxOwner == (ownerStamp{}) {
		work.owner.muxOwner = s.owner.currentOwnerStamp()
	}
	adapter := &itermDecodeJobAdapter{job: work.job}
	return s.inner.Submit(workscheduler.Work[PaneID, imageDecodeOwner, workscheduler.Result]{Key: work.owner.paneID, Owner: imageDecodeOwner{protocol: imageDecodeITerm, value: work.owner}, Job: adapter})
}
func (s *imageDecodeScheduler) takeCompletion() (imageDecodeCompletion, bool) {
	if s == nil {
		return imageDecodeCompletion{}, false
	}
	return s.inner.TakeCompletion()
}
func (s *imageDecodeScheduler) finish(id PaneID) {
	if s != nil {
		s.inner.Finish(id)
	}
}
func (s *imageDecodeScheduler) close() {
	if s != nil {
		s.inner.Close()
	}
}

func testRegistryMux(r *localSessionRegistry) *Mux {
	if r == nil {
		return nil
	}
	if r.owner == nil {
		r.owner = &Mux{sessions: r, model: NewModel()}
	}
	return r.owner
}

func (r *localSessionRegistry) reserve(id PaneID) error {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return e
	}
	defer o.leave(s)
	return r.reserveScoped(s, id)
}
func (r *localSessionRegistry) release(id PaneID) {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return
	}
	defer o.leave(s)
	r.releaseScoped(s, id)
}
func (r *localSessionRegistry) spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return r.spawnScoped(s, rows, cols, options)
}
func (r *localSessionRegistry) register(p *pane) error {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return e
	}
	defer o.leave(s)
	return r.registerScoped(s, p)
}
func (r *localSessionRegistry) prepareStarts(ids []PaneID) (func(), error) {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return r.prepareStartsScoped(s, ids)
}
func (r *localSessionRegistry) start(id PaneID) error {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return e
	}
	defer o.leave(s)
	return r.startScoped(s, id)
}
func (r *localSessionRegistry) detach(id PaneID) detachResult {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return detachResult{}
	}
	defer o.leave(s)
	return r.detachScoped(s, id)
}
func (r *localSessionRegistry) abort(id PaneID, p *pane) detachResult {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return detachResult{}
	}
	defer o.leave(s)
	return r.abortScoped(s, id, p)
}
func (r *localSessionRegistry) prepareShutdown() (*preparedRegistryShutdown, error) {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return nil, e
	}
	prepared, err := r.prepareShutdownScoped(s)
	if err != nil {
		o.leave(s)
		return nil, err
	}
	if prepared.completed {
		o.leave(s)
	}
	return prepared, nil
}
func (r *localSessionRegistry) shutdownRegistry() error {
	o, s, e := testMuxScope(testRegistryMux(r))
	if e != nil {
		return e
	}
	defer o.leave(s)
	return r.shutdownRegistryScoped(s)
}

func (m *Mux) advancePane(p *pane, data []byte) []Event { return advancePaneForTest(m, p, data) }
func (m *Mux) processKittyOutcomes(p *pane) []Event     { return processKittyOutcomesForTest(m, p) }
func (m *Mux) processSixelOutcomes(p *pane)             { processSixelOutcomesForTest(m, p) }
func (m *Mux) processITermOutcomes(p *pane)             { processITermOutcomesForTest(m, p) }
func (m *Mux) submitKittyDecode(p *pane, outcome kitty.Outcome) {
	withTestMuxMutation(m, false, func(scope mutationScope) bool { m.submitKittyDecodeScoped(scope, p, outcome); return true })
}
func (m *Mux) submitSixelDecode(p *pane, outcome sixel.Outcome) {
	withTestMuxMutation(m, false, func(scope mutationScope) bool { m.submitSixelDecodeScoped(scope, p, outcome); return true })
}
func (m *Mux) submitITermDecode(p *pane, outcome itermimage.Outcome) {
	withTestMuxMutation(m, false, func(scope mutationScope) bool { m.submitITermDecodeScoped(scope, p, outcome); return true })
}
func (m *Mux) applyImageCompletion(c imageDecodeCompletion) []Event {
	return applyImageCompletionForTest(m, c)
}
func (m *Mux) applyKittyCompletion(c kittyDecodeCompletion) []Event {
	return applyKittyCompletionForTest(m, c)
}
func (m *Mux) applySixelCompletion(c sixelDecodeCompletion) []Event {
	return applySixelCompletionForTest(m, c)
}
func (m *Mux) applyITermCompletion(c itermDecodeCompletion) []Event {
	return applyITermCompletionForTest(m, c)
}
func (m *Mux) expireImages(now time.Time) []Event { return expireImagesForTest(m, now) }
func (m *Mux) expireKitty(now time.Time) []Event  { return expireImagesForTest(m, now) }

func withTestMuxMutation[T any](m *Mux, failed T, fn func(mutationScope) T) T {
	o, scope, err := testMuxScope(m)
	if err != nil {
		return failed
	}
	defer o.leave(scope)
	return fn(scope)
}

func buildRestoreCandidate(m *Mux, snapshot layoutrestore.Snapshot, geometries []RestoreWindowGeometry) (restoreBuild, error) {
	type result struct {
		build restoreBuild
		err   error
	}
	r := withTestMuxMutation(m, result{err: ErrWrongOwner}, func(scope mutationScope) result {
		build, err := buildRestoreCandidateScoped(scope, m, snapshot, geometries)
		return result{build: build, err: err}
	})
	return r.build, r.err
}

func advancePaneForTest(m *Mux, p *pane, data []byte) []Event {
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.advancePaneScoped(scope, p, data) })
}
func processKittyOutcomesForTest(m *Mux, p *pane) []Event {
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.processKittyOutcomesScoped(scope, p) })
}
func processSixelOutcomesForTest(m *Mux, p *pane) {
	withTestMuxMutation(m, false, func(scope mutationScope) bool { m.processSixelOutcomesScoped(scope, p); return true })
}
func processITermOutcomesForTest(m *Mux, p *pane) {
	withTestMuxMutation(m, false, func(scope mutationScope) bool { m.processITermOutcomesScoped(scope, p); return true })
}
func applyImageCompletionForTest(m *Mux, c imageDecodeCompletion) []Event {
	switch owner := c.Owner.value.(type) {
	case kittyDecodeOwner:
		if owner.muxOwner == (ownerStamp{}) {
			owner.muxOwner = m.currentOwnerStamp()
			c.Owner.value = owner
		}
	case sixelDecodeOwner:
		if owner.muxOwner == (ownerStamp{}) {
			owner.muxOwner = m.currentOwnerStamp()
			c.Owner.value = owner
		}
	case itermDecodeOwner:
		if owner.muxOwner == (ownerStamp{}) {
			owner.muxOwner = m.currentOwnerStamp()
			c.Owner.value = owner
		}
	}
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.applyImageCompletionScoped(scope, c) })
}
func applyKittyCompletionForTest(m *Mux, c kittyDecodeCompletion) []Event {
	if c.Owner.muxOwner == (ownerStamp{}) {
		c.Owner.muxOwner = m.currentOwnerStamp()
	}
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.applyKittyCompletionScoped(scope, c) })
}
func applySixelCompletionForTest(m *Mux, c sixelDecodeCompletion) []Event {
	if c.Owner.muxOwner == (ownerStamp{}) {
		c.Owner.muxOwner = m.currentOwnerStamp()
	}
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.applySixelCompletionScoped(scope, c) })
}
func applyITermCompletionForTest(m *Mux, c itermDecodeCompletion) []Event {
	if c.Owner.muxOwner == (ownerStamp{}) {
		c.Owner.muxOwner = m.currentOwnerStamp()
	}
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.applyITermCompletionScoped(scope, c) })
}
func expireImagesForTest(m *Mux, now time.Time) []Event {
	return withTestMuxMutation(m, []Event(nil), func(scope mutationScope) []Event { return m.expireImagesScoped(scope, now) })
}
func registryAbortForTest(m *Mux, id PaneID, p *pane) detachResult {
	return withTestMuxMutation(m, detachResult{}, func(scope mutationScope) detachResult { return m.sessions.abortScoped(scope, id, p) })
}

func (m *Mux) Bootstrap(spec SpawnSpec, content PixelRect, metrics CellMetrics) (TabID, PaneID, []Event, error) {
	o, s, err := testMuxScope(m)
	if err != nil {
		return 0, 0, nil, err
	}
	defer o.leave(s)
	return m.bootstrap(s, spec, content, metrics)
}
func (m *Mux) SpawnSplit(origin PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	o, s, err := testMuxScope(m)
	if err != nil {
		return 0, nil, err
	}
	defer o.leave(s)
	return m.spawnSplit(s, origin, axis, spec)
}
func (m *Mux) FocusPane(id PaneID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.focusPane(s, id)
}
func (m *Mux) FocusDirection(d Direction) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.focusDirection(s, d)
}
func (m *Mux) FocusNext(reverse bool) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.focusNext(s, reverse)
}
func (m *Mux) Write(id PaneID, data []byte) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.write(s, id, data)
}
func (m *Mux) FeedFallback(id PaneID, data []byte) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.feedFallback(s, id, data)
}
func (m *Mux) Drain(limit int) []Event {
	o, scope, err := testMuxScope(m)
	if err != nil {
		return nil
	}
	defer o.leave(scope)
	return m.drain(scope, limit)
}
func (m *Mux) ClosePane(id PaneID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.closePane(s, id)
}
func (m *Mux) SetPaletteBase(base core.PaletteBase) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return
	}
	defer o.leave(s)
	_ = m.setPaletteBase(s, base)
}
func (m *Mux) SpawnTab(spec SpawnSpec, metrics CellMetrics, title string) (TabID, PaneID, []Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return 0, 0, nil, e
	}
	defer o.leave(s)
	return m.spawnTab(s, spec, metrics, title)
}
func (m *Mux) ActivateTab(id TabID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.activateTab(s, id)
}
func (m *Mux) RenameTab(id TabID, title string) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.renameTab(s, id, title)
}
func (m *Mux) MoveTab(id TabID, pos int) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.moveTab(s, id, pos)
}
func (m *Mux) CloseTab(id TabID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.closeTab(s, id)
}
func (m *Mux) TransferPane(p PaneID, t TabID, d PaneID, a SplitAxis) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.transferPane(s, p, t, d, a)
}
func (m *Mux) Split(p PaneID, a SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	return m.SpawnSplit(p, a, spec)
}
func (m *Mux) ResizeCurrentPane(d Direction, n int) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.resizeCurrentPane(s, d, n)
}
func (m *Mux) SwapCurrentPane(d Direction) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.swapCurrentPane(s, d)
}
func (m *Mux) MoveCurrentPane(d Direction) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.moveCurrentPane(s, d)
}
func (m *Mux) SetSplitRatio(id SplitID, r SplitRatio) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.setSplitRatio(s, id, r)
}
func (m *Mux) Resize(b PixelRect, c CellMetrics) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.resize(s, b, c)
}
func (m *Mux) ResizeGrid(b PixelRect, c CellMetrics) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.resizeGrid(s, b, c)
}
func (m *Mux) ResizeBounds(b PixelRect) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.resizeBounds(s, b)
}
func (m *Mux) ResizePaneGrid(id PaneID, c CellMetrics) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.resizePaneGrid(s, id, c)
}
func (m *Mux) ApplyResize(id PaneID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.applyResize(s, id)
}
func (m *Mux) PrepareRestore(b layoutrestore.Blueprint, g []RestoreWindowGeometry) (*RestoreCandidate, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.prepareRestore(s, b, g)
}
func (m *Mux) CommitRestore(c *RestoreCandidate) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.commitRestore(s, c)
}
func (m *Mux) AbortRestore(c *RestoreCandidate) error {
	o, s, e := testMuxScope(m)
	if e != nil {
		return e
	}
	defer o.leave(s)
	return m.abortRestoreOwned(s, c)
}
func (m *Mux) CreateWindow(spec SpawnSpec, b PixelRect, c CellMetrics, title string) (WindowView, []Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return WindowView{}, nil, e
	}
	defer o.leave(s)
	return m.createWindow(s, spec, b, c, title)
}
func (m *Mux) ActivateWindow(id WindowID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.activateWindow(s, id)
}
func (m *Mux) CloseWindow(id WindowID) (CloseWindowResult, []Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return CloseWindowResult{}, nil, e
	}
	defer o.leave(s)
	return m.closeWindow(s, id)
}
func (m *Mux) RollbackWindow(id WindowID) error {
	o, s, e := testMuxScope(m)
	if e != nil {
		return e
	}
	defer o.leave(s)
	return m.rollbackWindow(s, id)
}
func (m *Mux) TransferPaneBetweenWindows(r PaneTransferRequest) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.transferPaneBetweenWindows(s, r)
}
func (m *Mux) TransferTabBetweenWindows(r TabTransferRequest) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.transferTabBetweenWindows(s, r)
}
func (m *Mux) CreateWorkspace(n string) (WorkspaceView, []Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return WorkspaceView{}, nil, e
	}
	defer o.leave(s)
	return m.createWorkspace(s, n)
}
func (m *Mux) RenameWorkspace(id WorkspaceID, n string) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.renameWorkspace(s, id, n)
}
func (m *Mux) SwitchWorkspace(id WorkspaceID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.switchWorkspace(s, id)
}
func (m *Mux) MoveWindowToWorkspace(w WindowID, id WorkspaceID) ([]Event, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return nil, e
	}
	defer o.leave(s)
	return m.moveWindowToWorkspace(s, w, id)
}
func (m *Mux) ScrollViewport(id PaneID, n int) (bool, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return false, e
	}
	defer o.leave(s)
	return m.scrollViewport(s, id, n)
}
func (m *Mux) ScrollViewportToGlobalRow(id PaneID, n int) (bool, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return false, e
	}
	defer o.leave(s)
	return m.scrollViewportToGlobalRow(s, id, n)
}
func (m *Mux) SetScrollbackCapacity(n int) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return
	}
	defer o.leave(s)
	_ = m.setScrollbackCapacity(s, n)
}
func (m *Mux) SetHideCursorWhenScrolled(v bool) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return
	}
	defer o.leave(s)
	_ = m.setHideCursorWhenScrolled(s, v)
}
func (m *Mux) SetTitle(id PaneID, title string) (bool, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return false, e
	}
	defer o.leave(s)
	return m.setTitle(s, id, title)
}
func (m *Mux) SearchUpward(id PaneID, q string, h bool, r int) (int, int, bool, error) {
	o, s, e := testMuxScope(m)
	if e != nil {
		return 0, 0, false, e
	}
	defer o.leave(s)
	return m.searchUpward(s, id, q, h, r)
}
func (m *Mux) Shutdown() error {
	o := testOwnerForMux(m)
	if o == nil {
		return ErrWrongOwner
	}
	return o.Shutdown()
}
