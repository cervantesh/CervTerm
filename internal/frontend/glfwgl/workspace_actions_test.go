//go:build glfw

package glfwgl

import (
	"errors"
	"strings"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
)

func TestWorkspaceActionExecutorCRUDMoveAndStaleTargets(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.windowID = 1
	second, createEvents, err := a.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	controller := newWindowController(processServices{mux: a.mux}, fakeNativePump{log: &log})
	a.controller = controller
	child := &App{mux: a.mux, controller: controller, windowID: second.ID, paneUI: map[termmux.PaneID]*paneUIState{}, pendingPaneScroll: map[termmux.PaneID]int{}, pendingPaneResize: map[termmux.PaneID]termmux.PaneGeometry{}}
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, a, a.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(second.ID, &fakeNativeWindow{id: "two", log: &log}, child, child.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(createEvents)
	context := a.actionContext(termaction.SourceKeyboard)
	if err := a.executeAction(windowEnvelope(termaction.CreateWorkspace{Name: "build"}), context); err != nil {
		t.Fatal(err)
	}
	workspaces := a.mux.Workspaces()
	if len(workspaces) != 2 {
		t.Fatalf("workspaces=%#v", workspaces)
	}
	workspace := workspaces[1].ID
	if err := a.executeAction(windowEnvelope(termaction.RenameWorkspace{WorkspaceID: uint64(workspace), Name: "ops"}), context); err != nil {
		t.Fatal(err)
	}
	if err := a.executeAction(windowEnvelope(termaction.MoveWindowToWorkspace{WindowID: uint64(second.ID), WorkspaceID: uint64(workspace)}), context); err != nil {
		t.Fatal(err)
	}
	if controller.projectionVisible(second.ID) || !controller.projectionVisible(1) {
		t.Fatalf("visibility one=%v two=%v", controller.projectionVisible(1), controller.projectionVisible(second.ID))
	}
	if err := a.executeAction(windowEnvelope(termaction.SwitchWorkspace{WorkspaceID: uint64(workspace)}), context); err != nil {
		t.Fatal(err)
	}
	if controller.projectionVisible(1) || !controller.projectionVisible(second.ID) || a.mux.ActiveWorkspace().Name != "ops" {
		t.Fatalf("active=%#v", a.mux.ActiveWorkspace())
	}
	before := a.mux.Workspaces()
	for _, command := range []termaction.Action{
		termaction.SwitchWorkspace{WorkspaceID: 999},
		termaction.RenameWorkspace{WorkspaceID: 999, Name: "stale"},
		termaction.MoveWindowToWorkspace{WindowID: 999, WorkspaceID: uint64(workspace)},
		termaction.MoveWindowToWorkspace{WindowID: uint64(second.ID), WorkspaceID: 999},
	} {
		if err := a.executeAction(windowEnvelope(command), context); err == nil {
			t.Fatalf("stale action accepted: %#v", command)
		}
		if got := a.mux.Workspaces(); !workspaceViewsEqual(got, before) {
			t.Fatalf("state changed after %#v: %#v", command, got)
		}
	}
}

func TestWorkspaceSwitcherRetainsIdentityAcrossReload(t *testing.T) {
	a := newRunningMuxTestApp(t)
	workspace, _, err := a.mux.CreateWorkspace("build")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.executeAction(windowEnvelope(termaction.ActivateWorkspaceSwitcher{}), a.actionContext(termaction.SourceKeyboard)); err != nil {
		t.Fatal(err)
	}
	for _, r := range "build" {
		a.modal.AppendRune(r)
	}
	a.scriptGeneration++
	a.applyModalIntents(a.modal.Accept())
	if a.modal.Active() || a.mux.ActiveWorkspace().ID != workspace.ID {
		t.Fatalf("modal=%#v active=%#v", a.modal.Snapshot(), a.mux.ActiveWorkspace())
	}
}

func TestWorkspaceSwitcherRejectsRevisionDriftAndPreservesModal(t *testing.T) {
	a := newRunningMuxTestApp(t)
	workspace, _, err := a.mux.CreateWorkspace("build")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.openWorkspaceSwitcher(); err != nil {
		t.Fatal(err)
	}
	for _, r := range "build" {
		a.modal.AppendRune(r)
	}
	if _, err := a.mux.RenameWorkspace(workspace.ID, "renamed"); err != nil {
		t.Fatal(err)
	}
	a.applyModalIntents(a.modal.Accept())
	state := a.modal.Snapshot()
	if !a.modal.Active() || state.Mode != modal.ModeWorkspaceSwitcher || !strings.Contains(state.Error, "changed while switcher was open") || a.mux.ActiveWorkspace().ID != 1 {
		t.Fatalf("modal=%#v active=%#v", state, a.mux.ActiveWorkspace())
	}
}

func TestWorkspaceSwitcherRejectsRemovedIdentity(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.workspaceSwitcher = map[string]workspaceSwitcherActivation{"gone": {workspace: 999, revision: 1}}
	if err := a.acceptWorkspaceSwitcher(modal.Entry{ID: "missing"}); err == nil {
		t.Fatal("missing activation accepted")
	}
	if err := a.acceptWorkspaceSwitcher(modal.Entry{ID: "gone"}); !errors.Is(err, termmux.ErrWorkspaceNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func workspaceViewsEqual(a, b []termmux.WorkspaceView) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name || a[i].Active != b[i].Active || a[i].Focused != b[i].Focused || a[i].Revision != b[i].Revision || len(a[i].Windows) != len(b[i].Windows) {
			return false
		}
		for j := range a[i].Windows {
			if a[i].Windows[j] != b[i].Windows[j] {
				return false
			}
		}
	}
	return true
}

func TestWorkspaceSwitcherIsCommandPaletteDiscoverable(t *testing.T) {
	a := newRunningMuxTestApp(t)
	entries, activations := a.commandPaletteSnapshot(true)
	for _, entry := range entries {
		if entry.ID != "action:"+string(termaction.IDActivateWorkspaceSwitcher) {
			continue
		}
		activation, ok := activations[entry.ID]
		if !ok {
			t.Fatalf("missing activation for %#v", entry)
		}
		if _, ok := activation.envelope.Action.(termaction.ActivateWorkspaceSwitcher); !ok {
			t.Fatalf("action=%T", activation.envelope.Action)
		}
		return
	}
	t.Fatal("workspace switcher missing from command palette")
}
