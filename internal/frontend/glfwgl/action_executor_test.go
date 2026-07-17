//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

func executeFocusedAction(a *App, command termaction.Action) error {
	return a.executeAction(actionEnvelope(command), a.actionContext(termaction.SourceKeyboard))
}

func TestActionExecutorSplitFocusAndCloseSequence(t *testing.T) {
	a := newRunningMuxTestApp(t)
	first := a.focusedPane
	if err := executeFocusedAction(a, termaction.SplitPane{Axis: termaction.SplitColumns}); err != nil {
		t.Fatal(err)
	}
	if len(a.mux.PaneIDs()) != 2 || a.focusedPane == first {
		t.Fatalf("split panes=%v focus=%d", a.mux.PaneIDs(), a.focusedPane)
	}
	second := a.focusedPane
	multiple, err := termaction.NewMultiple(
		actionEnvelope(termaction.FocusPane{Direction: termaction.FocusLeft}),
		actionEnvelope(termaction.ClosePane{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := executeFocusedAction(a, multiple); err != nil {
		t.Fatal(err)
	}
	ids := a.mux.PaneIDs()
	if len(ids) != 1 || ids[0] != second || a.focusedPane != second {
		t.Fatalf("sequence panes=%v focus=%d, want remaining second=%d", ids, a.focusedPane, second)
	}
}

func TestActionExecutorResolvesOriginPerPane(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg.Font.Size = 14
	a.zoom.base = 14
	first := a.focusedPane
	a.ensurePaneUI(first).font.fontSize = 14
	if err := executeFocusedAction(a, termaction.SplitPane{Axis: termaction.SplitColumns}); err != nil {
		t.Fatal(err)
	}
	second := a.focusedPane
	a.ensurePaneUI(second).font.fontSize = 14
	context := termaction.Context{
		Source:  termaction.SourceScript,
		Origin:  termaction.Ref{Kind: termaction.RefPane, ID: uint64(first)},
		Focused: termaction.Ref{Kind: termaction.RefPane, ID: uint64(second)},
	}
	envelope := termaction.Envelope{
		Action: termaction.Zoom{Mode: termaction.ZoomDelta, Amount: 2},
		Target: termaction.TargetOrigin,
	}
	if err := a.executeAction(envelope, context); err != nil {
		t.Fatal(err)
	}
	if state := a.ensurePaneUI(first).font; !state.pending || state.pendingTarget != 16 {
		t.Fatalf("origin zoom state = %#v", state)
	}
	if state := a.ensurePaneUI(second).font; state.pending {
		t.Fatalf("focused sibling received origin zoom: %#v", state)
	}
}

func TestActionExecutorReportsUnavailableTarget(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	context := termaction.Context{
		Source:  termaction.SourceScript,
		Origin:  termaction.Ref{Kind: termaction.RefPane, ID: 999},
		Focused: a.focusedActionRef(),
	}
	err := a.executeAction(termaction.Envelope{Action: termaction.ClosePane{}, Target: termaction.TargetOrigin}, context)
	if !errors.Is(err, termaction.ErrTargetUnavailable) {
		t.Fatalf("error = %v, want ErrTargetUnavailable", err)
	}
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorTarget {
		t.Fatalf("execution error = %#v", execution)
	}
}

func TestActionExecutorGlobalAndDeferredActions(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	a.search.redraw = func() { a.needsRedraw = true }
	if err := executeFocusedAction(a, termaction.ToggleStats{}); err != nil || !a.showStats {
		t.Fatalf("toggle stats: shown=%v err=%v", a.showStats, err)
	}
	if err := executeFocusedAction(a, termaction.ToggleSearch{}); err != nil || !a.search.active {
		t.Fatalf("toggle search open: active=%v err=%v", a.search.active, err)
	}
	if err := executeFocusedAction(a, termaction.ToggleSearch{}); err != nil || a.search.active {
		t.Fatalf("toggle search close: active=%v err=%v", a.search.active, err)
	}
	err := executeFocusedAction(a, termaction.ReloadConfig{})
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorAction {
		t.Fatalf("reload error = %v", err)
	}
	err = executeFocusedAction(a, termaction.Callback{BindingIndex: 0})
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorScript {
		t.Fatalf("callback error = %v", err)
	}
}

func TestActionExecutorScrollsTargetPane(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	pane := a.focusedPane
	for range 100 {
		feedTestPane(t, a, []byte("\n"))
	}
	before, _ := a.mux.PaneView(pane)
	if before.ScrollbackLines == 0 {
		t.Fatal("expected scrollback")
	}
	if err := executeFocusedAction(a, termaction.Scroll{Unit: termaction.ScrollPage, Amount: 1}); err != nil {
		t.Fatal(err)
	}
	after, _ := a.mux.PaneView(pane)
	if after.DisplayOffset <= 0 {
		t.Fatalf("display offset = %d", after.DisplayOffset)
	}
	if _, ok := a.pendingPaneScroll[termmux.PaneID(pane)]; !ok {
		t.Fatal("scroll event was not recorded")
	}
}

func TestActionExecutorSequenceStopsOnFirstError(t *testing.T) {
	a := newMuxTestApp(t, 20, 10)
	multiple, err := termaction.NewMultiple(
		termaction.Envelope{Action: termaction.ClosePane{}, Target: termaction.TargetOrigin},
		actionEnvelope(termaction.ToggleStats{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	context := a.actionContext(termaction.SourceScript)
	context.Origin = termaction.Ref{Kind: termaction.RefPane, ID: 999}
	err = a.executeAction(actionEnvelope(multiple), context)
	if !errors.Is(err, termaction.ErrTargetUnavailable) {
		t.Fatalf("sequence error = %v", err)
	}
	if a.showStats {
		t.Fatal("sequence continued after first error")
	}
}
