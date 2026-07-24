//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func windowEnvelope(action termaction.Action) termaction.Envelope {
	return termaction.Envelope{Action: action, Target: termaction.TargetFocused}
}

func muxWindowPaneCount(m *termmux.Mux) int {
	total := 0
	for _, window := range m.Windows() {
		for _, tab := range window.Tabs {
			total += len(tab.Panes)
		}
	}
	return total
}

func TestWindowActionExecutorCreateFocusCloseAndRejectStale(t *testing.T) {
	a := newMuxTestApp(t, 80, 24)
	var log []string
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	a.controller = controller
	a.windowID = 1
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, a, a.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	child := &App{controller: controller, windowID: 2}
	factory := &fakeCandidateFactory{log: &log, host: &fakeNativeWindow{id: "two", log: &log}, app: child}
	runtime := &fakeRuntimeWindows{log: &log}
	controller.setCandidateFactory(factory)
	controller.setRuntimeWindows(runtime)
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	ctx := a.actionContext(termaction.SourceKeyboard)
	if err := a.executeAction(windowEnvelope(termaction.NewWindow{}), ctx); err != nil {
		t.Fatal(err)
	}
	if controller.projectionApp(2) == nil {
		t.Fatal("new window not published")
	}
	if err := a.executeAction(windowEnvelope(termaction.FocusWindow{WindowID: 2}), ctx); err != nil {
		t.Fatal(err)
	}
	if controller.active != 2 {
		t.Fatalf("active=%d", controller.active)
	}
	multiple, err := termaction.NewMultiple(
		windowEnvelope(termaction.FocusWindow{WindowID: 1}),
		windowEnvelope(termaction.ToggleStats{}),
		windowEnvelope(termaction.FocusWindow{WindowID: 2}),
		windowEnvelope(termaction.ToggleStats{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.executeAction(windowEnvelope(multiple), ctx); err != nil {
		t.Fatal(err)
	}
	if !a.showStats || !child.showStats || controller.active != 2 {
		t.Fatalf("per-child projection retarget root=%v child=%v active=%d", a.showStats, child.showStats, controller.active)
	}
	if err := a.executeAction(windowEnvelope(termaction.CloseWindow{WindowID: 2}), ctx); err != nil {
		t.Fatal(err)
	}
	if controller.projectionApp(2) != nil {
		t.Fatal("window not closed")
	}
	err = a.executeAction(windowEnvelope(termaction.FocusWindow{WindowID: 99}), ctx)
	if !errors.Is(err, termaction.ErrTargetUnavailable) {
		t.Fatalf("stale err=%v", err)
	}
}

func TestCrossWindowActionsUseStableOriginAndPreserveSessions(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.windowID = 1
	a.lastFBW, a.lastFBH = 800, 480
	view, events, err := a.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	controller := newWindowController(processServices{mux: a.mux}, fakeNativePump{log: &log})
	a.controller = controller
	child := &App{mux: a.mux, controller: controller, windowID: view.ID, lastFBW: 800, lastFBH: 480, cellW: 8, cellH: 16, paneUI: map[termmux.PaneID]*paneUIState{}, pendingPaneScroll: map[termmux.PaneID]int{}, pendingPaneResize: map[termmux.PaneID]termmux.PaneGeometry{}}
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, a, a.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(view.ID, &fakeNativeWindow{id: "two", log: &log}, child, child.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(events)
	before := muxWindowPaneCount(a.mux)
	ctx := termaction.Context{Source: termaction.SourceScript, Origin: termaction.Ref{Kind: termaction.RefPane, ID: 1}, Focused: termaction.Ref{Kind: termaction.RefPane, ID: 1}, OriginWindow: termaction.Ref{Kind: termaction.RefWindow, ID: 1}, FocusedWindow: termaction.Ref{Kind: termaction.RefWindow, ID: 1}}
	a.initCompositionCoordinator()
	if _, err := a.composition.start(); err != nil {
		t.Fatal(err)
	}
	err = a.executeAction(windowEnvelope(termaction.MoveTabToWindow{WindowID: uint64(view.ID), TabID: 1, Position: 1}), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot := a.composition.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelTargetChanged {
		t.Fatalf("source tab composition=%#v", snapshot)
	}
	after := muxWindowPaneCount(a.mux)
	if before != after {
		t.Fatalf("pane count %d -> %d", before, after)
	}
	if owner, ok := a.mux.WindowForPane(1); !ok || owner != view.ID {
		t.Fatalf("owner=%d ok=%v", owner, ok)
	}
	stable := a.mux.Windows()
	err = a.executeAction(windowEnvelope(termaction.MoveTabToWindow{WindowID: 999, TabID: 1, Position: 0}), ctx)
	if !errors.Is(err, termaction.ErrTargetUnavailable) || !reflect.DeepEqual(stable, a.mux.Windows()) {
		t.Fatalf("stale destination err=%v", err)
	}
	err = a.executeAction(windowEnvelope(termaction.MoveTabToWindow{WindowID: uint64(view.ID), TabID: 999, Position: 0}), ctx)
	if err == nil || !reflect.DeepEqual(stable, a.mux.Windows()) {
		t.Fatalf("stale tab err=%v", err)
	}
}

func TestMovePaneToWindowUsesPerPaneMetricsAndStaleSourceIsAtomic(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.windowID, a.lastFBW, a.lastFBH = 1, 800, 480
	view, createEvents, err := a.mux.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	activateEvents, err := a.mux.ActivateWindow(1)
	if err != nil {
		t.Fatal(err)
	}
	pane, splitEvents, err := a.mux.SpawnSplit(1, termmux.SplitColumns, termmux.SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	controller := newWindowController(processServices{mux: a.mux}, fakeNativePump{log: &log})
	a.controller = controller
	child := &App{mux: a.mux, controller: controller, windowID: view.ID, lastFBW: 800, lastFBH: 480, cellW: 8, cellH: 16, paneUI: map[termmux.PaneID]*paneUIState{}, pendingPaneScroll: map[termmux.PaneID]int{}, pendingPaneResize: map[termmux.PaneID]termmux.PaneGeometry{}}
	if err := controller.attachApp(1, &fakeNativeWindow{id: "one", log: &log}, a, a.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(view.ID, &fakeNativeWindow{id: "two", log: &log}, child, child.applyMuxEvents); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	controller.dispatch(append(append(createEvents, activateEvents...), splitEvents...))
	a.ensurePaneUI(pane).font.cellW, a.ensurePaneUI(pane).font.cellH = 10, 20
	child.ensurePaneUI(view.Tabs[0].Focused).font.cellW, child.ensurePaneUI(view.Tabs[0].Focused).font.cellH = 8, 16
	_, _, resolve, err := a.transferGeometry(1, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := resolve(pane); !ok || got.CellWidth != 10 || got.CellHeight != 20 {
		t.Fatalf("source metrics=%#v ok=%v", got, ok)
	}
	if got, ok := resolve(view.Tabs[0].Focused); !ok || got.CellWidth != 8 || got.CellHeight != 16 {
		t.Fatalf("destination metrics=%#v ok=%v", got, ok)
	}
	ctx := termaction.Context{Source: termaction.SourceScript, Origin: termaction.Ref{Kind: termaction.RefPane, ID: uint64(pane)}, Focused: termaction.Ref{Kind: termaction.RefPane, ID: uint64(pane)}, OriginWindow: termaction.Ref{Kind: termaction.RefWindow, ID: 1}, FocusedWindow: termaction.Ref{Kind: termaction.RefWindow, ID: 1}}
	beforeCount := muxWindowPaneCount(a.mux)
	a.initCompositionCoordinator()
	if _, err := a.composition.start(); err != nil {
		t.Fatal(err)
	}
	if err := a.executeAction(windowEnvelope(termaction.MovePaneToWindow{WindowID: uint64(view.ID), PaneID: uint64(pane), Axis: termaction.SplitRows}), ctx); err != nil {
		t.Fatal(err)
	}
	if snapshot := a.composition.snapshot(); snapshot.Active || snapshot.LastCancel != ime.CancelTargetChanged {
		t.Fatalf("source pane composition=%#v", snapshot)
	}
	if owner, ok := a.mux.WindowForPane(pane); !ok || owner != view.ID || muxWindowPaneCount(a.mux) != beforeCount {
		t.Fatalf("owner=%d ok=%v count=%d", owner, ok, muxWindowPaneCount(a.mux))
	}
	if _, err := a.composition.start(); err != nil {
		t.Fatal(err)
	}
	before := a.mux.Windows()
	err = a.executeAction(windowEnvelope(termaction.MovePaneToWindow{WindowID: uint64(view.ID), PaneID: 999, Axis: termaction.SplitRows}), ctx)
	if err == nil || !reflect.DeepEqual(before, a.mux.Windows()) {
		t.Fatalf("stale err=%v before=%#v after=%#v", err, before, a.mux.Windows())
	}
	if snapshot := a.composition.snapshot(); !snapshot.Active {
		t.Fatalf("failed transfer cancelled composition=%#v", snapshot)
	}
}
