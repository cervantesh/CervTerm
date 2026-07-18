//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"testing"
)

func TestTypedTabActionsRetainStableTargetsAndOriginOwnership(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.ensureConfigState()
	ctx := a.actionContext(termaction.SourceKeyboard)
	if err := a.executeAction(actionEnvelope(termaction.NewTab{}), ctx); err != nil {
		t.Fatal(err)
	}
	tabs := a.mux.Tabs()
	if len(tabs) != 2 || a.mux.ActiveTab() != 2 {
		t.Fatalf("tabs=%#v active=%d", tabs, a.mux.ActiveTab())
	}
	originPane := tabs[1].Focused
	if err := a.executeAction(actionEnvelope(termaction.ActivateTab{TabID: 1}), ctx); err != nil {
		t.Fatal(err)
	}
	origin := termaction.Context{Source: termaction.SourceKeyboard, Origin: termaction.Ref{Kind: termaction.RefPane, ID: uint64(originPane)}, Focused: termaction.Ref{Kind: termaction.RefPane, ID: 1}}
	env := termaction.Envelope{Action: termaction.ActivateTabRelative{Delta: 1}, Target: termaction.TargetOrigin}
	if err := a.executeAction(env, origin); err != nil {
		t.Fatal(err)
	}
	if a.mux.ActiveTab() != 1 {
		t.Fatalf("relative action used active focus instead of origin owner: %d", a.mux.ActiveTab())
	}
	if err := a.executeAction(actionEnvelope(termaction.RenameTab{TabID: 2, Title: "stable"}), ctx); err != nil {
		t.Fatal(err)
	}
	if a.mux.Tabs()[1].Title != "stable" {
		t.Fatalf("tabs=%#v", a.mux.Tabs())
	}
	if err := a.executeAction(actionEnvelope(termaction.CloseTab{TabID: 2}), ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.mux.Tabs()) != 1 {
		t.Fatalf("tabs=%#v", a.mux.Tabs())
	}
}

func TestMovePaneToTabActionUsesExplicitDestination(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg = config.Defaults()
	a.ensureConfigState()
	_, _, events, err := a.mux.SpawnTab(a.desiredShellSpawnSpec(), termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents(events)
	_, err = a.mux.ActivateTab(1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := termaction.Context{Source: termaction.SourceKeyboard, Origin: termaction.Ref{Kind: termaction.RefPane, ID: 1}, Focused: termaction.Ref{Kind: termaction.RefPane, ID: 1}}
	env := termaction.Envelope{Action: termaction.MovePaneToTab{TabID: 2, Axis: termaction.SplitColumns}, Target: termaction.TargetOrigin}
	if err := a.executeAction(env, ctx); err != nil {
		t.Fatal(err)
	}
	if owner, ok := a.mux.TabForPane(1); !ok || owner != 2 {
		t.Fatalf("owner=%d ok=%v", owner, ok)
	}
}
