package mux

import (
	"reflect"
	"testing"

	"cervterm/internal/pty"
)

func TestFreshSessionSnapshotExportsDetachedSecretFreeTopology(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	initial := SpawnSpec{TargetID: "dev", Options: pty.Options{ShellProgram: "pwsh", ShellArgs: []string{"-NoLogo"}, WorkingDirectory: "C:/start", Env: map[string]string{"SECRET_TOKEN": "never-export"}}}
	_, first, _, err := m.Bootstrap(initial, PixelRect{Width: 1000, Height: 600}, CellMetrics{CellWidth: 10, CellHeight: 20})
	if err != nil {
		t.Fatal(err)
	}
	initial.Options.ShellArgs[0] = "mutated"
	secondSpec := SpawnSpec{Options: pty.Options{ShellProgram: "cmd", ShellArgs: []string{"/k"}, WorkingDirectory: "C:/two", Env: map[string]string{"PASSWORD": "never-export"}}}
	second, _, err := m.SpawnSplit(first, SplitColumns, secondSpec)
	if err != nil {
		t.Fatal(err)
	}
	lookupPaneForTest(t, m.sessions, second).cwd = "C:/live"
	snapshot, err := m.FreshSessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveWorkspace != 0 || len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].ActiveWindow != 0 {
		t.Fatalf("workspace=%#v", snapshot)
	}
	window := snapshot.Workspaces[0].Windows[0]
	if window.ActiveTab != 0 || len(window.Tabs) != 1 || window.Tabs[0].FocusedLeaf != 1 {
		t.Fatalf("window=%#v", window)
	}
	root := window.Tabs[0].Root
	if root.Type != "split" || root.Axis != SplitColumns || root.Ratio != DefaultSplitRatio || root.First == nil || root.Second == nil {
		t.Fatalf("root=%#v", root)
	}
	if got := root.First.Launch; got == nil || got.TargetID != "dev" || got.Program != "pwsh" || !reflect.DeepEqual(got.Args, []string{"-NoLogo"}) || got.CWD != "C:/start" {
		t.Fatalf("first=%#v", got)
	}
	if got := root.Second.Launch; got == nil || got.Program != "cmd" || got.CWD != "C:/live" {
		t.Fatalf("second=%#v", got)
	}
	root.First.Launch.Args[0] = "snapshot-mutation"
	again, err := m.FreshSessionSnapshot()
	if err != nil || again.Workspaces[0].Windows[0].Tabs[0].Root.First.Launch.Args[0] != "-NoLogo" {
		t.Fatalf("snapshot aliases mux: %#v err=%v", again, err)
	}
}

func TestFreshSessionSnapshotRejectsUnbootstrappedAndMissingRegistry(t *testing.T) {
	m := New(&fakeFactory{}, Options{})
	if _, err := m.FreshSessionSnapshot(); err == nil {
		t.Fatal("unbootstrapped mux exported")
	}
	_, pane, _, err := m.Bootstrap(SpawnSpec{Options: pty.Options{ShellProgram: "shell"}}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := m.sessions.lookup(pane)
	m.sessions.abort(pane, owner)
	if _, err := m.FreshSessionSnapshot(); err == nil {
		t.Fatal("missing registry pane exported")
	}
	_ = owner.close()
	_ = m.Shutdown()
}
