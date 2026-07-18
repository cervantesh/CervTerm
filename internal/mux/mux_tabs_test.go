package mux

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/pty"
)

func tabMetrics() CellMetrics { return CellMetrics{CellWidth: 8, CellHeight: 16} }

func TestSpawnActivateRenameMoveAndCloseTabLifecycle(t *testing.T) {
	m, first, _ := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	tab, pane, events, err := m.SpawnTab(SpawnSpec{Options: pty.Options{ShellProgram: "tool", ShellArgs: []string{"a b"}}}, tabMetrics(), "Tools")
	if err != nil {
		t.Fatal(err)
	}
	if tab != 2 || pane != 2 || m.ActiveTab() != 2 || len(events) != 4 || events[0].Kind != TabSpawned || events[0].Tab != 2 {
		t.Fatalf("tab=%d pane=%d active=%d events=%#v", tab, pane, m.ActiveTab(), events)
	}
	if got := m.PaneIDs(); !reflect.DeepEqual(got, []PaneID{2}) {
		t.Fatalf("active panes=%v", got)
	}
	if _, err := m.RenameTab(2, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.MoveTab(2, 0); err != nil {
		t.Fatal(err)
	}
	if tabs := m.Tabs(); tabs[0].ID != 2 || tabs[0].Title != "Renamed" {
		t.Fatalf("tabs=%#v", tabs)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	focused, _ := m.FocusedPane()
	if focused != 1 {
		t.Fatalf("focus=%d", focused)
	}
	if _, err := m.ActivateTab(2); err != nil {
		t.Fatal(err)
	}
	closeEvents, err := m.CloseTab(2)
	if err != nil {
		t.Fatal(err)
	}
	if m.ActiveTab() != 1 || len(m.Tabs()) != 1 || factory.sessions[1].closes() != 1 || first.closes() != 0 {
		t.Fatalf("active=%d tabs=%#v closes=%d/%d events=%#v", m.ActiveTab(), m.Tabs(), first.closes(), factory.sessions[1].closes(), closeEvents)
	}
}

func TestSpawnTabFailureLeavesEveryIdentityAndOwnerUntouched(t *testing.T) {
	m, _, _ := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	candidate := newFakeSession()
	factory.err = errors.New("spawn failed")
	factory.sessionOnError = candidate
	beforeTabs := m.Tabs()
	beforeTabID, beforePaneID := m.model.nextTabID, m.model.nextPaneID
	beforePanes := len(m.panes)
	if _, _, events, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "bad"); err == nil || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextTabID != beforeTabID || m.model.nextPaneID != beforePaneID || len(m.panes) != beforePanes || candidate.closes() != 1 {
		t.Fatalf("tabs=%#v ids=%d/%d panes=%d close=%d", m.Tabs(), m.model.nextTabID, m.model.nextPaneID, len(m.panes), candidate.closes())
	}
}

func TestCloseTabAggregatesErrorAfterAtomicDetachAndAddressesFinalEvent(t *testing.T) {
	m, first, _ := newTestMux(t)
	first.closeErr = errors.New("close failed")
	events, err := m.CloseTab(1)
	if err == nil || !m.model.Empty() || len(m.panes) != 0 || first.closes() != 1 {
		t.Fatalf("err=%v empty=%v panes=%d closes=%d", err, m.model.Empty(), len(m.panes), first.closes())
	}
	var failed, closed, window bool
	for _, event := range events {
		if event.Tab != 1 {
			t.Fatalf("unaddressed event=%#v", event)
		}
		failed = failed || event.Kind == PaneCloseFailed
		closed = closed || event.Kind == TabClosed
		window = window || event.Kind == WindowTabsEmpty
	}
	if !failed || !closed || !window {
		t.Fatalf("events=%#v", events)
	}
}

func TestInactiveTabIngressDoesNotChangeActiveProjection(t *testing.T) {
	m, _, wakes := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	_, pane, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "background")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	if err := factory.sessions[1].feed([]byte("background")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	events := m.Drain(16)
	if len(events) == 0 {
		t.Fatal("missing ingress events")
	}
	if m.ActiveTab() != 1 || !reflect.DeepEqual(m.PaneIDs(), []PaneID{1}) {
		t.Fatalf("active=%d panes=%v background=%d", m.ActiveTab(), m.PaneIDs(), pane)
	}
	view, ok := m.panes[pane]
	if !ok || len(view.snapshot.Cells) == 0 || view.snapshot.Cells[0].Rune != 'b' {
		t.Fatalf("background pane not advanced")
	}
}
