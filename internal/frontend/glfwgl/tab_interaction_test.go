//go:build glfw

package glfwgl

import (
	"cervterm/internal/config"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"strings"
	"testing"
)

func twoTabInteractionApp(t *testing.T) (*App, termmux.PaneID) {
	t.Helper()
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.ensureConfigState()
	_, pane, events, err := a.mux.SpawnTab(a.desiredShellSpawnSpec(), termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents(events)
	return a, pane
}
func TestCloseConfirmationRetainsTabAndRejectsRevisionChange(t *testing.T) {
	a, _ := twoTabInteractionApp(t)
	if _, err := a.mux.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	events, err := a.requestTabClose(2)
	if err != nil || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if a.modal.Mode() != modal.ModeTabCloseConfirmation || a.tabClose.Tab != 2 {
		t.Fatalf("mode=%v target=%#v", a.modal.Mode(), a.tabClose)
	}
	if _, err := a.mux.RenameTab(2, "changed"); err != nil {
		t.Fatal(err)
	}
	err = a.acceptTabClose(modal.Entry{ID: "close-tab"})
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("err=%v", err)
	}
	if len(a.mux.Tabs()) != 2 {
		t.Fatalf("tabs=%#v", a.mux.Tabs())
	}
}
func TestCloseConfirmationAcceptsUnchangedClickedTab(t *testing.T) {
	a, _ := twoTabInteractionApp(t)
	if _, err := a.mux.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	if _, err := a.requestTabClose(2); err != nil {
		t.Fatal(err)
	}
	if err := a.acceptTabClose(modal.Entry{ID: "close-tab"}); err != nil {
		t.Fatal(err)
	}
	if len(a.mux.Tabs()) != 1 || a.mux.Tabs()[0].ID != 1 {
		t.Fatalf("tabs=%#v", a.mux.Tabs())
	}
}
func TestTabSwitcherRetainsStableIDAcrossReorder(t *testing.T) {
	a, _ := twoTabInteractionApp(t)
	if err := a.openTabSwitcher(); err != nil {
		t.Fatal(err)
	}
	if a.modal.Mode() != modal.ModeTabSwitcher {
		t.Fatalf("mode=%v", a.modal.Mode())
	}
	entry := modal.Entry{ID: "tab:2", Label: "two"}
	if _, err := a.mux.MoveTab(2, 0); err != nil {
		t.Fatal(err)
	}
	if err := a.acceptCommandPalette(entry, a.focusedPane); err != nil {
		t.Fatal(err)
	}
	if a.mux.ActiveTab() != 2 {
		t.Fatalf("active=%d", a.mux.ActiveTab())
	}
}
func TestInactivePaneOutputSetsOneActivityBadgeUntilActivation(t *testing.T) {
	a, pane := twoTabInteractionApp(t)
	if _, err := a.mux.ActivateTab(1); err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents([]termmux.Event{{Kind: termmux.PaneOutput, Pane: pane, Data: []byte("x")}})
	if !a.tabActivity[2] {
		t.Fatal("activity not recorded")
	}
	a.handleMuxEvents([]termmux.Event{{Kind: termmux.TabActivated, Tab: 2, Pane: pane}})
	if a.tabActivity[2] {
		t.Fatal("activity not cleared")
	}
}

func TestCloseConfirmationInvalidatesEveryTabMutationPath(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*App) error
	}{
		{"rename", func(a *App) error { _, err := a.mux.RenameTab(2, "changed"); return err }},
		{"reorder", func(a *App) error { _, err := a.mux.MoveTab(2, 0); return err }},
		{"pane membership", func(a *App) error {
			target := a.mux.Tabs()[1].Focused
			_, err := a.mux.TransferPane(1, 2, target, termmux.SplitColumns)
			return err
		}},
		{"lifecycle", func(a *App) error { _, err := a.mux.CloseTab(2); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := twoTabInteractionApp(t)
			if _, err := a.mux.ActivateTab(1); err != nil {
				t.Fatal(err)
			}
			if _, err := a.requestTabClose(2); err != nil {
				t.Fatal(err)
			}
			if err := tc.mutate(a); err != nil {
				t.Fatal(err)
			}
			if err := a.acceptTabClose(modal.Entry{ID: "close-tab"}); err == nil {
				t.Fatal("stale confirmation accepted")
			}
		})
	}
}
