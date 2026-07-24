package mux

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"cervterm/internal/pty"
	"cervterm/internal/termimage"
)

type restoreObservationSession struct {
	*fakeSession
	id       PaneID
	onReader func()
	onClose  func(PaneID)
}

func (s *restoreObservationSession) Reader() io.Reader {
	if s.onReader != nil {
		s.onReader()
	}
	return s.fakeSession.Reader()
}

func (s *restoreObservationSession) Close() error {
	if s.onClose != nil {
		s.onClose(s.id)
	}
	return s.fakeSession.Close()
}

type restoreObservationFactory struct {
	trace      []string
	closeOrder []PaneID
	sessions   []*restoreObservationSession
	onSpawn    func(int)
	onReader   func(int)
}

func (f *restoreObservationFactory) Spawn(_, _ uint16, options pty.Options) (pty.Session, error) {
	index := len(f.sessions) + 1
	session := &restoreObservationSession{
		fakeSession: newFakeSession(),
		id:          restoreMatrixPaneID(options.ShellProgram),
	}
	session.onReader = func() {
		f.trace = append(f.trace, fmt.Sprintf("reader:%d", index))
		if f.onReader != nil {
			f.onReader(index)
		}
	}
	session.onClose = func(id PaneID) { f.closeOrder = append(f.closeOrder, id) }
	f.sessions = append(f.sessions, session)
	f.trace = append(f.trace, fmt.Sprintf("spawn:%d", index))
	if f.onSpawn != nil {
		f.onSpawn(index)
	}
	return session, nil
}

func restoreMatrixPaneID(program string) PaneID {
	if len(program) != 1 || program[0] < 'a' || program[0] > 'g' {
		return 0
	}
	return PaneID(program[0]-'a') + 2
}

func TestMuxRestoreSpawnsBeforePublicationAndReadersObserveWholeCommit(t *testing.T) {
	factory := &restoreObservationFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	before := m.model
	readerCalls := 0

	factory.onSpawn = func(index int) {
		if m.model != before || m.bootstrapped {
			t.Fatalf("spawn %d observed published mux", index)
		}
		if m.pending == nil {
			t.Fatalf("spawn %d observed no pending candidate", index)
		}
		if readerCalls != 0 {
			t.Fatalf("spawn %d followed %d reader calls", index, readerCalls)
		}
	}

	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	factory.onReader = func(index int) {
		readerCalls++
		if len(factory.sessions) != len(candidate.panes) {
			t.Fatalf("reader %d saw %d/%d spawns", index, len(factory.sessions), len(candidate.panes))
		}
		if m.pending != nil || !candidate.committed || candidate.aborted || !m.bootstrapped || m.model != candidate.model || m.bounds != candidate.bounds || !reflect.DeepEqual(m.paneMetrics, candidate.paneMetrics) {
			t.Fatalf("reader %d observed partial publication: pending=%p committed=%v aborted=%v bootstrapped=%v model=%p/%p bounds=%#v/%#v", index, m.pending, candidate.committed, candidate.aborted, m.bootstrapped, m.model, candidate.model, m.bounds, candidate.bounds)
		}
		panes, reserved, started := m.sessions.activeCounts()
		if panes != len(candidate.panes) || reserved != 0 || started != len(candidate.panes) {
			t.Fatalf("reader %d ownership counts=%d/%d/%d", index, panes, reserved, started)
		}
		for _, pane := range candidate.panes {
			owned, ok := m.sessions.lookup(pane.id)
			if !ok || owned != pane {
				t.Fatalf("reader %d pane %d not fully published", index, pane.id)
			}
			if _, visible := m.PaneView(pane.id); !visible {
				t.Fatalf("reader %d pane %d not publicly visible", index, pane.id)
			}
		}
	}

	if readerCalls != 0 {
		t.Fatalf("prepare called %d readers", readerCalls)
	}
	if _, err = m.CommitRestore(candidate); err != nil {
		t.Fatal(err)
	}
	wantTrace := make([]string, 0, 14)
	for index := 1; index <= 7; index++ {
		wantTrace = append(wantTrace, fmt.Sprintf("spawn:%d", index))
	}
	for index := 1; index <= 7; index++ {
		wantTrace = append(wantTrace, fmt.Sprintf("reader:%d", index))
	}
	if !reflect.DeepEqual(factory.trace, wantTrace) {
		t.Fatalf("restore trace=%v want=%v", factory.trace, wantTrace)
	}
}

func TestMuxRestoreReaderPreparationFailureReverseAbortsPristineAndRetriesIDs(t *testing.T) {
	factory := &restoreObservationFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	before := m.model
	beforeBounds, beforeMetrics := m.bounds, m.paneMetrics
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())

	candidate, err := m.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	m.sessions.mu.Lock()
	m.sessions.started[candidate.panes[3].id] = struct{}{}
	m.sessions.mu.Unlock()

	events, err := m.CommitRestore(candidate)
	if err == nil || len(events) != 0 || !strings.Contains(err.Error(), "reader is already started") {
		t.Fatalf("reader preparation failure events=%#v err=%v", events, err)
	}
	if !candidate.aborted || candidate.committed {
		t.Fatalf("failed candidate committed=%v aborted=%v", candidate.committed, candidate.aborted)
	}
	if want := []PaneID{8, 7, 6, 5, 4, 3, 2}; !reflect.DeepEqual(factory.closeOrder, want) {
		t.Fatalf("abort order=%v want=%v", factory.closeOrder, want)
	}
	assertPristineRestoreMux(t, m, before, beforeBounds, beforeMetrics)

	factory.closeOrder = nil
	retry, err := m.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]PaneID, len(retry.panes))
	for index, pane := range retry.panes {
		gotIDs[index] = pane.id
	}
	if want := []PaneID{2, 3, 4, 5, 6, 7, 8}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("retry pane IDs=%v want=%v", gotIDs, want)
	}
	if err = m.AbortRestore(retry); err != nil {
		t.Fatal(err)
	}
}

func TestMuxRestoreAbortClosesInReverseAndJoinsEveryCloseError(t *testing.T) {
	factory := &restoreObservationFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	closeThree := errors.New("close three")
	closeSeven := errors.New("close seven")
	factory.sessions[1].closeErr = closeThree
	factory.sessions[5].closeErr = closeSeven

	err = m.AbortRestore(candidate)
	if !errors.Is(err, closeThree) || !errors.Is(err, closeSeven) {
		t.Fatalf("abort error=%v", err)
	}
	if want := "pane 7 close: close seven\npane 3 close: close three"; err.Error() != want {
		t.Fatalf("joined error=%q want=%q", err, want)
	}
	if want := []PaneID{8, 7, 6, 5, 4, 3, 2}; !reflect.DeepEqual(factory.closeOrder, want) {
		t.Fatalf("abort order=%v want=%v", factory.closeOrder, want)
	}
}

func TestMuxRestoreEventsCharacterizeRevisionsFocusAndAddresses(t *testing.T) {
	factory := &restoreTestFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	events, err := m.CommitRestore(candidate)
	if err != nil {
		t.Fatal(err)
	}

	windows := []WindowID{2, 2, 2, 2, 3, 3, 3}
	tabs := []TabID{2, 2, 2, 3, 4, 4, 4}
	want := make([]Event, 0, 18)
	for index, pane := range candidate.panes {
		address := Event{Workspace: 3, Window: windows[index], Tab: tabs[index], Pane: pane.id}
		started, geometry := address, address
		started.Kind = PaneStarted
		geometry.Kind, geometry.Geometry = PaneGeometryChanged, pane.geometry
		want = append(want, started, geometry)
	}
	want = append(want,
		Event{Kind: WorkspaceActivated, Workspace: 3},
		Event{Kind: WindowActivated, Workspace: 3, Window: 3},
		Event{Kind: TabActivated, Workspace: 3, Window: 3, Tab: 4},
		Event{Kind: PaneFocused, Workspace: 3, Window: 3, Tab: 4, Pane: 7},
	)
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("restore events=%#v want=%#v", events, want)
	}
	for index, event := range events {
		if event.Revision != 0 {
			t.Fatalf("event %d revision=%d want=0", index, event.Revision)
		}
	}
	workspaces := m.model.Workspaces()
	if len(workspaces) != 2 || workspaces[0].Revision != 1 || workspaces[0].Focused != 0 || workspaces[0].Active || workspaces[1].Revision != 1 || workspaces[1].Focused != 3 || !workspaces[1].Active {
		t.Fatalf("workspace revisions/focus=%#v", workspaces)
	}
	windowsView := m.model.Windows()
	if len(windowsView) != 2 || windowsView[0].Revision != 1 || windowsView[0].Active || windowsView[1].Revision != 1 || !windowsView[1].Active {
		t.Fatalf("window revisions/focus=%#v", windowsView)
	}
	if got := windowsView[0].Tabs; len(got) != 2 || got[0].Revision != 1 || got[0].Focused != 4 || got[0].Active || got[1].Revision != 1 || got[1].Focused != 5 || !got[1].Active {
		t.Fatalf("first window tab revisions/focus=%#v", got)
	}
	if got := windowsView[1].Tabs; len(got) != 1 || got[0].Revision != 1 || got[0].Focused != 7 || !got[0].Active {
		t.Fatalf("second window tab revisions/focus=%#v", got)
	}
}

func TestMuxRestoreCreatesDistinctImageProtocolAdapters(t *testing.T) {
	limits := termimage.DefaultLimits()
	m := New(&restoreTestFactory{}, Options{
		IngressCapacity: 64,
		ImageLimits:     &limits,
		KittyEnabled:    true,
		SixelEnabled:    true,
		ITermEnabled:    true,
	})
	defer m.Shutdown()
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	defer m.AbortRestore(candidate)

	kittyAdapters := make(map[any]PaneID, len(candidate.panes))
	sixelAdapters := make(map[any]PaneID, len(candidate.panes))
	itermAdapters := make(map[any]PaneID, len(candidate.panes))
	for _, pane := range candidate.panes {
		if pane.imageStore == nil || pane.kittyAdapter == nil || pane.sixelAdapter == nil || pane.itermAdapter == nil {
			t.Fatalf("pane %d image state store=%p kitty=%p sixel=%p iterm=%p", pane.id, pane.imageStore, pane.kittyAdapter, pane.sixelAdapter, pane.itermAdapter)
		}
		if other, duplicate := kittyAdapters[pane.kittyAdapter]; duplicate {
			t.Fatalf("panes %d and %d share kitty adapter", other, pane.id)
		}
		if other, duplicate := sixelAdapters[pane.sixelAdapter]; duplicate {
			t.Fatalf("panes %d and %d share sixel adapter", other, pane.id)
		}
		if other, duplicate := itermAdapters[pane.itermAdapter]; duplicate {
			t.Fatalf("panes %d and %d share iTerm adapter", other, pane.id)
		}
		kittyAdapters[pane.kittyAdapter] = pane.id
		sixelAdapters[pane.sixelAdapter] = pane.id
		itermAdapters[pane.itermAdapter] = pane.id
	}
}

// TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread expires Slice 3.1.
func TestKnownDefect_L3_02_RestoreAcceptsDifferentOwnerThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	m := newRestoreMux(&restoreTestFactory{})
	defer m.Shutdown()
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	type result struct {
		candidate *RestoreCandidate
		err       error
	}
	resultCh := make(chan result, 1)
	go func() {
		candidate, err := m.PrepareRestore(blueprint, restoreGeometries())
		resultCh <- result{candidate: candidate, err: err}
	}()
	got := <-resultCh
	if got.err != nil || got.candidate == nil {
		t.Fatalf("different-thread restore candidate=%p err=%v", got.candidate, got.err)
	}
	if err := m.AbortRestore(got.candidate); err != nil {
		t.Fatal(err)
	}
}

// TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD expires Slice 4.3.
func TestKnownDefect_L3_07_FreshSessionUsesObservedTerminalCWD(t *testing.T) {
	m := New(&fakeFactory{}, Options{})
	defer m.Shutdown()
	configured := "C:/configured"
	observed := "C:/terminal-controlled"
	_, paneID, _, err := m.Bootstrap(SpawnSpec{Options: pty.Options{ShellProgram: "shell", WorkingDirectory: configured}}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	lookupPaneForTest(t, m.sessions, paneID).cwd = observed
	snapshot, err := m.FreshSessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.Workspaces[0].Windows[0].Tabs[0].Root.Launch.CWD
	if got != observed || got == configured {
		t.Fatalf("fresh CWD=%q configured=%q observed=%q", got, configured, observed)
	}
}

type restoreBenchmarkSession struct{ identity byte }

type restoreEOFReader struct{}

func (restoreEOFReader) Read([]byte) (int, error) { return 0, io.EOF }

func (*restoreBenchmarkSession) Reader() io.Reader              { return restoreEOFReader{} }
func (*restoreBenchmarkSession) Write(data []byte) (int, error) { return len(data), nil }
func (*restoreBenchmarkSession) Resize(pty.Size) error          { return nil }
func (*restoreBenchmarkSession) Close() error                   { return nil }

type restoreBenchmarkFactory struct{}

func (restoreBenchmarkFactory) Spawn(_, _ uint16, _ pty.Options) (pty.Session, error) {
	return &restoreBenchmarkSession{}, nil
}

var (
	benchmarkRestoreCandidate *RestoreCandidate
	benchmarkRestoreIDs       []WindowID
	benchmarkFreshSnapshot    FreshSessionSnapshot
	benchmarkRestoreError     error
)

func BenchmarkMuxRestorePrepareAbort(b *testing.B) {
	m := newRestoreMux(restoreBenchmarkFactory{})
	b.Cleanup(func() { _ = m.Shutdown() })
	blueprint := blueprintFromSnapshot(b, restoreSnapshot())
	geometries := restoreGeometries()
	run := func() {
		candidate, err := m.PrepareRestore(blueprint, geometries)
		if err != nil {
			b.Fatal(err)
		}
		if err = m.AbortRestore(candidate); err != nil {
			b.Fatal(err)
		}
		benchmarkRestoreCandidate = candidate
	}
	if allocs := testing.AllocsPerRun(20, run); allocs > 200 {
		b.Fatalf("prepare+abort allocations=%v max=200", allocs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		run()
	}
}

func BenchmarkMuxRestoreWindowIDs(b *testing.B) {
	m := newRestoreMux(restoreBenchmarkFactory{})
	b.Cleanup(func() { _ = m.Shutdown() })
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(b, restoreSnapshot()), restoreGeometries())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.AbortRestore(candidate) })
	run := func() {
		benchmarkRestoreIDs, benchmarkRestoreError = m.RestoreWindowIDs(candidate)
		if benchmarkRestoreError != nil {
			b.Fatal(benchmarkRestoreError)
		}
	}
	if allocs := testing.AllocsPerRun(100, run); allocs != 1 {
		b.Fatalf("window IDs allocations=%v want=1", allocs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		run()
	}
}

func BenchmarkMuxFreshSessionSnapshot(b *testing.B) {
	m := newRestoreMux(restoreBenchmarkFactory{})
	b.Cleanup(func() { _ = m.Shutdown() })
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(b, restoreSnapshot()), restoreGeometries())
	if err != nil {
		b.Fatal(err)
	}
	if _, err = m.CommitRestore(candidate); err != nil {
		b.Fatal(err)
	}
	run := func() {
		benchmarkFreshSnapshot, benchmarkRestoreError = m.FreshSessionSnapshot()
		if benchmarkRestoreError != nil {
			b.Fatal(benchmarkRestoreError)
		}
	}
	if allocs := testing.AllocsPerRun(20, run); allocs > 40 {
		b.Fatalf("fresh snapshot allocations=%v max=40", allocs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		run()
	}
}

func BenchmarkMuxRestoreFastPaths(b *testing.B) {
	blueprint := blueprintFromSnapshot(b, restoreSnapshot())
	geometries := restoreGeometries()
	b.Run("invalid", func(b *testing.B) {
		m := newRestoreMux(restoreBenchmarkFactory{})
		b.Cleanup(func() { _ = m.Shutdown() })
		run := func() {
			benchmarkRestoreIDs, benchmarkRestoreError = m.RestoreWindowIDs(nil)
		}
		run()
		if !errors.Is(benchmarkRestoreError, ErrInvalidRestore) {
			b.Fatalf("invalid error=%v", benchmarkRestoreError)
		}
		if allocs := testing.AllocsPerRun(1000, run); allocs != 0 {
			b.Fatalf("invalid allocations=%v want=0", allocs)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			run()
		}
	})
	b.Run("pending", func(b *testing.B) {
		m := newRestoreMux(restoreBenchmarkFactory{})
		b.Cleanup(func() { _ = m.Shutdown() })
		candidate, err := m.PrepareRestore(blueprint, geometries)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = m.AbortRestore(candidate) })
		run := func() {
			benchmarkRestoreCandidate, benchmarkRestoreError = m.PrepareRestore(blueprint, geometries)
		}
		run()
		if benchmarkRestoreCandidate != nil || !errors.Is(benchmarkRestoreError, ErrRestorePending) {
			b.Fatalf("pending candidate=%p err=%v", benchmarkRestoreCandidate, benchmarkRestoreError)
		}
		if allocs := testing.AllocsPerRun(1000, run); allocs != 0 {
			b.Fatalf("pending allocations=%v want=0", allocs)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			run()
		}
	})
}
