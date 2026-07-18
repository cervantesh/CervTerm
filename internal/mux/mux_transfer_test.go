package mux

import (
	"errors"
	"reflect"
	"testing"
)

func fakeResizeCount(session *fakeSession) int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return len(session.resizes)
}
func transferEventKinds(events []Event) []EventKind {
	out := make([]EventKind, len(events))
	for i := range events {
		out[i] = events[i].Kind
	}
	return out
}

func TestMuxTransferPanePreservesSessionAndDefersPTYResize(t *testing.T) {
	m, _, wakes := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	if _, _, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	pane, _, err := m.SpawnSplit(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	owned := m.panes[pane]
	session := factory.sessions[2]
	sessionsBefore := len(factory.sessions)
	resizeBefore := []int{fakeResizeCount(factory.sessions[0]), fakeResizeCount(factory.sessions[1]), fakeResizeCount(factory.sessions[2])}
	destinationGeometry := m.panes[2].geometry
	events, err := m.TransferPane(pane, 2, 2, SplitRows)
	if err != nil {
		t.Fatal(err)
	}
	if m.panes[pane] != owned || owned.session != session || len(factory.sessions) != sessionsBefore || session.closes() != 0 {
		t.Fatalf("identity changed pane=%p/%p session=%p/%p count=%d closes=%d", m.panes[pane], owned, owned.session, session, len(factory.sessions), session.closes())
	}
	for i, s := range factory.sessions {
		if got := fakeResizeCount(s); got != resizeBefore[i] {
			t.Fatalf("session %d resized: %d -> %d", i, resizeBefore[i], got)
		}
	}
	if len(events) == 0 || events[0].Kind != PaneTransferred || events[0].SourceTab != 1 || events[0].Tab != 2 {
		t.Fatalf("events=%#v", events)
	}
	wantKinds := []EventKind{PaneTransferred, TabRevisionChanged, TabRevisionChanged, PaneFocused, PaneGeometryChanged, PaneDirty}
	if got := transferEventKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event order=%v want=%v", got, wantKinds)
	}
	if owner := m.model.tabForPane(pane); owner == nil || owner.id != 2 {
		t.Fatalf("owner=%#v", owner)
	}
	if m.panes[2].geometry != destinationGeometry {
		t.Fatalf("inactive destination geometry changed: %#v -> %#v", destinationGeometry, m.panes[2].geometry)
	}
	activateEvents, err := m.ActivateTab(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(activateEvents) < 4 || m.panes[2].geometry == destinationGeometry {
		t.Fatalf("activation did not apply deferred geometry: events=%#v geometry=%#v", activateEvents, m.panes[2].geometry)
	}
	for i, s := range factory.sessions {
		if got := fakeResizeCount(s); got != resizeBefore[i] {
			t.Fatalf("activation resized PTY %d: %d -> %d", i, resizeBefore[i], got)
		}
	}
	if err := session.feed([]byte("moved")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	_ = m.Drain(16)
	if view, ok := m.PaneView(pane); !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'm' {
		t.Fatalf("transferred ingress lost: %#v", view)
	}
}

func TestMuxTransferFinalActiveSourceEmitsNoCloseOrEmpty(t *testing.T) {
	m, first, _ := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	if _, _, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	events, err := m.TransferPane(1, 2, 2, SplitColumns)
	if err != nil {
		t.Fatal(err)
	}
	if m.ActiveTab() != 2 || first.closes() != 0 || len(factory.sessions) != 2 {
		t.Fatalf("active=%d closes=%d sessions=%d", m.ActiveTab(), first.closes(), len(factory.sessions))
	}
	var transferred, tabClosed, activated bool
	for _, event := range events {
		transferred = transferred || event.Kind == PaneTransferred
		tabClosed = tabClosed || event.Kind == TabClosed
		activated = activated || event.Kind == TabActivated
		if event.Kind == PaneClosed || event.Kind == WindowTabsEmpty || event.Kind == TabEmpty {
			t.Fatalf("unexpected event=%#v", event)
		}
	}
	if !transferred || !tabClosed || !activated {
		t.Fatalf("events=%#v", events)
	}
}

func TestMuxTransferFailureLeavesViewsSessionsAndCountersUntouched(t *testing.T) {
	m, _, _ := newTestMux(t)
	factory := m.factory.(*fakeFactory)
	if _, _, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "two"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	pane, _, err := m.SpawnSplit(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	beforeTabs := m.Tabs()
	beforeSplit := m.model.nextSplitID
	beforeSessions := len(factory.sessions)
	beforePane := m.panes[pane]
	m.bounds = PixelRect{Width: 16, Height: 16}
	events, err := m.TransferPane(pane, 2, 2, SplitColumns)
	if !errors.Is(err, ErrTopologyTooSmall) || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextSplitID != beforeSplit || len(factory.sessions) != beforeSessions || m.panes[pane] != beforePane {
		t.Fatalf("tabs=%#v split=%d sessions=%d pane=%p", m.Tabs(), m.model.nextSplitID, len(factory.sessions), m.panes[pane])
	}
}
