//go:build glfw

package glfwgl

import (
	"image/color"
	"sort"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/modal"
	"cervterm/internal/script"
)

func TestCommandPaletteSnapshotIsBoundedDiscoverableAndSorted(t *testing.T) {
	a := newRunningMuxTestApp(t)
	entries, activations := a.commandPaletteSnapshot(true)
	if len(entries) == 0 || len(entries) > maxCommandPaletteEntries || len(entries) != len(activations) {
		t.Fatalf("entries=%d activations=%d", len(entries), len(activations))
	}
	labels := make([]string, len(entries))
	found := false
	for i, e := range entries {
		labels[i] = e.Label
		if e.ID == "action:"+string(termaction.IDActivateCommandPalette) {
			found = true
		}
	}
	if !sort.StringsAreSorted(labels) {
		t.Fatalf("labels not sorted: %v", labels)
	}
	if !found {
		t.Fatal("discoverable palette action missing")
	}
}

func TestCommandPaletteDiscoversSafeWindowActionsOnly(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.windowID = 1
	entries, _ := a.commandPaletteSnapshot(true)
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.ID] = true
	}
	for _, id := range []termaction.ID{termaction.IDNewWindow, termaction.IDCloseWindow, termaction.IDFocusWindow} {
		if !found["action:"+string(id)] {
			t.Fatalf("safe window action %s missing", id)
		}
	}
	for _, id := range []termaction.ID{termaction.IDMoveTabToWindow, termaction.IDMovePaneToWindow} {
		if found["action:"+string(id)] {
			t.Fatalf("parameterized window action %s exposed", id)
		}
	}
}

func TestCommandPaletteOpensAndSuccessfulActionCloses(t *testing.T) {
	a := newRunningMuxTestApp(t)
	if err := a.openCommandPalette(); err != nil {
		t.Fatal(err)
	}
	if a.modal.Mode() != modal.ModeCommandPalette {
		t.Fatalf("mode=%v", a.modal.Mode())
	}
	var entry modal.Entry
	for _, candidate := range a.modal.Snapshot().Entries {
		if candidate.ID == "action:"+string(termaction.IDToggleStats) {
			entry = candidate
			break
		}
	}
	if entry.ID == "" {
		t.Fatal("toggle stats entry missing")
	}
	if err := a.acceptCommandPalette(entry, a.focusedPane); err != nil {
		t.Fatal(err)
	}
	a.applyModalIntents(a.modal.Close())
	if !a.showStats || a.modal.Active() {
		t.Fatalf("stats=%v active=%v", a.showStats, a.modal.Active())
	}
}

func TestCommandPaletteReloadInvalidationPreservesModalErrorState(t *testing.T) {
	a := newRunningMuxTestApp(t)
	entry := modal.Entry{ID: "binding:root//0", Label: "Old callback"}
	if !a.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(a.focusedPane), modal.FocusIdentity(a.focusedPane), []modal.Entry{entry}) {
		t.Fatal("open")
	}
	ref := script.CallbackRef{Domain: script.CallbackRoot, Slot: 0}
	a.commandPalette = map[string]commandPaletteActivation{entry.ID: {callback: &ref, generation: 1}}
	a.scriptGeneration = 2
	a.applyModalIntents(a.modal.Accept())
	state := a.modal.Snapshot()
	if !a.modal.Active() || state.Error == "" || string(state.Query) != "" || state.Selection != 0 {
		t.Fatalf("state=%#v", state)
	}
}

func TestModalDrawListContainsRetainedChromeAndSelection(t *testing.T) {
	state := modal.State{Mode: modal.ModeCommandPalette, Entries: []modal.Entry{{Label: "Copy"}}, Filtered: []int{0}}
	layout := modal.ListLayout(state, modal.LayoutGeometry{Columns: 20, Rows: 4, VisibleRows: 2})
	cmds := modalDrawList(layout, 20, 4, 400, 200, 8, 16, 1, color.RGBA{A: 220}, color.RGBA{R: 1, G: 2, B: 3, A: 255}, color.RGBA{A: 255})
	if len(cmds) < 5 || cmds[0].kind != cmdRect {
		t.Fatalf("cmds=%#v", cmds)
	}
	selected := false
	for _, cmd := range cmds {
		if cmd.kind == cmdRect && cmd.col.A == 48 {
			selected = true
		}
	}
	if !selected {
		t.Fatal("selected row background missing")
	}
}
