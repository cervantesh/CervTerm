package mux

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/layoutrestore"
	"cervterm/internal/layoutstate"
	"cervterm/internal/pty"
	"cervterm/internal/windowbounds"
)

type restoreTestFactory struct {
	calls         []pty.Size
	options       []pty.Options
	sessions      []*fakeSession
	failAt        int
	returnOnError bool
}

func (f *restoreTestFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	f.calls = append(f.calls, pty.Size{Rows: rows, Cols: cols})
	options.ShellArgs = append([]string(nil), options.ShellArgs...)
	options.Env = cloneEnvironment(options.Env)
	f.options = append(f.options, options)
	s := newFakeSession()
	f.sessions = append(f.sessions, s)
	if f.failAt == len(f.calls) {
		if f.returnOnError {
			return s, errors.New("injected spawn failure")
		}
		_ = s.Close()
		return nil, errors.New("injected spawn failure")
	}
	return s, nil
}

func restorePane(program string) layoutrestore.Node {
	return layoutrestore.Node{Type: "pane", Launch: &layoutrestore.ResolvedLaunch{Program: program, Args: []string{"--" + program}, CWD: "/" + program}}
}

func restoreSplit(axis string, ratio int, first, second layoutrestore.Node) layoutrestore.Node {
	return layoutrestore.Node{Type: "split", Axis: axis, Ratio: ratio, First: &first, Second: &second}
}

func restoreSnapshot() layoutrestore.Snapshot {
	return layoutrestore.Snapshot{
		ActiveWorkspace: 1,
		Workspaces: []layoutrestore.Workspace{
			{Name: "empty", ActiveWindow: -1},
			{Name: "active", ActiveWindow: 1, Windows: []layoutrestore.Window{
				{Title: "first", ActiveTab: 1, Tabs: []layoutrestore.Tab{
					{Title: "one", FocusedLeaf: 2, Root: restoreSplit("columns", 3700, restorePane("a"), restoreSplit("rows", 6100, restorePane("b"), restorePane("c")))},
					{Title: "two", FocusedLeaf: 0, Root: restorePane("d")},
				}},
				{Title: "second", ActiveTab: 0, Tabs: []layoutrestore.Tab{
					{Title: "three", FocusedLeaf: 1, Root: restoreSplit("rows", 4200, restoreSplit("columns", 7300, restorePane("e"), restorePane("f")), restorePane("g"))},
				}},
			}},
		},
	}
}

func blueprintFromSnapshot(t *testing.T, snapshot layoutrestore.Snapshot) layoutrestore.Blueprint {
	t.Helper()
	doc := layoutstate.Document{Version: layoutstate.Version1, ActiveWorkspace: snapshot.ActiveWorkspace, Workspaces: make([]layoutstate.Workspace, len(snapshot.Workspaces))}
	for wi, ws := range snapshot.Workspaces {
		doc.Workspaces[wi] = layoutstate.Workspace{Name: ws.Name, ActiveWindow: ws.ActiveWindow, Windows: make([]layoutstate.Window, len(ws.Windows))}
		for wini, win := range ws.Windows {
			out := layoutstate.Window{Title: win.Title, Bounds: layoutstate.Bounds{Width: 800, Height: 600}, ActiveTab: win.ActiveTab, Tabs: make([]layoutstate.Tab, len(win.Tabs))}
			for ti, tab := range win.Tabs {
				out.Tabs[ti] = layoutstate.Tab{Title: tab.Title, FocusedLeaf: tab.FocusedLeaf, Root: persistedRestoreNode(tab.Root)}
			}
			doc.Workspaces[wi].Windows[wini] = out
		}
	}
	plan, err := layoutstate.NewPlan(doc)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, err := layoutrestore.Prepare(plan, layoutrestore.Options{
		DefaultLaunch: layoutrestore.Launch{Program: "default"},
		Monitors:      []windowbounds.Monitor{{ID: "test", WorkArea: windowbounds.Rect{Width: 1920, Height: 1080}, ScaleX: 1, ScaleY: 1, Primary: true}},
		Policy:        windowbounds.Policy{FallbackWidth: 800, FallbackHeight: 600, MinWidth: 100, MinHeight: 100, ChromeHeight: 30, MinVisibleChromeX: 20, MinVisibleChromeY: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	return blueprint
}

func persistedRestoreNode(n layoutrestore.Node) layoutstate.Node {
	if n.Type == "pane" {
		return layoutstate.Node{Type: "pane", Launch: &layoutstate.Launch{Program: n.Launch.Program, Args: append([]string(nil), n.Launch.Args...), CWD: n.Launch.CWD}}
	}
	first, second := persistedRestoreNode(*n.First), persistedRestoreNode(*n.Second)
	return layoutstate.Node{Type: "split", Axis: n.Axis, Ratio: n.Ratio, First: &first, Second: &second}
}

func restoreGeometries() []RestoreWindowGeometry {
	return []RestoreWindowGeometry{
		{Content: PixelRect{X: 10, Y: 20, Width: 1000, Height: 640}, Metrics: CellMetrics{CellWidth: 10, CellHeight: 20}},
		{Content: PixelRect{X: 30, Y: 40, Width: 720, Height: 480}, Metrics: CellMetrics{CellWidth: 8, CellHeight: 16}},
	}
}

func newRestoreMux(factory SessionFactory) *Mux {
	return New(factory, Options{IngressCapacity: 64})
}

func assertPristineRestoreMux(t *testing.T, m *Mux, before *Model, bounds PixelRect, metrics map[PaneID]CellMetrics) {
	t.Helper()
	panes, reserved, started := m.sessions.activeCounts()
	if m.pending != nil || panes != 0 || reserved != 0 || started != 0 || m.bootstrapped || m.bounds != bounds || !reflect.DeepEqual(m.paneMetrics, metrics) || !reflect.DeepEqual(m.model, before) {
		t.Fatalf("mux not pristine: pending=%p counts=%d/%d/%d bootstrapped=%v bounds=%#v metrics=%#v model=%#v", m.pending, panes, reserved, started, m.bootstrapped, m.bounds, m.paneMetrics, m.model)
	}
}

func TestMuxRestoreCommitPublishesExactTopologySpawnsAndFinalEvents(t *testing.T) {
	factory := &restoreTestFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	geometries := restoreGeometries()
	beforeModel := m.model
	beforeWindows, beforeWorkspaces, beforePanes := m.model.Windows(), m.model.Workspaces(), m.PaneIDs()
	beforeBounds, beforeMetrics, beforeBootstrapped := m.bounds, m.paneMetrics, m.bootstrapped

	candidate, err := m.PrepareRestore(blueprint, geometries)
	if err != nil {
		t.Fatal(err)
	}
	if m.model != beforeModel || !reflect.DeepEqual(m.model.Windows(), beforeWindows) || !reflect.DeepEqual(m.model.Workspaces(), beforeWorkspaces) || !reflect.DeepEqual(m.PaneIDs(), beforePanes) || m.bounds != beforeBounds || !reflect.DeepEqual(m.paneMetrics, beforeMetrics) || m.bootstrapped != beforeBootstrapped {
		t.Fatal("Prepare published compatibility state")
	}
	if panes, reserved, started := m.sessions.activeCounts(); panes != 7 || reserved != 0 || started != 0 {
		t.Fatalf("candidate ownership counts=%d/%d/%d", panes, reserved, started)
	}
	for id := PaneID(2); id <= 8; id++ {
		if _, owned := m.sessions.lookup(id); !owned {
			t.Fatalf("candidate pane %d not registry-owned", id)
		}
		if _, visible := m.PaneView(id); visible {
			t.Fatalf("uncommitted pane %d reachable through PaneView", id)
		}
	}
	if events := m.Drain(64); len(events) != 0 {
		t.Fatalf("Prepare leaked events %#v", events)
	}

	latestPalette := core.DefaultPaletteBase()
	latestPalette.FG = core.RGB{R: 1, G: 2, B: 3}
	m.SetPaletteBase(latestPalette)
	if got := candidate.panes[0].terminal.PaletteBase(); got == latestPalette {
		t.Fatal("palette reload mutated unpublished candidate")
	}
	wantPrograms := []string{"a", "b", "c", "d", "e", "f", "g"}
	if len(factory.options) != len(wantPrograms) || len(factory.sessions) != len(wantPrograms) {
		t.Fatalf("spawn count options=%d sessions=%d", len(factory.options), len(factory.sessions))
	}
	seen := map[*fakeSession]bool{}
	for i, program := range wantPrograms {
		opt := factory.options[i]
		if opt.ShellProgram != program || !reflect.DeepEqual(opt.ShellArgs, []string{"--" + program}) || opt.WorkingDirectory != "/"+program || opt.Env != nil {
			t.Fatalf("spawn %d options=%#v", i+1, opt)
		}
		if seen[factory.sessions[i]] {
			t.Fatalf("spawn %d reused session", i+1)
		}
		seen[factory.sessions[i]] = true
		rows, cols := terminalSize(candidate.panes[i].geometry)
		want := pty.Size{Rows: rows, Cols: cols}
		if factory.calls[i] != want {
			t.Fatalf("spawn %d size=%#v want=%#v", i+1, factory.calls[i], want)
		}
	}

	events, err := m.CommitRestore(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range candidate.panes {
		if got := p.terminal.PaletteBase(); got != latestPalette {
			t.Fatalf("committed pane %d palette=%#v", p.id, got)
		}
	}
	if len(events) != 18 {
		t.Fatalf("event count=%d events=%#v", len(events), events)
	}
	for i := 0; i < 7; i++ {
		started, geometry := events[i*2], events[i*2+1]
		if started.Kind != PaneStarted || geometry.Kind != PaneGeometryChanged || started.Pane != PaneID(i+2) || geometry.Pane != PaneID(i+2) || started.Workspace == 0 || started.Window == 0 || started.Tab == 0 || geometry.Workspace != started.Workspace || geometry.Window != started.Window || geometry.Tab != started.Tab {
			t.Fatalf("leaf events %d=%#v %#v", i, started, geometry)
		}
	}
	wantFinal := []EventKind{WorkspaceActivated, WindowActivated, TabActivated, PaneFocused}
	for i, kind := range wantFinal {
		if events[14+i].Kind != kind {
			t.Fatalf("final events=%#v", events[14:])
		}
	}
	for _, event := range events[:17] {
		if event.Kind == PaneFocused {
			t.Fatalf("interim focus event %#v", event)
		}
	}
	workspaces := m.model.Workspaces()
	if len(workspaces) != 2 || workspaces[0].ID != 2 || workspaces[0].Name != "empty" || len(workspaces[0].Windows) != 0 || workspaces[0].Focused != 0 || workspaces[0].Active || workspaces[1].ID != 3 || workspaces[1].Name != "active" || !workspaces[1].Active || workspaces[1].Focused != 3 {
		t.Fatalf("workspaces=%#v", workspaces)
	}
	windows := m.model.Windows()
	if len(windows) != 2 || windows[0].ID != 2 || windows[0].Title != "first" || windows[0].Active || windows[1].ID != 3 || windows[1].Title != "second" || !windows[1].Active {
		t.Fatalf("windows=%#v", windows)
	}
	if got := windows[0].Tabs; len(got) != 2 || got[0].Title != "one" || got[0].Focused != 4 || got[0].Active || got[1].Title != "two" || got[1].Focused != 5 || !got[1].Active {
		t.Fatalf("window one tabs=%#v", got)
	}
	if got := windows[1].Tabs; len(got) != 1 || got[0].Title != "three" || got[0].Focused != 7 || !got[0].Active || !reflect.DeepEqual(got[0].Panes, []PaneID{6, 7, 8}) {
		t.Fatalf("window two tabs=%#v", got)
	}
	if m.bounds != geometries[1].Content {
		t.Fatalf("active bounds=%#v want=%#v", m.bounds, geometries[1].Content)
	}
	if candidate.model.windows[0].tabs[0].root.ratio != 3700 || candidate.model.windows[0].tabs[0].root.second.ratio != 6100 || candidate.model.windows[1].tabs[0].root.ratio != 4200 || candidate.model.windows[1].tabs[0].root.first.ratio != 7300 {
		t.Fatal("restore ratios changed")
	}
	view, _, err := m.CreateWindow(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}, "fresh")
	if err != nil || view.ID != 4 || view.Tabs[0].ID != 5 || view.Tabs[0].Focused != 9 {
		t.Fatalf("fresh IDs view=%#v err=%v", view, err)
	}
}

func TestMuxRestoreSpawnFailuresRollbackExactlyAndRetryFromOne(t *testing.T) {
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	for _, returned := range []bool{false, true} {
		for failAt := 1; failAt <= 7; failAt++ {
			t.Run(string(rune('A'+failAt-1))+map[bool]string{false: "-nil", true: "-session"}[returned], func(t *testing.T) {
				factory := &restoreTestFactory{failAt: failAt, returnOnError: returned}
				m := newRestoreMux(factory)
				defer m.Shutdown()
				before := m.model
				bounds, metrics := m.bounds, m.paneMetrics
				if candidate, err := m.PrepareRestore(blueprint, restoreGeometries()); candidate != nil || err == nil {
					t.Fatalf("PrepareRestore=%p,%v", candidate, err)
				}
				for i, session := range factory.sessions {
					if session.closes() != 1 {
						t.Fatalf("session %d closes=%d", i+1, session.closes())
					}
				}
				assertPristineRestoreMux(t, m, before, bounds, metrics)
				factory.failAt = 0
				candidate, err := m.PrepareRestore(blueprint, restoreGeometries())
				if err != nil || len(candidate.panes) != 7 || candidate.panes[0].id != 2 || candidate.panes[6].id != 8 {
					t.Fatalf("retry candidate=%#v err=%v", candidate, err)
				}
				if err := m.AbortRestore(candidate); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMuxRestoreAbortCommitMisuseAndStaleIngress(t *testing.T) {
	blueprint := blueprintFromSnapshot(t, restoreSnapshot())
	factory := &restoreTestFactory{}
	m := newRestoreMux(factory)
	defer m.Shutdown()
	candidate, err := m.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	oldOwner := candidate.panes[0]
	if err := m.AbortRestore(candidate); err != nil || m.AbortRestore(candidate) != nil {
		t.Fatalf("abort/idempotent=%v", err)
	}
	for i, session := range factory.sessions {
		if session.closes() != 1 {
			t.Fatalf("session %d closes=%d", i, session.closes())
		}
	}
	if _, err := m.CommitRestore(candidate); !errors.Is(err, ErrInvalidRestore) {
		t.Fatalf("commit aborted=%v", err)
	}
	if _, err := m.CommitRestore(nil); !errors.Is(err, ErrInvalidRestore) {
		t.Fatalf("commit nil=%v", err)
	}
	other := newRestoreMux(&restoreTestFactory{})
	defer other.Shutdown()
	if _, err := other.CommitRestore(candidate); !errors.Is(err, ErrInvalidRestore) {
		t.Fatalf("commit wrong mux=%v", err)
	}

	retry, err := m.PrepareRestore(blueprint, restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	m.sessions.incoming <- ingressRecord{pane: oldOwner.id, owner: oldOwner, data: []byte("stale")}
	if events := m.Drain(8); len(events) != 0 {
		t.Fatalf("stale ingress events=%#v", events)
	}
	if retry.panes[0].contentGen != 0 {
		t.Fatal("stale ingress reached retry pane")
	}
	events, err := m.CommitRestore(retry)
	if err != nil || len(events) == 0 {
		t.Fatalf("commit retry=%#v,%v", events, err)
	}
	closes := make([]int, len(factory.sessions))
	for i, session := range factory.sessions {
		closes[i] = session.closes()
	}
	if err := m.AbortRestore(retry); !errors.Is(err, ErrRestoreCommitted) {
		t.Fatalf("abort committed=%v", err)
	}
	for i, session := range factory.sessions {
		if session.closes() != closes[i] {
			t.Fatalf("abort committed closed session %d", i)
		}
	}
	if _, err := m.CommitRestore(retry); !errors.Is(err, ErrInvalidRestore) {
		t.Fatalf("commit stale=%v", err)
	}
}

func TestMuxRestoreRejectsInvalidBlueprintAndGeometryBeforeSpawn(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*layoutrestore.Snapshot)
		geometries []RestoreWindowGeometry
	}{
		{name: "geometry-count", geometries: restoreGeometries()[:1]},
		{name: "invalid-geometry", geometries: []RestoreWindowGeometry{{Content: PixelRect{Width: 1000, Height: 640}, Metrics: CellMetrics{}}, restoreGeometries()[1]}},
		{name: "compressed", geometries: []RestoreWindowGeometry{{Content: PixelRect{Width: 20, Height: 20}, Metrics: CellMetrics{CellWidth: 10, CellHeight: 20}}, restoreGeometries()[1]}},
		{name: "invalid-tree", mutate: func(s *layoutrestore.Snapshot) { s.Workspaces[1].Windows[0].Tabs[0].Root.Ratio = 0 }, geometries: restoreGeometries()},
		{name: "active-empty", mutate: func(s *layoutrestore.Snapshot) { s.ActiveWorkspace = 0 }, geometries: restoreGeometries()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := restoreSnapshot()
			// The blueprint path validates malformed trees first, so use the detached builder for
			// adversarial unions that layoutrestore correctly refuses to construct.
			if tc.mutate != nil {
				tc.mutate(&snapshot)
			}
			factory := &restoreTestFactory{}
			m := newRestoreMux(factory)
			defer m.Shutdown()
			if tc.name == "invalid-tree" || tc.name == "active-empty" {
				if _, err := buildRestoreCandidate(m, snapshot, tc.geometries); err == nil {
					t.Fatal("invalid tree accepted")
				}
			} else {
				blueprint := blueprintFromSnapshot(t, snapshot)
				if _, err := m.PrepareRestore(blueprint, tc.geometries); err == nil {
					t.Fatal("invalid restore accepted")
				}
			}
			if len(factory.calls) != 0 {
				t.Fatalf("rejection spawned %d sessions", len(factory.calls))
			}
		})
	}
}

func TestMuxRestorePrepareDoesNotStartReaders(t *testing.T) {
	factory := &restoreTestFactory{}
	wakes := make(chan struct{}, 8)
	m := New(factory, Options{IngressCapacity: 8, Wake: func() { wakes <- struct{}{} }})
	defer m.Shutdown()
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	feedDone := make(chan error, 1)
	go func() { feedDone <- factory.sessions[0].feed([]byte("hidden")) }()
	select {
	case err := <-feedDone:
		t.Fatalf("write completed before reader start: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case <-wakes:
		t.Fatal("reader started before commit")
	default:
	}
	if events := m.Drain(8); len(events) != 0 {
		t.Fatalf("candidate ingress published events=%#v", events)
	}
	if candidate.panes[0].contentGen != 0 {
		t.Fatal("candidate ingress mutated unpublished pane")
	}
	if err := m.AbortRestore(candidate); err != nil {
		t.Fatal(err)
	}
	select {
	case <-feedDone:
	case <-time.After(time.Second):
		t.Fatal("aborting candidate did not unblock session writer")
	}
}
