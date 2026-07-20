package mux

import (
	"errors"
	"reflect"
	"testing"
)

func windowTestGeometry() (PixelRect, CellMetrics) {
	return PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}
}

func TestMuxCreateActivateCloseWindowOwnsRuntimeExactlyOnce(t *testing.T) {
	m, first, wakes := newTestMux(t)
	bounds, metrics := windowTestGeometry()
	factory := m.sessions.factoryForTest().(*fakeFactory)
	view, events, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "second")
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != 2 || len(view.Tabs) != 1 || view.Tabs[0].Focused != 2 || m.model.ActiveWindow().ID != 2 || m.sessions.count() != 2 {
		t.Fatalf("view=%#v events=%#v count=%d", view, events, m.sessions.count())
	}
	second := factory.sessions[1]
	if err := second.feed([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	m.Drain(16)
	if pv, ok := m.PaneView(2); !ok || len(pv.Snapshot.Cells) == 0 || pv.Snapshot.Cells[0].Rune != 'o' {
		t.Fatalf("ingress=%#v", pv)
	}
	result, closeEvents, err := m.CloseWindow(2)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Closed || result.Empty || result.Active != 1 || m.sessions.count() != 1 || second.closes() != 1 || first.closes() != 0 {
		t.Fatalf("result=%#v count=%d closes=%d/%d", result, m.sessions.count(), first.closes(), second.closes())
	}
	for _, event := range closeEvents {
		if event.Kind == PaneClosed || event.Kind == TabClosed || event.Kind == WindowClosed {
			if event.Window != 2 {
				t.Fatalf("unaddressed %#v", event)
			}
		}
	}
	again, againEvents, err := m.CloseWindow(2)
	if err != nil || again.Closed || len(againEvents) != 0 || second.closes() != 1 {
		t.Fatalf("repeat=%#v events=%#v err=%v closes=%d", again, againEvents, err, second.closes())
	}
}

func TestMuxCreateWindowFailureRestoresModelAndRegistry(t *testing.T) {
	for _, stage := range []string{"reserve", "spawn", "register", "start", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m, first, _ := newTestMux(t)
			bounds, metrics := windowTestGeometry()
			beforeWindows := m.model.Windows()
			beforeWindow, beforeTab, beforePane := m.model.nextWindowID, m.model.nextTabID, m.model.nextPaneID
			beforeCount := m.sessions.count()
			factory := m.sessions.factoryForTest().(*fakeFactory)
			m.windowFault = func(got string) error {
				if got == stage {
					return errors.New(stage)
				}
				return nil
			}
			if _, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "failed"); err == nil {
				t.Fatal("expected failure")
			}
			if !reflect.DeepEqual(beforeWindows, m.model.Windows()) || m.model.nextWindowID != beforeWindow || m.model.nextTabID != beforeTab || m.model.nextPaneID != beforePane || m.sessions.count() != beforeCount {
				t.Fatalf("mutation windows=%#v count=%d ids=%d/%d/%d", m.model.Windows(), m.sessions.count(), m.model.nextWindowID, m.model.nextTabID, m.model.nextPaneID)
			}
			if first.closes() != 0 {
				t.Fatal("existing session closed")
			}
			for _, session := range factory.sessions[1:] {
				if session.closes() != 1 {
					t.Fatalf("leaked candidate at %s closes=%d", stage, session.closes())
				}
			}
		})
	}
}

func TestMuxCloseFinalWindowReportsProcessEmptiness(t *testing.T) {
	m, first, _ := newTestMux(t)
	result, events, err := m.CloseWindow(1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Empty || result.Active != 0 || m.sessions.count() != 0 || first.closes() != 1 {
		t.Fatalf("result=%#v events=%#v", result, events)
	}
	if len(events) == 0 || events[len(events)-1].Kind != WindowClosed {
		t.Fatalf("events=%#v", events)
	}
}

func TestMuxRollbackWindowReusesUnpublishedIDs(t *testing.T) {
	m, _, _ := newTestMux(t)
	bounds, metrics := windowTestGeometry()
	beforeWindow, beforeTab, beforePane := m.model.nextWindowID, m.model.nextTabID, m.model.nextPaneID
	view, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RollbackWindow(view.ID); err != nil {
		t.Fatal(err)
	}
	if m.model.nextWindowID != beforeWindow || m.model.nextTabID != beforeTab || m.model.nextPaneID != beforePane || m.sessions.count() != 1 {
		t.Fatalf("rollback ids=%d/%d/%d count=%d", m.model.nextWindowID, m.model.nextTabID, m.model.nextPaneID, m.sessions.count())
	}
	reused, _, err := m.CreateWindow(SpawnSpec{}, bounds, metrics, "reused")
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != view.ID || reused.Tabs[0].ID != beforeTab || reused.Tabs[0].Focused != beforePane {
		t.Fatalf("reused=%#v", reused)
	}
}
