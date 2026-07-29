package mux

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/pty"
	"cervterm/internal/termimage"
)

func tabMetrics() CellMetrics { return CellMetrics{CellWidth: 8, CellHeight: 16} }

func TestSpawnActivateRenameMoveAndCloseTabLifecycle(t *testing.T) {
	m, first, _ := newTestMux(t)
	factory := m.sessions.factory.(*fakeFactory)
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
	factory := m.sessions.factory.(*fakeFactory)
	candidate := newFakeSession()
	spawnFailure := errors.New("spawn failed")
	closeFailure := errors.New("partial session close failed")
	candidate.closeErr = closeFailure
	factory.err = spawnFailure
	factory.sessionOnError = candidate
	beforeTabs := m.Tabs()
	beforeTabID, beforePaneID := m.model.nextTabID, m.model.nextPaneID
	beforePanes := len(m.sessions.panes)
	_, _, events, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "bad")
	if !errors.Is(err, spawnFailure) || !errors.Is(err, closeFailure) || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextTabID != beforeTabID || m.model.nextPaneID != beforePaneID || len(m.sessions.panes) != beforePanes || candidate.closes() != 1 {
		t.Fatalf("tabs=%#v ids=%d/%d panes=%d close=%d", m.Tabs(), m.model.nextTabID, m.model.nextPaneID, len(m.sessions.panes), candidate.closes())
	}
}

func newImageTabRollbackMux(t *testing.T) (*Mux, *fakeFactory) {
	t.Helper()
	limits := termimage.DefaultLimits()
	factory := &fakeFactory{}
	m := New(factory, Options{ImageLimits: &limits, KittyEnabled: true})
	if _, pane, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, tabMetrics()); err != nil || pane != 1 {
		t.Fatalf("bootstrap pane=%d err=%v", pane, err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, factory
}

func assertSpawnTabRollbackPristine(t *testing.T, m *Mux, beforeTabs []TabView, failed *pane, failedSession *fakeSession) {
	t.Helper()
	panes, reserved, started := m.sessions.activeCounts()
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextTabID != 2 || m.model.nextPaneID != 2 || panes != 1 || reserved != 0 || started != 1 {
		t.Fatalf("rollback tabs=%#v ids=%d/%d counts=%d/%d/%d", m.Tabs(), m.model.nextTabID, m.model.nextPaneID, panes, reserved, started)
	}
	if failed == nil || failedSession == nil {
		t.Fatalf("missing failed pane/session: pane=%#v session=%#v", failed, failedSession)
	}
	if failed.state != PaneStateClosed || failed.imageStore != nil || failed.session != failedSession || failedSession.closes() != 1 {
		t.Fatalf("failed pane=%#v store=%p session=%#v closes=%d", failed, failed.imageStore, failedSession, failedSession.closes())
	}
	if _, owned := m.sessions.lookup(2); owned || m.sessions.wasClosed(2) {
		t.Fatal("failed unpublished pane remained registered or tombstoned")
	}
	if _, exists := m.paneMetrics[2]; exists {
		t.Fatal("failed unpublished pane leaked metrics")
	}
	if events := m.Drain(32); len(events) != 0 {
		t.Fatalf("failed unpublished pane leaked events %#v", events)
	}
	if err := m.model.CheckInvariants(); err != nil {
		t.Fatal(err)
	}
}

func TestSpawnTabClosePreflightFailureDetachesClosesOnceAndRetries(t *testing.T) {
	m, factory := newImageTabRollbackMux(t)
	beforeTabs := m.Tabs()
	injected := errors.New("injected close preflight failure")
	closeFailure := errors.New("injected rollback close failure")
	var failed *pane
	m.rollbackFault = func(stage string, p *pane) error {
		if stage != "spawn-tab-close-preflight" {
			return nil
		}
		failed = p
		p.session.(*fakeSession).closeErr = closeFailure
		return injected
	}
	_, _, events, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "failed")
	m.rollbackFault = nil
	if !errors.Is(err, injected) || !errors.Is(err, closeFailure) || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if len(factory.sessions) != 2 {
		t.Fatalf("spawned sessions=%d", len(factory.sessions))
	}
	assertSpawnTabRollbackPristine(t, m, beforeTabs, failed, factory.sessions[1])
	tab, paneID, retryEvents, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "retry")
	if err != nil || tab != 2 || paneID != 2 || len(retryEvents) != 4 {
		t.Fatalf("retry tab=%d pane=%d events=%#v err=%v", tab, paneID, retryEvents, err)
	}
}

func TestSpawnTabRealClosePreflightFailureRetainsBoundedRollbackAndNoReader(t *testing.T) {
	m, factory := newImageTabRollbackMux(t)
	beforeTabs := m.Tabs()
	var failed *pane
	var held *preparedPaneClose
	m.rollbackFault = func(stage string, p *pane) error {
		if stage != "spawn-tab-close-preflight" {
			return nil
		}
		failed = p
		var err error
		held, err = p.prepareClose()
		return err
	}
	t.Cleanup(func() {
		if held != nil && !held.finished {
			_ = held.abort()
		}
	})

	_, _, events, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "blocked")
	m.rollbackFault = nil
	if !errors.Is(err, termimage.ErrOwnerBusy) || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if failed == nil || held == nil || m.unpublishedRollback == nil {
		t.Fatalf("failed=%p held=%p rollback=%p", failed, held, m.unpublishedRollback)
	}
	failedStore := failed.imageStore
	panes, reserved, started := m.sessions.activeCounts()
	if panes != 1 || reserved != 0 || started != 1 {
		t.Fatalf("hidden registry state counts=%d/%d/%d", panes, reserved, started)
	}
	if _, registered := m.sessions.lookup(2); registered || m.sessions.wasClosed(2) {
		t.Fatal("persistently blocked unpublished pane entered registry or tombstones")
	}
	if failed.session == nil || failed.session.(*fakeSession).closes() != 0 || failed.session.(*fakeSession).readers() != 0 || failedStore == nil || failedStore.Closed() {
		t.Fatalf("retained pane state=%v session=%#v closes/readers=%d/%d store=%p closed=%v", failed.state, failed.session, failed.session.(*fakeSession).closes(), failed.session.(*fakeSession).readers(), failedStore, failedStore == nil || failedStore.Closed())
	}
	if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextTabID != 2 || m.model.nextPaneID != 2 || len(m.Drain(32)) != 0 {
		t.Fatal("real preflight failure published events or identities")
	}
	retainedRollback := m.unpublishedRollback
	_, _, blockedEvents, blockedErr := m.SpawnTab(SpawnSpec{}, tabMetrics(), "still-blocked")
	if !errors.Is(blockedErr, termimage.ErrOwnerBusy) || len(blockedEvents) != 0 || m.unpublishedRollback != retainedRollback || len(factory.sessions) != 2 {
		t.Fatalf("persistent retry events=%#v err=%v rollback=%p sessions=%d", blockedEvents, blockedErr, m.unpublishedRollback, len(factory.sessions))
	}

	if err := held.abort(); err != nil {
		t.Fatal(err)
	}
	tab, paneID, retryEvents, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "retry")
	if err != nil || tab != 2 || paneID != 2 || len(retryEvents) != 4 {
		t.Fatalf("retry tab=%d pane=%d events=%#v err=%v", tab, paneID, retryEvents, err)
	}
	if m.unpublishedRollback != nil || failed.state != PaneStateClosed || failed.imageStore != nil || !failedStore.Closed() || failed.session.(*fakeSession).closes() != 1 || failed.session.(*fakeSession).readers() != 0 {
		t.Fatalf("rollback leak candidate=%p state=%v store=%p closed=%v closes/readers=%d/%d", m.unpublishedRollback, failed.state, failed.imageStore, failedStore.Closed(), failed.session.(*fakeSession).closes(), failed.session.(*fakeSession).readers())
	}
	if len(factory.sessions) != 3 {
		t.Fatalf("spawned sessions=%d want bootstrap+failed+retry", len(factory.sessions))
	}
}

func TestSpawnTabCloseAbortAndModelFailuresFullyUnwind(t *testing.T) {
	for _, stage := range []string{"spawn-tab-close-abort", "spawn-tab-model"} {
		t.Run(stage, func(t *testing.T) {
			m, factory := newImageTabRollbackMux(t)
			beforeTabs := m.Tabs()
			injected := errors.New("injected " + stage)
			var failed *pane
			m.rollbackFault = func(got string, p *pane) error {
				if got != stage {
					return nil
				}
				failed = p
				if !reflect.DeepEqual(m.Tabs(), beforeTabs) || m.model.nextTabID != 2 || m.model.nextPaneID != 2 {
					t.Fatalf("%s observed model publication tabs=%#v ids=%d/%d", stage, m.Tabs(), m.model.nextTabID, m.model.nextPaneID)
				}
				probe, probeErr := p.prepareClose()
				if stage == "spawn-tab-close-abort" {
					if probe != nil || !errors.Is(probeErr, termimage.ErrOwnerBusy) {
						t.Fatalf("retained close token probe=%#v err=%v", probe, probeErr)
					}
				} else {
					if probeErr != nil {
						t.Fatalf("close token was not aborted before model stage: %v", probeErr)
					}
					if err := probe.abort(); err != nil {
						t.Fatal(err)
					}
				}
				return injected
			}
			_, _, events, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "failed")
			m.rollbackFault = nil
			if !errors.Is(err, injected) || len(events) != 0 || len(factory.sessions) != 2 {
				t.Fatalf("events=%#v sessions=%d err=%v", events, len(factory.sessions), err)
			}
			assertSpawnTabRollbackPristine(t, m, beforeTabs, failed, factory.sessions[1])
			tab, paneID, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "retry")
			if err != nil || tab != 2 || paneID != 2 {
				t.Fatalf("retry tab=%d pane=%d err=%v", tab, paneID, err)
			}
		})
	}
}

func TestCloseTabAggregatesErrorAfterAtomicDetachAndAddressesFinalEvent(t *testing.T) {
	m, first, _ := newTestMux(t)
	first.closeErr = errors.New("close failed")
	events, err := m.CloseTab(1)
	if err == nil || !m.model.Empty() || len(m.sessions.panes) != 0 || first.closes() != 1 {
		t.Fatalf("err=%v empty=%v panes=%d closes=%d", err, m.model.Empty(), len(m.sessions.panes), first.closes())
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
	factory := m.sessions.factory.(*fakeFactory)
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
	view, ok := m.sessions.panes[pane]
	if !ok || len(view.snapshot.Cells) == 0 || view.snapshot.Cells[0].Rune != 'b' {
		t.Fatalf("background pane not advanced")
	}
}

func TestTabForPaneFindsInactiveOwnership(t *testing.T) {
	m, _, _ := newTestMux(t)
	_, pane, _, err := m.SpawnTab(SpawnSpec{}, tabMetrics(), "two")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	if tab, ok := m.TabForPane(pane); !ok || tab != 2 {
		t.Fatalf("tab=%d ok=%v", tab, ok)
	}
	if _, ok := m.TabForPane(999); ok {
		t.Fatal("unknown pane resolved")
	}
}
