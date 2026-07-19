package mux

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

type modelTransferSnapshot struct {
	windows                       []windowState
	active                        WindowID
	nextWindow, nextTab, nextPane uint64
	nextSplit                     SplitID
	allocatedWindows              map[WindowID]struct{}
	allocatedTabs                 map[TabID]struct{}
	allocatedPanes                map[PaneID]struct{}
	allocatedSplits               map[SplitID]struct{}
}

func snapshotTransferModel(m *Model) modelTransferSnapshot {
	return modelTransferSnapshot{
		windows: cloneWindowStates(m.windows), active: m.activeWindow,
		nextWindow: uint64(m.nextWindowID), nextTab: uint64(m.nextTabID), nextPane: uint64(m.nextPaneID), nextSplit: m.nextSplitID,
		allocatedWindows: cloneSet(m.allocatedWindows), allocatedTabs: cloneSet(m.allocatedTabs),
		allocatedPanes: cloneSet(m.allocated), allocatedSplits: cloneSet(m.allocatedSplits),
	}
}

func cloneSet[K comparable](in map[K]struct{}) map[K]struct{} {
	out := make(map[K]struct{}, len(in))
	for key := range in {
		out[key] = struct{}{}
	}
	return out
}

func crossWindowModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel()
	if _, err := m.CreateWindow("destination"); err != nil {
		t.Fatal(err)
	}
	return m
}

func varyingMetrics(id PaneID) (CellMetrics, bool) {
	if id == 0 {
		return CellMetrics{}, false
	}
	return CellMetrics{CellWidth: 5 + int(id%5), CellHeight: 9 + int(id%7)}, true
}

func TestTransferTabBetweenWindowsPreservesWholeTreeAndCounters(t *testing.T) {
	m := crossWindowModel(t)
	if err := m.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	nested, err := m.SplitWithMetrics(1, SplitColumns, PixelRect{Width: 900, Height: 500}, varyingMetrics)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.SplitWithMetrics(nested, SplitRows, PixelRect{Width: 900, Height: 500}, varyingMetrics); err != nil {
		t.Fatal(err)
	}
	beforeRoot := m.tabByID(1).root
	beforeFocus := m.tabByID(1).focused
	beforePane, beforeTab, beforeSplit := m.nextPaneID, m.nextTabID, m.nextSplitID
	beforeAllocated := len(m.allocatedSplits)
	result, err := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: 1, SourceBounds: PixelRect{Width: 300, Height: 240}, DestinationBounds: PixelRect{Width: 1200, Height: 700}, Resolve: varyingMetrics})
	if err != nil {
		t.Fatal(err)
	}
	moved := m.tabByID(1)
	if moved == nil || moved.root != beforeRoot || moved.focused != beforeFocus {
		t.Fatalf("identity changed: %#v", moved)
	}
	if m.nextPaneID != beforePane || m.nextTabID != beforeTab || m.nextSplitID != beforeSplit || len(m.allocatedSplits) != beforeAllocated {
		t.Fatal("whole-tab move allocated identity")
	}
	if !result.SourceWindowEmpty || m.windowByID(1).active != 0 || m.activeWindow != 2 || m.windowByID(2).active != 1 {
		t.Fatalf("result=%#v windows=%#v", result, m.Windows())
	}
	if got := []TabID{m.windowByID(2).tabs[0].id, m.windowByID(2).tabs[1].id}; !reflect.DeepEqual(got, []TabID{2, 1}) {
		t.Fatalf("order=%v", got)
	}
}

func TestTransferPaneBetweenWindowsUsesDifferentBoundsAndMetrics(t *testing.T) {
	m := crossWindowModel(t)
	if err := m.ActivateWindow(1); err != nil {
		t.Fatal(err)
	}
	pane, err := m.SplitWithMetrics(1, SplitColumns, PixelRect{Width: 800, Height: 480}, varyingMetrics)
	if err != nil {
		t.Fatal(err)
	}
	beforeSplit := m.nextSplitID
	result, err := m.TransferPaneBetweenWindows(PaneTransferRequest{SourceWindow: 1, DestinationWindow: 2, Pane: pane, DestinationTab: 2, DestinationPane: 2, Axis: SplitRows, Ratio: DefaultSplitRatio, SourceBounds: PixelRect{Width: 320, Height: 180}, DestinationBounds: PixelRect{Width: 1000, Height: 640}, Resolve: varyingMetrics})
	if err != nil {
		t.Fatal(err)
	}
	if result.Window != 2 || result.SourceWindow != 1 || result.Focused != pane || m.nextSplitID != beforeSplit+1 {
		t.Fatalf("result=%#v", result)
	}
	if len(result.SourceLayout.Panes) != 1 || len(result.DestinationLayout.Panes) != 2 || result.SourceLayout.Panes[0].Pixels.Width != 320 || result.DestinationLayout.Panes[0].Pixels.Width != 1000 {
		t.Fatalf("layouts=%#v/%#v", result.SourceLayout, result.DestinationLayout)
	}
	if owner := m.windowForTab(m.tabForPane(pane).id); owner == nil || owner.id != 2 {
		t.Fatalf("owner=%#v", owner)
	}
}

func TestCrossWindowTransferFailuresAreByteIdentical(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Model) error
	}{
		{"source-window", func(m *Model) error {
			_, e := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 99, DestinationWindow: 2, Tab: 1, Position: 0, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics})
			return e
		}},
		{"destination-window", func(m *Model) error {
			_, e := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 99, Tab: 1, Position: 0, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics})
			return e
		}},
		{"negative-position", func(m *Model) error {
			_, e := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: -1, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics})
			return e
		}},
		{"large-position", func(m *Model) error {
			_, e := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: 3, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics})
			return e
		}},
		{"too-small", func(m *Model) error {
			_, e := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: 0, SourceBounds: transferBounds(), DestinationBounds: PixelRect{Width: 1, Height: 1}, Resolve: varyingMetrics})
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := crossWindowModel(t)
			before := snapshotTransferModel(m)
			if err := tc.run(m); err == nil {
				t.Fatal("expected failure")
			}
			if after := snapshotTransferModel(m); !reflect.DeepEqual(after, before) {
				t.Fatalf("mutation on failure\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestTransferTabRandomizedInvariantsAndRollback(t *testing.T) {
	rng := rand.New(rand.NewSource(93))
	for i := 0; i < 128; i++ {
		m := crossWindowModel(t)
		position := rng.Intn(4) - 1
		before := snapshotTransferModel(m)
		_, err := m.TransferTabBetweenWindows(TabTransferRequest{SourceWindow: 1, DestinationWindow: 2, Tab: 1, Position: position, SourceBounds: PixelRect{Width: 200 + rng.Intn(700), Height: 150 + rng.Intn(500)}, DestinationBounds: PixelRect{Width: 200 + rng.Intn(700), Height: 150 + rng.Intn(500)}, Resolve: varyingMetrics})
		if err != nil && !reflect.DeepEqual(snapshotTransferModel(m), before) {
			t.Fatalf("case %d rollback failed: %v", i, err)
		}
		if err == nil {
			if inv := m.CheckInvariants(); inv != nil {
				t.Fatalf("case %d: %v", i, inv)
			}
		} else if !(errors.Is(err, ErrInvalidTabPosition) || errors.Is(err, ErrTopologyTooSmall)) {
			t.Fatalf("case %d unexpected: %v", i, err)
		}
	}
}

func TestTransferPaneBetweenInactiveWindowsPreservesUnrelatedActiveWindow(t *testing.T) {
	m := crossWindowModel(t)
	third, err := m.CreateWindow("active")
	if err != nil {
		t.Fatal(err)
	}
	if !third.Active {
		t.Fatalf("third window not active: %#v", third)
	}
	result, err := m.TransferPaneBetweenWindows(PaneTransferRequest{
		SourceWindow: 1, DestinationWindow: 2, Pane: 1, DestinationTab: 2, DestinationPane: 2,
		Axis: SplitColumns, Ratio: DefaultSplitRatio, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveWindow != third.ID || result.ActiveTab != third.Tabs[0].ID || result.ActiveFocused != third.Tabs[0].Focused {
		t.Fatalf("unrelated active state changed: result=%#v windows=%#v", result, m.Windows())
	}
	if result.WindowActivated || result.TabActivated || result.FocusChanged {
		t.Fatalf("unrelated active change flags=%#v", result)
	}
	if !result.SourceTabClosed || !result.SourceWindowEmpty {
		t.Fatalf("source emptiness=%#v", result)
	}
}

func TestTransferPaneIntoActiveDestinationReportsOnlyRealFocusChange(t *testing.T) {
	m := crossWindowModel(t) // window 2 is active
	result, err := m.TransferPaneBetweenWindows(PaneTransferRequest{
		SourceWindow: 1, DestinationWindow: 2, Pane: 1, DestinationTab: 2, DestinationPane: 2,
		Axis: SplitRows, Ratio: DefaultSplitRatio, SourceBounds: transferBounds(), DestinationBounds: transferBounds(), Resolve: varyingMetrics,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.WindowActivated || result.TabActivated || !result.FocusChanged {
		t.Fatalf("changes=%#v", result)
	}
}
