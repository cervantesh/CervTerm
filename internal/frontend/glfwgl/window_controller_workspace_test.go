//go:build glfw

package glfwgl

import (
	"reflect"
	"testing"

	termmux "cervterm/internal/mux"
)

func TestWindowControllerProjectsWorkspaceVisibilityAndFocus(t *testing.T) {
	a := newRunningMuxTestApp(t)
	second, _, err := a.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _, err := a.mux.CreateWorkspace("work")
	if err != nil {
		t.Fatal(err)
	}
	moveEvents, err := a.mux.MoveWindowToWorkspace(second.ID, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	one, two := &fakeNativeWindow{id: "one", log: &log}, &fakeNativeWindow{id: "two", log: &log}
	controller := newWindowController(processServices{mux: a.mux}, fakeNativePump{log: &log})
	if err := controller.attach(1, one, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.attach(2, two, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(moveEvents)
	if !controller.projectionVisible(1) || controller.projectionVisible(2) || controller.active != 1 {
		t.Fatalf("default visibility active=%d", controller.active)
	}
	log = nil
	switchEvents, err := a.mux.SwitchWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller.dispatch(switchEvents)
	want := []string{"hide:one", "show:two", "focus:two"}
	if !reflect.DeepEqual(log, want) || controller.projectionVisible(1) || !controller.projectionVisible(2) || controller.active != 2 {
		t.Fatalf("log=%v active=%d", log, controller.active)
	}
	controller.clearDamage(1)
	log = nil
	controller.dispatch([]termmux.Event{{Kind: termmux.PaneDirty, Pane: 1}})
	if !controller.windows[1].dirty || controller.active != 2 || len(log) != 0 {
		t.Fatalf("hidden damage dirty=%v active=%d log=%v", controller.windows[1].dirty, controller.active, log)
	}
	empty, _, err := a.mux.CreateWorkspace("empty")
	if err != nil {
		t.Fatal(err)
	}
	log = nil
	events, err := a.mux.SwitchWorkspace(empty.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller.dispatch(events)
	if controller.active != 0 || controller.projectionVisible(1) || controller.projectionVisible(2) || !reflect.DeepEqual(log, []string{"hide:one", "hide:two"}) {
		t.Fatalf("empty log=%v active=%d", log, controller.active)
	}
}

func TestInactiveWorkspaceCloseDoesNotRefocusActiveProjection(t *testing.T) {
	a := newRunningMuxTestApp(t)
	second, _, err := a.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _, err := a.mux.CreateWorkspace("work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.mux.MoveWindowToWorkspace(second.ID, workspace.ID); err != nil {
		t.Fatal(err)
	}
	switchEvents, err := a.mux.SwitchWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	controller := newWindowController(processServices{mux: a.mux}, fakeNativePump{log: &log})
	if err := controller.attach(1, &fakeNativeWindow{id: "one", log: &log}, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.attach(2, &fakeNativeWindow{id: "two", log: &log}, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(switchEvents)
	log = nil
	result, events, err := a.mux.CloseWindow(1)
	if err != nil {
		t.Fatal(err)
	}
	controller.dispatch(events)
	if result.ActiveChanged || result.WorkspaceChanged {
		t.Fatalf("result=%#v", result)
	}
	for _, entry := range log {
		if entry == "focus:two" {
			t.Fatalf("spurious focus log=%v", log)
		}
	}
}
