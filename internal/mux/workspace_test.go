package mux

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceModelDefaultCreateRenameSwitchAndMembership(t *testing.T) {
	m := NewModel()
	if got := m.ActiveWorkspace(); got.ID != 1 || got.Name != DefaultWorkspaceName || !reflect.DeepEqual(got.Windows, []WindowID{1}) || got.Focused != 1 {
		t.Fatalf("default=%#v", got)
	}
	work, err := m.CreateWorkspace("  work  ")
	if err != nil {
		t.Fatal(err)
	}
	if work.ID != 2 || work.Name != "work" || len(work.Windows) != 0 {
		t.Fatalf("work=%#v", work)
	}
	if _, err := m.CreateWorkspace("work"); !errors.Is(err, ErrWorkspaceNameExists) {
		t.Fatalf("duplicate=%v", err)
	}
	if err := m.SwitchWorkspace(work.ID); err != nil {
		t.Fatal(err)
	}
	if got := m.ActiveWorkspace(); got.ID != work.ID || got.Focused != 0 || m.ActiveWindow().ID != 0 {
		t.Fatalf("empty switch=%#v window=%#v", got, m.ActiveWindow())
	}
	window, err := m.CreateWindow("two")
	if err != nil {
		t.Fatal(err)
	}
	if window.Workspace != work.ID || m.ActiveWorkspace().Focused != window.ID {
		t.Fatalf("window=%#v workspace=%#v", window, m.ActiveWorkspace())
	}
	if err := m.RenameWorkspace(work.ID, "build"); err != nil {
		t.Fatal(err)
	}
	if err := m.SwitchWorkspace(1); err != nil {
		t.Fatal(err)
	}
	if m.ActiveWindow().ID != 1 {
		t.Fatalf("remembered=%#v", m.ActiveWindow())
	}
	if err := m.MoveWindowToWorkspace(1, work.ID); err != nil {
		t.Fatal(err)
	}
	if got := m.ActiveWorkspace(); got.ID != 1 || len(got.Windows) != 0 || got.Focused != 0 || m.ActiveWindow().ID != 0 {
		t.Fatalf("emptied default=%#v active=%#v", got, m.ActiveWindow())
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceNamesAreBoundedNormalizedAndValid(t *testing.T) {
	m := NewModel()
	for _, name := range []string{"", " \t ", "bad\x00name", string([]byte{0xff})} {
		if _, err := m.CreateWorkspace(name); !errors.Is(err, ErrInvalidWorkspaceName) {
			t.Fatalf("name %q err=%v", name, err)
		}
	}
	if _, err := m.CreateWorkspace(strings.Repeat("x", MaxWorkspaceNameBytes+1)); !errors.Is(err, ErrWorkspaceNameTooLong) {
		t.Fatalf("long=%v", err)
	}
	for i := 1; i < MaxWorkspaces; i++ {
		if _, err := m.CreateWorkspace("w" + string(rune(0x100+i))); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := m.CreateWorkspace("overflow"); !errors.Is(err, ErrWorkspaceLimitReached) {
		t.Fatalf("limit=%v", err)
	}
}

func TestWorkspaceRandomizedOwnershipInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(97))
	m := NewModel()
	for i := 0; i < 6; i++ {
		if _, err := m.CreateWorkspace("ws" + string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 12; i++ {
		if _, err := m.CreateWindow(""); err != nil {
			t.Fatal(err)
		}
	}
	for step := 0; step < 500; step++ {
		workspaces := m.Workspaces()
		windows := m.Windows()
		switch rng.Intn(3) {
		case 0:
			_ = m.SwitchWorkspace(workspaces[rng.Intn(len(workspaces))].ID)
		case 1:
			if len(windows) > 0 {
				_ = m.MoveWindowToWorkspace(windows[rng.Intn(len(windows))].ID, workspaces[rng.Intn(len(workspaces))].ID)
			}
		case 2:
			ws := workspaces[rng.Intn(len(workspaces))]
			_ = m.RenameWorkspace(ws.ID, ws.Name)
		}
		if err := m.CheckInvariants(); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
}

func TestMuxWorkspaceSwitchNeverSuspendsSessions(t *testing.T) {
	m, first, wakes := newTestMux(t)
	bounds, metrics := windowTestGeometry()
	secondView, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "two")
	if err != nil {
		t.Fatal(err)
	}
	factory := m.sessions.factoryForTest().(*fakeFactory)
	second := factory.sessions[1]
	workspace, _, err := m.CreateWorkspace("work")
	if err != nil {
		t.Fatal(err)
	}
	moveEvents, err := m.MoveWindowToWorkspace(secondView.ID, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	switchEvents, err := m.SwitchWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(moveEvents) == 0 || len(switchEvents) == 0 || m.sessions.count() != 2 || first.closes() != 0 || second.closes() != 0 {
		t.Fatalf("events=%#v/%#v count=%d", moveEvents, switchEvents, m.sessions.count())
	}
	if err := first.feed([]byte("hidden")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	hiddenEvents := m.Drain(16)
	for _, event := range hiddenEvents {
		if (event.Kind == PaneOutput || event.Kind == PaneDirty) && (event.Window != 1 || event.Workspace != 1) {
			t.Fatalf("unaddressed hidden event=%#v", event)
		}
	}
	if view, ok := m.PaneView(1); !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'h' {
		t.Fatalf("hidden ingress=%#v", view)
	}
	if _, err := m.SwitchWorkspace(1); err != nil {
		t.Fatal(err)
	}
	if first.closes() != 0 || second.closes() != 0 {
		t.Fatal("switch closed session")
	}
}

func TestWorkspaceFailuresAndNoOpsAreAtomic(t *testing.T) {
	m := NewModel()
	work, err := m.CreateWorkspace("work")
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func(name string, run func() error) {
		t.Helper()
		before := snapshotTransferModel(m)
		_ = run()
		if after := snapshotTransferModel(m); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s mutated\nbefore=%#v\nafter=%#v", name, before, after)
		}
	}
	assertUnchanged("duplicate create", func() error { _, err := m.CreateWorkspace(" work "); return err })
	assertUnchanged("duplicate rename", func() error { return m.RenameWorkspace(1, "work") })
	assertUnchanged("missing switch", func() error { return m.SwitchWorkspace(999) })
	assertUnchanged("missing move window", func() error { return m.MoveWindowToWorkspace(999, work.ID) })
	assertUnchanged("missing move workspace", func() error { return m.MoveWindowToWorkspace(1, 999) })
	assertUnchanged("same membership", func() error { return m.MoveWindowToWorkspace(1, 1) })
}

func TestCrossWorkspaceTransferDoesNotSwitchActiveWorkspace(t *testing.T) {
	m := NewModel()
	second, err := m.CreateWindow("two")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := m.CreateWorkspace("hidden")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MoveWindowToWorkspace(second.ID, workspace.ID); err != nil {
		t.Fatal(err)
	}
	result, err := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: second.ID, Tab: 1, Position: 1, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics})
	if err != nil {
		t.Fatal(err)
	}
	if m.ActiveWorkspace().ID != 1 || m.activeWindow != 1 || result.ActiveWindow != 1 || m.workspaceByID(workspace.ID).active != second.ID {
		t.Fatalf("result=%#v workspaces=%#v", result, m.Workspaces())
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseWindowWorkspaceActivationSemantics(t *testing.T) {
	t.Run("inactive close has no focus events", func(t *testing.T) {
		m, first, _ := newTestMux(t)
		bounds, metrics := windowTestGeometry()
		second, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "two")
		if err != nil {
			t.Fatal(err)
		}
		workspace, _, err := m.CreateWorkspace("hidden")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.MoveWindowToWorkspace(second.ID, workspace.ID); err != nil {
			t.Fatal(err)
		}
		result, events, err := m.CloseWindow(second.ID)
		if err != nil {
			t.Fatal(err)
		}
		if result.ActiveChanged || result.WorkspaceChanged || result.Active != 1 || first.closes() != 0 {
			t.Fatalf("result=%#v", result)
		}
		for _, event := range events {
			if event.Kind == WindowActivated || event.Kind == WorkspaceActivated || event.Kind == PaneFocused {
				t.Fatalf("spurious event=%#v", event)
			}
		}
	})
	t.Run("closing active final window reveals next workspace", func(t *testing.T) {
		m, _, _ := newTestMux(t)
		bounds, metrics := windowTestGeometry()
		second, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "two")
		if err != nil {
			t.Fatal(err)
		}
		workspace, _, err := m.CreateWorkspace("hidden")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.MoveWindowToWorkspace(second.ID, workspace.ID); err != nil {
			t.Fatal(err)
		}
		result, events, err := m.CloseWindow(1)
		if err != nil {
			t.Fatal(err)
		}
		if !result.ActiveChanged || !result.WorkspaceChanged || result.ActiveWorkspace != workspace.ID || result.Active != second.ID {
			t.Fatalf("result=%#v", result)
		}
		if len(events) < 4 || events[len(events)-4].Kind != WorkspaceActivated || events[len(events)-3].Kind != WindowActivated {
			t.Fatalf("events=%#v", events)
		}
	})
}

func TestCloseWindowValidatesWorkspaceOwnershipBeforeMutation(t *testing.T) {
	m := NewModel()
	m.workspaces[0].windows = nil
	beforeWindows, beforeWorkspaces := cloneWindowStates(m.windows), cloneWorkspaceStates(m.workspaces)
	if _, err := m.CloseWindow(1); err == nil {
		t.Fatal("expected invariant error")
	}
	if !reflect.DeepEqual(beforeWindows, m.windows) || !reflect.DeepEqual(beforeWorkspaces, m.workspaces) {
		t.Fatalf("mutated windows=%#v workspaces=%#v", m.windows, m.workspaces)
	}
}
