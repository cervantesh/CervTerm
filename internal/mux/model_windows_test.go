package mux

import (
	"errors"
	"reflect"
	"testing"
)

func TestWindowModelPreservesOneWindowCompatibility(t *testing.T) {
	m := NewModel()
	windows := m.Windows()
	if len(windows) != 1 || windows[0].ID != 1 || !windows[0].Active || len(windows[0].Tabs) != 1 || windows[0].Tabs[0].ID != 1 || windows[0].Tabs[0].Focused != 1 {
		t.Fatalf("windows=%#v", windows)
	}
	if m.TabID() != 1 || m.FocusedPane() != 1 || !reflect.DeepEqual(m.PaneIDs(), []PaneID{1}) {
		t.Fatalf("tab=%d focus=%d panes=%v", m.TabID(), m.FocusedPane(), m.PaneIDs())
	}
}

func TestWindowModelAllocatesGlobalMonotonicTopologyIDs(t *testing.T) {
	m := NewModel()
	second, err := m.CreateWindow("two")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != 2 || second.Tabs[0].ID != 2 || second.Tabs[0].Focused != 2 {
		t.Fatalf("second=%#v", second)
	}
	bounds := PixelRect{Width: 800, Height: 480}
	resolve := func(PaneID) (CellMetrics, bool) { return CellMetrics{CellWidth: 8, CellHeight: 16}, true }
	pane3, err := m.SplitWithMetrics(2, SplitColumns, bounds, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	pane4, err := m.SplitWithMetrics(1, SplitRows, bounds, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if pane3 != 3 || pane4 != 4 || m.nextSplitID != 3 {
		t.Fatalf("panes=%d/%d nextSplit=%d", pane3, pane4, m.nextSplitID)
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowModelCapFailureDoesNotConsumeIdentity(t *testing.T) {
	m := NewModel()
	for i := 1; i < MaxWindows; i++ {
		if _, err := m.CreateWindow(""); err != nil {
			t.Fatal(err)
		}
	}
	beforeWindow, beforeTab, beforePane := m.nextWindowID, m.nextTabID, m.nextPaneID
	before := m.Windows()
	if _, err := m.CreateWindow("overflow"); !errors.Is(err, ErrWindowLimitReached) {
		t.Fatalf("err=%v", err)
	}
	if m.nextWindowID != beforeWindow || m.nextTabID != beforeTab || m.nextPaneID != beforePane || !reflect.DeepEqual(m.Windows(), before) {
		t.Fatal("failed create mutated identities or windows")
	}
}

func TestWindowCloseUsesAdjacentFallbackAndNeverReusesIDs(t *testing.T) {
	m := NewModel()
	if _, err := m.CreateWindow("two"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateWindow("three"); err != nil {
		t.Fatal(err)
	}
	if err := m.ActivateWindow(2); err != nil {
		t.Fatal(err)
	}
	result, err := m.CloseWindow(2)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Closed || result.Active != 3 || m.ActiveWindow().ID != 3 {
		t.Fatalf("result=%#v active=%#v", result, m.ActiveWindow())
	}
	again, err := m.CloseWindow(2)
	if err != nil || again.Closed {
		t.Fatalf("again=%#v err=%v", again, err)
	}
	created, err := m.CreateWindow("four")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 4 || created.Tabs[0].ID != 4 || created.Tabs[0].Focused != 4 {
		t.Fatalf("created=%#v", created)
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowModelInvariantRejectsCrossWindowPaneOwnership(t *testing.T) {
	m := NewModel()
	if _, err := m.CreateWindow("two"); err != nil {
		t.Fatal(err)
	}
	m.windows[1].tabs[0].root = leafNode(1)
	m.windows[1].tabs[0].focused = 1
	if err := m.CheckInvariants(); !errors.Is(err, ErrInvariant) {
		t.Fatalf("err=%v", err)
	}
}
