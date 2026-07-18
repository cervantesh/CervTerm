//go:build glfw

package glfwgl

import (
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/config"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
)

func newLaunchMenuApp(t *testing.T) (*App, *capturingTestFactory) {
	t.Helper()
	factory := &capturingTestFactory{}
	m := termmux.New(factory, termmux.Options{})
	_, pane, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{mux: m, focusedPane: pane, paneUI: map[termmux.PaneID]*paneUIState{}, pendingPaneScroll: map[termmux.PaneID]int{}, cellW: 8, cellH: 16}
	a.handleMuxEvents(events)
	a.syncFocusedProjection()
	factory.reset()
	t.Cleanup(func() { _ = m.Shutdown() })
	return a, factory
}

func launchTarget() config.LaunchTarget {
	return config.LaunchTarget{ID: "tool", Label: "Tool", Program: "tool.exe", Args: []string{"a b", "&literal"}, CWD: "C:/work", Env: map[string]string{"TOKEN": "secret"}}
}

func TestLaunchMenuSuccessReResolvesAndSpawnsExactlyOnePane(t *testing.T) {
	a, factory := newLaunchMenuApp(t)
	a.desiredCfg = config.Defaults()
	a.desiredCfg.LaunchMenu = []config.LaunchTarget{launchTarget()}
	origin := a.focusedPane
	if err := a.openLaunchMenu(origin); err != nil {
		t.Fatal(err)
	}
	a.applyModalIntents(a.modal.Accept())
	if a.modal.Active() || len(a.mux.PaneIDs()) != 2 {
		t.Fatalf("active=%v panes=%v", a.modal.Active(), a.mux.PaneIDs())
	}
	got, ok := factory.last()
	if !ok {
		t.Fatal("spawn options missing")
	}
	want := pty.Options{ShellProgram: "tool.exe", ShellArgs: []string{"a b", "&literal"}, WorkingDirectory: "C:/work", Env: map[string]string{"TOKEN": "secret"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options=%#v want=%#v", got, want)
	}
}

func TestLaunchMenuStaleIDPreservesModalAndDoesNotSpawn(t *testing.T) {
	a, factory := newLaunchMenuApp(t)
	a.desiredCfg = config.Defaults()
	a.desiredCfg.LaunchMenu = []config.LaunchTarget{launchTarget()}
	if err := a.openLaunchMenu(a.focusedPane); err != nil {
		t.Fatal(err)
	}
	a.desiredCfg.LaunchMenu = nil
	a.applyModalIntents(a.modal.Accept())
	if !a.modal.Active() || !strings.Contains(a.modal.Snapshot().Error, "no longer") || len(a.mux.PaneIDs()) != 1 {
		t.Fatalf("state=%#v panes=%v", a.modal.Snapshot(), a.mux.PaneIDs())
	}
	if _, ok := factory.last(); ok {
		t.Fatal("stale target spawned")
	}
}

func TestLaunchMenuSpawnFailurePreservesFullState(t *testing.T) {
	a := newMuxTestApp(t, 800, 480)
	a.desiredCfg = config.Defaults()
	a.desiredCfg.LaunchMenu = []config.LaunchTarget{launchTarget()}
	if err := a.openLaunchMenu(a.focusedPane); err != nil {
		t.Fatal(err)
	}
	a.modal.AppendRune('T')
	before := a.modal.Snapshot()
	a.applyModalIntents(a.modal.Accept())
	after := a.modal.Snapshot()
	if !a.modal.Active() || after.Mode != modal.ModeLaunchMenu || string(after.Query) != string(before.Query) || after.Selection != before.Selection || len(a.mux.PaneIDs()) != 1 || after.Error == "" {
		t.Fatalf("before=%#v after=%#v panes=%v", before, after, a.mux.PaneIDs())
	}
}
