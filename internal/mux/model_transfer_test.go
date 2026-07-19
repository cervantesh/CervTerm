package mux

import (
	"errors"
	"reflect"
	"testing"
)

func transferResolver(id PaneID) (CellMetrics, bool) {
	if id == 0 {
		return CellMetrics{}, false
	}
	return CellMetrics{CellWidth: 8, CellHeight: 16}, true
}
func transferBounds() PixelRect { return PixelRect{Width: 800, Height: 480} }

func transferModel(t *testing.T) (*Model, PaneID) {
	t.Helper()
	m := NewModel()
	if err := m.commitTab(2, 2, "two"); err != nil {
		t.Fatal(err)
	}
	if err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	pane, err := m.SplitWithMetrics(1, SplitColumns, transferBounds(), transferResolver)
	if err != nil {
		t.Fatal(err)
	}
	return m, pane
}

func TestTransferPaneCommitsBothTreesAndPreservesIdentity(t *testing.T) {
	m, pane := transferModel(t)
	beforeSplit := m.nextSplitID
	result, err := m.TransferPane(pane, 2, 2, SplitRows, DefaultSplitRatio, transferBounds(), transferResolver)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pane != pane || result.SourceTab != 1 || result.DestinationTab != 2 || result.SourceTabClosed || m.nextSplitID != beforeSplit+1 {
		t.Fatalf("result=%#v next=%d", result, m.nextSplitID)
	}
	if got := m.tabForPane(pane); got == nil || got.id != 2 {
		t.Fatalf("owner=%#v", got)
	}
	if m.tabByID(1).focused != 1 || m.tabByID(2).focused != pane || m.TabID() != 1 {
		t.Fatalf("tabs=%#v", m.Tabs())
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestTransferFinalPaneRemovesSourceAndSelectsAdjacentTab(t *testing.T) {
	m := NewModel()
	if err := m.commitTab(2, 2, "two"); err != nil {
		t.Fatal(err)
	}
	if err := m.commitTab(3, 3, "three"); err != nil {
		t.Fatal(err)
	}
	if err := m.ActivateTab(2); err != nil {
		t.Fatal(err)
	}
	result, err := m.TransferPane(2, 3, 3, SplitColumns, DefaultSplitRatio, transferBounds(), transferResolver)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SourceTabClosed || m.tabByID(2) != nil || m.TabID() != 3 || result.ActiveTab != 3 || !result.ActiveChanged {
		t.Fatalf("result=%#v tabs=%#v", result, m.Tabs())
	}
	if got := m.tabForPane(2); got == nil || got.id != 3 {
		t.Fatalf("owner=%#v", got)
	}
}

func TestTransferPaneFailureRollsBackEveryLogicalOwner(t *testing.T) {
	m, pane := transferModel(t)
	beforeTabs := m.Tabs()
	beforeSplit := m.nextSplitID
	beforeAllocated := len(m.allocatedSplits)
	_, err := m.TransferPane(pane, 2, 2, SplitColumns, DefaultSplitRatio, PixelRect{Width: 16, Height: 16}, transferResolver)
	if !errors.Is(err, ErrTopologyTooSmall) {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.nextSplitID != beforeSplit || len(m.allocatedSplits) != beforeAllocated {
		t.Fatalf("tabs=%#v next=%d allocated=%d", m.Tabs(), m.nextSplitID, len(m.allocatedSplits))
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestTransferPaneRejectsSameTabAndWrongDestinationOwnership(t *testing.T) {
	m, pane := transferModel(t)
	if _, err := m.TransferPane(1, 1, pane, SplitColumns, DefaultSplitRatio, transferBounds(), transferResolver); !errors.Is(err, ErrSameTabTransfer) {
		t.Fatalf("same tab err=%v", err)
	}
	if _, err := m.TransferPane(pane, 2, 99, SplitColumns, DefaultSplitRatio, transferBounds(), transferResolver); !errors.Is(err, ErrPaneNotFound) {
		t.Fatalf("target err=%v", err)
	}
}

func TestTransferPaneCollapsesNestedSourceCandidate(t *testing.T) {
	m, pane := transferModel(t)
	survivor, err := m.SplitWithMetrics(pane, SplitRows, transferBounds(), transferResolver)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.TransferPane(pane, 2, 2, SplitColumns, DefaultSplitRatio, transferBounds(), transferResolver); err != nil {
		t.Fatal(err)
	}
	if got := paneIDs(m.tabByID(1).root); !reflect.DeepEqual(got, []PaneID{1, survivor}) {
		t.Fatalf("source panes=%v", got)
	}
	if got := paneIDs(m.tabByID(2).root); !reflect.DeepEqual(got, []PaneID{2, pane}) {
		t.Fatalf("destination panes=%v", got)
	}
	if m.tabByID(1).focused != survivor {
		t.Fatalf("source focus=%d", m.tabByID(1).focused)
	}
	if err := m.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestTransferPaneDeterministicTopologyMatrix(t *testing.T) {
	for i := 0; i < 48; i++ {
		m, pane := transferModel(t)
		nested, err := m.SplitWithMetrics(pane, SplitAxis(i%2+1), transferBounds(), transferResolver)
		if err != nil {
			t.Fatal(err)
		}
		moved := pane
		if i%2 == 1 {
			moved = nested
		}
		ratio := SplitRatio(2500 + (i*7919)%5000)
		if _, err := m.TransferPane(moved, 2, 2, SplitAxis((i+1)%2+1), ratio, transferBounds(), transferResolver); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if owner := m.tabForPane(moved); owner == nil || owner.id != 2 {
			t.Fatalf("case %d owner=%#v", i, owner)
		}
		if err := m.CheckInvariants(); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
