package mux

import (
	"errors"
	"reflect"
	"runtime"
	"testing"

	"cervterm/internal/layoutrestore"
	"cervterm/internal/termimage"
)

func closeOwnerForTest(owner *Owner) {
	if owner == nil {
		return
	}
	if owner.Closed() {
		_ = owner.mux.Shutdown()
	}
	_ = owner.Shutdown()
}

func TestMuxSinglePaneLayoutMatchesModelProjection(t *testing.T) {
	spawnErr := errors.New("single-pane layout")
	m := New(&fakeFactory{err: spawnErr}, Options{})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(err, spawnErr) {
		t.Fatalf("bootstrap err=%v", err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	want, err := m.model.LayoutWithMetrics(m.bounds, m.resolveMetrics)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("single-pane layout=%#v want=%#v", got, want)
	}
}

func TestOwnerCapabilityRejectsWrongCopyBusyClosedAndStale(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	other := NewOwner(&fakeFactory{}, Options{})
	t.Cleanup(func() {
		closeOwnerForTest(owner)
		closeOwnerForTest(other)
	})

	if _, err := owner.enter(other.mux); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("wrong owner error=%v", err)
	}

	stamp, err := owner.enter(owner.mux)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.enter(owner.mux); !errors.Is(err, ErrOwnerBusy) {
		t.Fatalf("reentrant owner error=%v", err)
	}
	if err := stamp.valid(owner.mux); err != nil {
		t.Fatalf("active stamp error=%v", err)
	}
	owner.leave(stamp)

	stale := ownerStamp{state: &owner.state, generation: owner.generation}
	owner.publishClosed(nil)
	if _, err := owner.enter(owner.mux); !errors.Is(err, ErrOwnerClosed) {
		t.Fatalf("closed owner error=%v", err)
	}
	if err := stale.valid(owner.mux); !errors.Is(err, ErrStaleOwner) {
		t.Fatalf("stale stamp error=%v", err)
	}
}

func TestOwnerCapabilityMisusePreservesMuxState(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	other := NewOwner(&fakeFactory{}, Options{})
	t.Cleanup(func() {
		closeOwnerForTest(owner)
		closeOwnerForTest(other)
	})
	before := owner.mux.Workspaces()
	if _, err := other.enter(owner.mux); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("wrong owner error=%v", err)
	}
	after := owner.mux.Workspaces()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-owner state changed: before=%#v after=%#v", before, after)
	}
}

type l302PaneFingerprint struct {
	state                          PaneState
	owner                          ownerStamp
	geometry                       PaneGeometry
	contentGen, reflowGen, viewGen uint64
	desired, applied               any
	resizeErr, closeErr            string
	title, cwd                     string
	bellCount                      int
	notificationSeq                uint64
	view                           PaneView
	replies                        []replyEntry
	replyNext                      uint64
	replyStats                     ReplyCounters
	kittyOutcomes, kittyEvents     int
	sixelOutcomes, itermOutcomes   int
	imageUsage                     termimage.Usage
	imageEpoch                     termimage.StoreEpoch
	imageRefs                      []termimage.ResourceRef
	imageClosed                    bool
	imageGeneration, anchorGen     uint64
}

type l302RestoreFingerprint struct {
	present, committed, aborted bool
	windows                     []WindowID
	bounds                      PixelRect
	paneMetrics                 map[PaneID]CellMetrics
}

type l302MuxFingerprint struct {
	workspaces       []WorkspaceView
	windows          []WindowView
	tabs             []TabView
	panes            []PaneID
	views            map[PaneID]PaneView
	paneState        map[PaneID]l302PaneFingerprint
	activeWorkspace  WorkspaceID
	activeWindow     WindowID
	nextWorkspace    WorkspaceID
	nextWindow       WindowID
	nextTab          TabID
	nextPane         PaneID
	nextSplit        SplitID
	allocatedWindows map[WindowID]struct{}
	allocatedTabs    map[TabID]struct{}
	allocatedPanes   map[PaneID]struct{}
	allocatedSplits  map[SplitID]struct{}
	registryReserved map[PaneID]struct{}
	registryClosed   map[PaneID]struct{}
	registryStarted  map[PaneID]struct{}
	registryShutdown bool
	registryStopping bool
	incomingLen      int
	bounds           PixelRect
	paneMetrics      map[PaneID]CellMetrics
	palette          any
	bootstrapped     bool
	restore          l302RestoreFingerprint
	kittyPending     map[uint64]kittyDecodeOwner
	sixelPending     map[uint64]sixelDecodeOwner
	itermPending     map[uint64]itermDecodeOwner
	kittyToken       uint64
	sixelToken       uint64
	itermToken       uint64
	schedulerPresent bool
	schedulerReady   int
}

func l302CopySet[K comparable](source map[K]struct{}) map[K]struct{} {
	result := make(map[K]struct{}, len(source))
	for value := range source {
		result[value] = struct{}{}
	}
	return result
}

func l302CopyMap[K comparable, V any](source map[K]V) map[K]V {
	result := make(map[K]V, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func l302ErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func captureL302MuxFingerprint(m *Mux) l302MuxFingerprint {
	result := l302MuxFingerprint{
		workspaces: m.Workspaces(), windows: m.Windows(), tabs: m.Tabs(), panes: m.PaneIDs(),
		views: make(map[PaneID]PaneView), paneState: make(map[PaneID]l302PaneFingerprint),
		activeWorkspace: m.model.activeWorkspace, activeWindow: m.model.activeWindow,
		nextWorkspace: m.model.nextWorkspaceID, nextWindow: m.model.nextWindowID, nextTab: m.model.nextTabID, nextPane: m.model.nextPaneID, nextSplit: m.model.nextSplitID,
		allocatedWindows: l302CopySet(m.model.allocatedWindows), allocatedTabs: l302CopySet(m.model.allocatedTabs),
		allocatedPanes: l302CopySet(m.model.allocated), allocatedSplits: l302CopySet(m.model.allocatedSplits),
		bounds: m.bounds, paneMetrics: l302CopyMap(m.paneMetrics), palette: *m.paletteBase, bootstrapped: m.bootstrapped,
		kittyPending: l302CopyMap(m.kittyPending), sixelPending: l302CopyMap(m.sixelPending), itermPending: l302CopyMap(m.itermPending),
		kittyToken: m.kittyNextToken, sixelToken: m.sixelNextToken, itermToken: m.itermNextToken, schedulerPresent: m.imageScheduler != nil,
	}
	if m.pending != nil {
		result.restore = l302RestoreFingerprint{present: true, committed: m.pending.committed, aborted: m.pending.aborted, windows: append([]WindowID(nil), m.pending.windows...), bounds: m.pending.bounds, paneMetrics: l302CopyMap(m.pending.paneMetrics)}
	}
	if m.imageScheduler != nil {
		result.schedulerReady = len(m.imageScheduler.ready())
	}
	m.sessions.mu.Lock()
	result.registryReserved = l302CopySet(m.sessions.reserved)
	result.registryClosed = l302CopySet(m.sessions.closed)
	result.registryStarted = l302CopySet(m.sessions.started)
	result.registryShutdown = m.sessions.shutdown
	result.registryStopping = m.sessions.shuttingDown
	result.incomingLen = len(m.sessions.incoming)
	registered := l302CopyMap(m.sessions.panes)
	m.sessions.mu.Unlock()
	for id, pane := range registered {
		view, _ := m.PaneView(id)
		result.views[id] = view
		replies := make([]replyEntry, len(pane.replies.entries))
		for index := range pane.replies.entries {
			replies[index] = pane.replies.entries[index]
			replies[index].data = append([]byte(nil), pane.replies.entries[index].data...)
		}
		state := l302PaneFingerprint{
			state: pane.state, owner: pane.ownerStamp, geometry: pane.geometry, contentGen: pane.contentGen, reflowGen: pane.reflowGen, viewGen: pane.viewportGen,
			desired: pane.desiredSize, applied: pane.appliedSize, resizeErr: l302ErrorText(pane.resizeErr), closeErr: l302ErrorText(pane.closeErr),
			title: pane.title, cwd: pane.cwd, bellCount: pane.bellCount, notificationSeq: pane.notificationSeq, view: view,
			replies: replies, replyNext: pane.replies.next, replyStats: pane.replies.stats, kittyOutcomes: len(pane.kittyOutcomes), kittyEvents: len(pane.kittyEvents),
			sixelOutcomes: len(pane.sixelOutcomes), itermOutcomes: len(pane.itermOutcomes),
		}
		if pane.imageStore != nil {
			state.imageUsage, state.imageEpoch, state.imageRefs, state.imageClosed = pane.imageStore.Usage(), pane.imageStore.Epoch(), pane.imageStore.ResourceRefs(), pane.imageStore.Closed()
		}
		if pane.terminal != nil {
			state.imageGeneration, state.anchorGen = pane.terminal.ImageGeneration(), pane.terminal.ImageAnchorGeneration()
		}
		result.paneState[id] = state
	}
	return result
}

func TestOwnerConcurrentAndReentrantMutationFamiliesFailWithoutStateChange(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	window := testWindowOwnerForOwner(owner)
	if _, _, _, err := window.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown() })

	stamp, err := owner.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.leave(stamp)

	attempts := map[string]func() error{
		"window": func() error {
			_, _, err := owner.CreateWindow(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}, "blocked")
			return err
		},
		"tab": func() error {
			_, _, _, err := window.SpawnTab(SpawnSpec{}, CellMetrics{CellWidth: 8, CellHeight: 16}, "blocked")
			return err
		},
		"pane":                func() error { _, err := window.SetTitle(1, "blocked"); return err },
		"topology":            func() error { _, err := window.ResizeCurrentPane(FocusLeft, 1); return err },
		"focus":               func() error { _, err := window.FocusPane(1); return err },
		"resize":              func() error { _, err := window.ResizeBounds(PixelRect{Width: 640, Height: 400}); return err },
		"viewport":            func() error { _, err := window.ScrollViewport(1, 1); return err },
		"policy":              func() error { return owner.SetHideCursorWhenScrolled(true) },
		"session-ingress":     func() error { _, err := owner.Drain(1); return err },
		"restore":             func() error { _, err := owner.PrepareRestore(layoutrestore.Blueprint{}, nil); return err },
		"workspace":           func() error { _, _, err := owner.CreateWorkspace("blocked"); return err },
		"termimage":           func() error { _, err := window.FeedFallback(1, []byte("blocked")); return err },
		"protocol-scheduling": func() error { _, err := owner.Drain(0); return err },
		"close":               owner.Shutdown,
	}
	for family, attempt := range attempts {
		t.Run(family, func(t *testing.T) {
			before := captureL302MuxFingerprint(owner.mux)
			result := make(chan error, 1)
			go func() { result <- attempt() }()
			if err := <-result; !errors.Is(err, ErrWrongOwnerThread) {
				t.Fatalf("cross-thread error=%v want=%v", err, ErrWrongOwnerThread)
			}
			after := captureL302MuxFingerprint(owner.mux)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected mutation changed state: before=%#v after=%#v", before, after)
			}
		})
	}
	before := captureL302MuxFingerprint(owner.mux)
	if _, _, err := owner.CreateWorkspace("reentrant"); !errors.Is(err, ErrOwnerBusy) {
		t.Fatalf("same-thread reentrant error=%v", err)
	}
	if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(before, after) {
		t.Fatalf("same-thread reentrant mutation changed state: before=%#v after=%#v", before, after)
	}
}

func TestClosedOwnerMutationFailsWithoutStateChange(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	if _, _, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Shutdown(); err != nil {
		t.Fatal(err)
	}
	before := captureL302MuxFingerprint(owner.mux)
	if _, _, err := owner.CreateWorkspace("closed"); !errors.Is(err, ErrOwnerClosed) {
		t.Fatalf("closed mutation error=%v", err)
	}
	after := captureL302MuxFingerprint(owner.mux)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("closed mutation changed state: before=%#v after=%#v", before, after)
	}
	if err := owner.Shutdown(); err != nil {
		t.Fatalf("idempotent shutdown=%v", err)
	}
}

func TestMutationScopeCopiesLeakAndExpireFailClosed(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	t.Cleanup(func() { _ = owner.Shutdown() })
	scope, err := owner.begin()
	if err != nil {
		t.Fatal(err)
	}
	copyOfScope := scope
	if err := copyOfScope.valid(owner.mux); err != nil {
		t.Fatalf("active copied scope=%v", err)
	}
	crossThread := make(chan error, 1)
	go func() { crossThread <- copyOfScope.valid(owner.mux) }()
	if err := <-crossThread; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("leaked active scope error=%v", err)
	}
	owner.leave(scope)
	if err := scope.valid(owner.mux); !errors.Is(err, ErrStaleMutationScope) {
		t.Fatalf("stale original scope error=%v", err)
	}
	if err := copyOfScope.valid(owner.mux); !errors.Is(err, ErrStaleMutationScope) {
		t.Fatalf("stale copied scope error=%v", err)
	}
	before := captureL302MuxFingerprint(owner.mux)
	sequential := make(chan error, 1)
	go func() { _, _, err := owner.CreateWorkspace("cross-thread"); sequential <- err }()
	if err := <-sequential; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("sequential cross-thread mutation error=%v", err)
	}
	if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(before, after) {
		t.Fatalf("sequential cross-thread mutation changed state: before=%#v after=%#v", before, after)
	}
}

func TestWindowOwnerFeedFallbackAttestsOnceAtDispatchBoundary(t *testing.T) {
	spawnErr := errors.New("fallback-only pane")
	owner := NewOwner(&fakeFactory{err: spawnErr}, Options{})
	if _, pane, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(err, spawnErr) || pane != 1 {
		t.Fatalf("bootstrap pane=%d err=%v", pane, err)
	}
	t.Cleanup(func() { _ = owner.Shutdown() })
	attestations := 0
	window, err := owner.ForWindow(1, func(identity WindowIdentity) bool {
		attestations++
		return identity.ID == 1 && identity.Incarnation != 0
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := window.FeedFallback(1, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if attestations != 1 {
		t.Fatalf("window dispatch attestations=%d want=1", attestations)
	}
	if _, err := window.Resize(PixelRect{Width: 801, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	if attestations != 2 {
		t.Fatalf("two window dispatches attestations=%d want=2", attestations)
	}
}

func TestWindowOwnerRevalidatesOriginAgainstCurrentTopology(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	if _, _, _, err := testWindowOwnerForOwner(owner).Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	second, _, err := owner.CreateWindow(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Shutdown() })
	attest := func(identity WindowIdentity) bool { return identity.ID != 0 && identity.Incarnation != 0 }
	firstWindow, err := owner.ForWindow(1, attest)
	if err != nil {
		t.Fatal(err)
	}
	secondWindow, err := owner.ForWindow(second.ID, attest)
	if err != nil {
		t.Fatal(err)
	}
	secondPane := second.Tabs[0].Focused
	before := captureL302MuxFingerprint(owner.mux)
	if _, err := firstWindow.Write(secondPane, []byte("blocked")); !errors.Is(err, ErrWrongOrigin) {
		t.Fatalf("cross-window pane origin error=%v", err)
	}
	if _, err := firstWindow.FeedFallback(secondPane, []byte("blocked")); !errors.Is(err, ErrWrongOrigin) {
		t.Fatalf("cross-window fallback origin error=%v", err)
	}
	if _, err := firstWindow.ResizeBounds(PixelRect{Width: 640, Height: 400}); !errors.Is(err, ErrWrongOrigin) {
		t.Fatalf("inactive window origin error=%v", err)
	}
	if after := captureL302MuxFingerprint(owner.mux); !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-origin mutation changed state: before=%#v after=%#v", before, after)
	}
	if changed, err := secondWindow.SetTitle(secondPane, "owned"); err != nil || !changed {
		t.Fatalf("owned mutation changed=%v err=%v", changed, err)
	}
	if _, err := owner.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	before = captureL302MuxFingerprint(owner.mux)
	if _, err := secondWindow.ResizeBounds(PixelRect{Width: 600, Height: 360}); !errors.Is(err, ErrWrongOrigin) {
		t.Fatalf("stale active-origin error=%v", err)
	}
	if _, _, err := owner.CloseWindow(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := secondWindow.Write(secondPane, []byte("stale")); !errors.Is(err, ErrWrongOrigin) {
		t.Fatalf("closed topology origin error=%v", err)
	}
}

func l302OwnerMutationInvokers() map[string]func(*Owner) error {
	return map[string]func(*Owner) error{
		"Drain":          func(o *Owner) error { _, err := o.Drain(1); return err },
		"SetPaletteBase": func(o *Owner) error { return o.SetPaletteBase(*o.mux.paletteBase) },
		"PrepareRestore": func(o *Owner) error { _, err := o.PrepareRestore(layoutrestore.Blueprint{}, nil); return err },
		"CommitRestore":  func(o *Owner) error { _, err := o.CommitRestore(nil); return err },
		"AbortRestore":   func(o *Owner) error { return o.AbortRestore(nil) },
		"CreateWindow": func(o *Owner) error {
			_, _, err := o.CreateWindow(SpawnSpec{}, PixelRect{Width: 80, Height: 24}, CellMetrics{CellWidth: 1, CellHeight: 1}, "x")
			return err
		},
		"ActivateWindow":             func(o *Owner) error { _, err := o.ActivateWindow(1); return err },
		"CloseWindow":                func(o *Owner) error { _, _, err := o.CloseWindow(1); return err },
		"RollbackWindow":             func(o *Owner) error { return o.RollbackWindow(1) },
		"TransferPaneBetweenWindows": func(o *Owner) error { _, err := o.TransferPaneBetweenWindows(PaneTransferRequest{}); return err },
		"TransferTabBetweenWindows":  func(o *Owner) error { _, err := o.TransferTabBetweenWindows(TabTransferRequest{}); return err },
		"CreateWorkspace":            func(o *Owner) error { _, _, err := o.CreateWorkspace("x"); return err },
		"RenameWorkspace":            func(o *Owner) error { _, err := o.RenameWorkspace(1, "x"); return err },
		"SwitchWorkspace":            func(o *Owner) error { _, err := o.SwitchWorkspace(1); return err },
		"MoveWindowToWorkspace":      func(o *Owner) error { _, err := o.MoveWindowToWorkspace(1, 1); return err },
		"SetScrollbackCapacity":      func(o *Owner) error { return o.SetScrollbackCapacity(100) },
		"SetHideCursorWhenScrolled":  func(o *Owner) error { return o.SetHideCursorWhenScrolled(true) },
		"Shutdown":                   func(o *Owner) error { return o.Shutdown() },
		"Close":                      func(o *Owner) error { return o.Close() },
	}
}

func TestOwnerDispatchFastPathIsLockFreeAndAllocationFree(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	defer owner.Shutdown()
	allocations := testing.AllocsPerRun(1000, func() {
		scope, err := owner.begin()
		if err != nil {
			panic(err)
		}
		if err := scope.valid(owner.mux); err != nil {
			panic(err)
		}
		owner.leave(scope)
	})
	if allocations != 0 {
		t.Fatalf("owner dispatch allocations=%v want=0", allocations)
	}
}

func TestOwnerConcurrentShutdownDoesNotRaceThreadRelease(t *testing.T) {
	owner := NewOwner(&fakeFactory{}, Options{})
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		for !owner.Closed() {
			_ = owner.Shutdown()
			runtime.Gosched()
		}
		done <- owner.Shutdown()
	}()
	close(start)
	if err := owner.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if owner.threadLocked.Load() {
		t.Fatal("owner thread remained locked after shutdown")
	}
}
