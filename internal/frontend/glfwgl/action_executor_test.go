//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	termaction "cervterm/internal/action"
	"cervterm/internal/core"
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

func TestActionExecutorTopologyActionsTargetOriginAndPreserveFailure(t *testing.T) {
	a := newRunningMuxTestApp(t)
	first := a.focusedPane
	if err := executeFocusedAction(a, termaction.SplitPane{Axis: termaction.SplitColumns}); err != nil {
		t.Fatal(err)
	}
	second := a.focusedPane
	context := termaction.Context{
		Source:  termaction.SourceScript,
		Origin:  termaction.Ref{Kind: termaction.RefPane, ID: uint64(first)},
		Focused: termaction.Ref{Kind: termaction.RefPane, ID: uint64(second)},
	}
	resize := termaction.Envelope{Action: termaction.ResizePane{Direction: termaction.FocusRight, Delta: 1}, Target: termaction.TargetOrigin}
	if err := a.executeAction(resize, context); err != nil {
		t.Fatal(err)
	}
	if focused, _ := a.mux.FocusedPane(); focused != first {
		t.Fatalf("resize focused pane = %d, want origin %d", focused, first)
	}
	if err := a.executeAction(termaction.Envelope{Action: termaction.SwapPane{Direction: termaction.FocusRight}, Target: termaction.TargetOrigin}, context); err != nil {
		t.Fatal(err)
	}
	if err := a.executeAction(termaction.Envelope{Action: termaction.MovePane{Direction: termaction.FocusRight}, Target: termaction.TargetFocused}, a.actionContext(termaction.SourceKeyboard)); err != nil {
		t.Fatal(err)
	}
	before := a.mux.PaneIDs()
	err := executeFocusedAction(a, termaction.SwapPane{Direction: termaction.FocusUp})
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorMux {
		t.Fatalf("topology failure = %v", err)
	}
	after := a.mux.PaneIDs()
	if len(before) != len(after) {
		t.Fatalf("topology changed after failure: before=%v after=%v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("topology identities changed after failure: before=%v after=%v", before, after)
		}
	}
}

func TestActionExecutorScrollsBetweenSemanticPrompts(t *testing.T) {
	a := newMuxTestApp(t, 20, 5)
	pane := a.focusedPane
	for index := 0; index < 4; index++ {
		feedTestPane(t, a, []byte("\x1b]133;A\x1b\\P\x1b]133;B\x1b\\cmd\x1b]133;C\x1b\\\r\nout\r\n\x1b]133;D;0\x1b\\"))
	}
	semantic, _ := a.mux.SemanticSnapshot(pane)
	expected := -1
	for index := len(semantic.Ranges) - 1; index >= 0; index-- {
		if semantic.Ranges[index].Kind == core.SemanticPrompt && semantic.Ranges[index].Start.GlobalRow < semantic.ViewportTopGlobalRow {
			expected = semantic.Ranges[index].Start.GlobalRow
			break
		}
	}
	if expected < 0 {
		t.Fatal("missing previous prompt setup")
	}
	if err := executeFocusedAction(a, termaction.ScrollToPrompt{Delta: -1}); err != nil {
		t.Fatal(err)
	}
	first, _ := a.mux.PaneView(pane)
	if first.DisplayOffset == 0 {
		t.Fatal("previous prompt did not scroll")
	}
	firstSemantic, _ := a.mux.SemanticSnapshot(pane)
	if firstSemantic.ViewportTopGlobalRow != expected {
		t.Fatalf("top=%d expected=%d", firstSemantic.ViewportTopGlobalRow, expected)
	}
	if err := executeFocusedAction(a, termaction.ScrollToPrompt{Delta: -1}); err != nil {
		t.Fatal(err)
	}
	second, _ := a.mux.PaneView(pane)
	if second.DisplayOffset <= first.DisplayOffset {
		t.Fatalf("offsets first=%d second=%d", first.DisplayOffset, second.DisplayOffset)
	}
	if err := executeFocusedAction(a, termaction.ScrollToPrompt{Delta: 1}); err != nil {
		t.Fatal(err)
	}
	third, _ := a.mux.PaneView(pane)
	if third.DisplayOffset >= second.DisplayOffset {
		t.Fatalf("next prompt offset=%d previous=%d", third.DisplayOffset, second.DisplayOffset)
	}
}

func TestActionExecutorPromptNavigationFailsWithoutMetadata(t *testing.T) {
	a := newMuxTestApp(t, 20, 5)
	err := executeFocusedAction(a, termaction.ScrollToPrompt{Delta: -1})
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorMux {
		t.Fatalf("error=%v", err)
	}
}
