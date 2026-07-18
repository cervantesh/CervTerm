package mux

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func newNestedTopologyMux(t *testing.T) (*Mux, *fakeFactory) {
	t.Helper()
	factory := &fakeFactory{}
	m := New(factory, Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	bounds := PixelRect{Width: 800, Height: 480}
	metrics := CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, bounds, metrics); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Split(1, SplitColumns, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FocusPane(1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Split(1, SplitRows, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FocusPane(1); err != nil {
		t.Fatal(err)
	}
	return m, factory
}

func TestDirectionalNeighborNestedTopologyIsDeterministic(t *testing.T) {
	m, _ := newNestedTopologyMux(t)
	for i := 0; i < 100; i++ {
		right, err := m.model.DirectionalNeighbor(1, FocusRight, m.bounds, m.resolveMetrics)
		if err != nil || right != 2 {
			t.Fatalf("iteration %d right neighbor=%d err=%v, want pane 2", i, right, err)
		}
		down, err := m.model.DirectionalNeighbor(1, FocusDown, m.bounds, m.resolveMetrics)
		if err != nil || down != 3 {
			t.Fatalf("iteration %d down neighbor=%d err=%v, want pane 3", i, down, err)
		}
	}
}

func TestResizeCurrentPaneIsTransactionalAndDefersPTY(t *testing.T) {
	m, factory := newNestedTopologyMux(t)
	before, _ := m.PaneView(1)
	counts := resizeCounts(factory)
	events, err := m.ResizeCurrentPane(FocusRight, 4)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := m.PaneView(1)
	if after.Geometry.Cols <= before.Geometry.Cols || len(events) == 0 {
		t.Fatalf("resize before=%#v after=%#v events=%#v", before.Geometry, after.Geometry, events)
	}
	if got := resizeCounts(factory); !reflect.DeepEqual(got, counts) {
		t.Fatalf("PTY resize was not deferred: before=%v after=%v", counts, got)
	}
	for _, event := range events {
		if event.Kind != PaneGeometryChanged && event.Kind != PaneDirty {
			t.Fatalf("unexpected geometry event %#v", event)
		}
	}
}

func TestSwapAndMoveCurrentPaneFocusSemantics(t *testing.T) {
	t.Run("swap transfers focus to preserve visual slot", func(t *testing.T) {
		m, factory := newNestedTopologyMux(t)
		beforeOne, _ := m.PaneView(1)
		beforeTwo, _ := m.PaneView(2)
		counts := resizeCounts(factory)
		events, err := m.SwapCurrentPane(FocusRight)
		if err != nil {
			t.Fatal(err)
		}
		focused, _ := m.FocusedPane()
		afterOne, _ := m.PaneView(1)
		afterTwo, _ := m.PaneView(2)
		if focused != 2 || afterOne.Geometry.Pixels != beforeTwo.Geometry.Pixels || afterTwo.Geometry.Pixels != beforeOne.Geometry.Pixels {
			t.Fatalf("focus=%d one=%#v two=%#v", focused, afterOne.Geometry, afterTwo.Geometry)
		}
		if len(events) == 0 || events[0].Kind != PaneFocused || events[0].Pane != 2 {
			t.Fatalf("swap events=%#v", events)
		}
		if got := resizeCounts(factory); !reflect.DeepEqual(got, counts) {
			t.Fatalf("swap notified PTYs: before=%v after=%v", counts, got)
		}
	})

	t.Run("move preserves focused pane identity", func(t *testing.T) {
		m, factory := newNestedTopologyMux(t)
		beforeOne, _ := m.PaneView(1)
		beforeTwo, _ := m.PaneView(2)
		counts := resizeCounts(factory)
		events, err := m.MoveCurrentPane(FocusRight)
		if err != nil {
			t.Fatal(err)
		}
		focused, _ := m.FocusedPane()
		afterOne, _ := m.PaneView(1)
		afterTwo, _ := m.PaneView(2)
		if focused != 1 || afterOne.Geometry.Pixels != beforeTwo.Geometry.Pixels || afterTwo.Geometry.Pixels != beforeOne.Geometry.Pixels {
			t.Fatalf("focus=%d one=%#v two=%#v", focused, afterOne.Geometry, afterTwo.Geometry)
		}
		for _, event := range events {
			if event.Kind == PaneFocused {
				t.Fatalf("move emitted focus transfer: %#v", events)
			}
		}
		if got := resizeCounts(factory); !reflect.DeepEqual(got, counts) {
			t.Fatalf("move notified PTYs: before=%v after=%v", counts, got)
		}
	})
}

func TestTopologyFailuresRollbackExactlyWithoutEffects(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Mux) ([]Event, error)
		want error
	}{
		{name: "no neighbor", run: func(m *Mux) ([]Event, error) { return m.MoveCurrentPane(FocusLeft) }, want: ErrNoPaneInDirection},
		{name: "invalid direction", run: func(m *Mux) ([]Event, error) { return m.SwapCurrentPane(Direction(99)) }, want: ErrInvalidDirection},
		{name: "invalid delta", run: func(m *Mux) ([]Event, error) { return m.ResizeCurrentPane(FocusRight, 0) }, want: ErrInvalidResizeDelta},
		{name: "undersized", run: func(m *Mux) ([]Event, error) { return m.ResizeCurrentPane(FocusRight, 10_000) }, want: ErrTopologyTooSmall},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, factory := newNestedTopologyMux(t)
			beforeTree := topologyString(m.model.root)
			beforeFocus := m.model.focused
			beforeViews := paneViews(m)
			beforeCounts := resizeCounts(factory)
			events, err := tt.run(m)
			if !errors.Is(err, tt.want) || len(events) != 0 {
				t.Fatalf("events=%#v err=%v, want %v", events, err, tt.want)
			}
			if got := topologyString(m.model.root); got != beforeTree || m.model.focused != beforeFocus || !reflect.DeepEqual(paneViews(m), beforeViews) {
				t.Fatalf("failure mutated state: tree %s -> %s focus %d -> %d", beforeTree, got, beforeFocus, m.model.focused)
			}
			if got := resizeCounts(factory); !reflect.DeepEqual(got, beforeCounts) {
				t.Fatalf("failure resized PTYs: before=%v after=%v", beforeCounts, got)
			}
		})
	}
}

func topologyString(n *node) string {
	if n == nil {
		return "nil"
	}
	if n.isLeaf() {
		return fmt.Sprintf("p%d", n.pane)
	}
	return fmt.Sprintf("s%d:%d:%d(%s,%s)", n.split, n.axis, n.ratio, topologyString(n.first), topologyString(n.second))
}

func paneViews(m *Mux) []PaneView {
	views := make([]PaneView, 0, len(m.PaneIDs()))
	for _, id := range m.PaneIDs() {
		view, _ := m.PaneView(id)
		views = append(views, view)
	}
	return views
}

func resizeCounts(factory *fakeFactory) []int {
	counts := make([]int, len(factory.sessions))
	for i, session := range factory.sessions {
		counts[i] = session.resizeCount()
	}
	return counts
}
