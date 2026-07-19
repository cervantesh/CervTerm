package mux

import (
	"reflect"
	"testing"

	"cervterm/internal/pty"
)

func addRuntimeTestWindow(t *testing.T, m *Mux, metrics CellMetrics) (WindowView, *fakeSession) {
	t.Helper()
	view, err := m.model.CreateWindow("two")
	if err != nil {
		t.Fatal(err)
	}
	paneID := view.Tabs[0].Focused
	cols, rows := cellGeometry(m.bounds, metrics)
	p := newPane(paneID, cols, rows, m.options.ScrollbackCapacity, m.options.HideCursorWhenScrolled)
	session, err := m.sessions.spawn(uint16(rows), uint16(cols), pty.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p.session = session
	p.state = PaneStateRunning
	p.geometry = PaneGeometry{Pane: paneID, Pixels: m.bounds, Cols: cols, Rows: rows}
	p.desiredSize = pty.Size{Rows: uint16(rows), Cols: uint16(cols)}
	p.appliedSize = p.desiredSize
	if err := m.sessions.reserve(paneID); err != nil {
		t.Fatal(err)
	}
	if err := m.sessions.register(p); err != nil {
		t.Fatal(err)
	}
	if err := m.sessions.start(paneID); err != nil {
		t.Fatal(err)
	}
	m.paneMetrics[paneID] = metrics
	return view, session.(*fakeSession)
}

func TestMuxCrossWindowPaneTransferPreservesRegistrySessionAndIngress(t *testing.T) {
	m, _, wakes := newTestMux(t)
	metrics := tabMetrics()
	window2, _ := addRuntimeTestWindow(t, m, metrics)
	if err := m.model.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	pane, _, err := m.SpawnSplit(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	owned, _ := m.sessions.lookup(pane)
	session := owned.session.(*fakeSession)
	registryCount := m.sessions.count()
	resizeCount := fakeResizeCount(session)
	events, err := m.TransferPaneBetweenWindows(PaneTransferRequest{SourceWindow: 1, DestinationWindow: 2, Pane: pane, DestinationTab: window2.Tabs[0].ID, DestinationPane: window2.Tabs[0].Focused, Axis: SplitRows, Ratio: DefaultSplitRatio, SourceBounds: PixelRect{Width: 800, Height: 480}, DestinationBounds: PixelRect{Width: 480, Height: 320}, Resolve: m.resolveMetrics})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := m.sessions.lookup(pane)
	if after != owned || after.session != session || m.sessions.count() != registryCount || session.closes() != 0 || fakeResizeCount(session) != resizeCount {
		t.Fatalf("ownership/session changed events=%#v", events)
	}
	owner := m.model.windowForTab(m.model.tabForPane(pane).id)
	if owner == nil || owner.id != 2 {
		t.Fatalf("owner=%#v", owner)
	}
	if len(events) == 0 || events[0].Window != 2 || events[0].SourceWindow != 1 {
		t.Fatalf("events=%#v", events)
	}
	if err := session.feed([]byte("cross")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	_ = m.Drain(16)
	if view, ok := m.PaneView(pane); !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'c' {
		t.Fatalf("ingress lost %#v", view)
	}
}

func TestMuxCrossWindowWholeTabMoveDoesNotTouchSessions(t *testing.T) {
	m, first, _ := newTestMux(t)
	metrics := tabMetrics()
	_, second := addRuntimeTestWindow(t, m, metrics)
	if err := m.model.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	before := []int{first.closes(), second.closes(), fakeResizeCount(first), fakeResizeCount(second)}
	events, err := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: 1, SourceBounds: PixelRect{Width: 800, Height: 480}, DestinationBounds: PixelRect{Width: 600, Height: 360}, Resolve: m.resolveMetrics})
	if err != nil {
		t.Fatal(err)
	}
	after := []int{first.closes(), second.closes(), fakeResizeCount(first), fakeResizeCount(second)}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("session lifecycle=%v -> %v", before, after)
	}
	windows := m.model.Windows()
	if m.model.windowForTab(1).id != 2 || len(windows[0].Tabs) != 0 || len(windows[1].Tabs) != 2 || !windows[1].Active {
		t.Fatalf("windows=%#v", windows)
	}
	if len(events) < 3 || events[0].Kind != TabMoved || events[0].Window != 2 || events[0].SourceWindow != 1 {
		t.Fatalf("events=%#v", events)
	}
}
