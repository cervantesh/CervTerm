//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	termaction "cervterm/internal/action"
	termsel "cervterm/internal/selection"
)

func TestCopySemanticZoneCopiesCurrentCycleInputAndOutput(t *testing.T) {
	a := newMuxTestApp(t, 20, 5)
	copied := ""
	a.clipboardSetter = func(text string) { copied = text }
	feedTestPane(t, a, []byte("\x1b]133;A\x1b\\P> \x1b]133;B\x1b\\echo hi\x1b]133;C\x1b\\\r\nout\r\nmore\x1b]133;D;0\x1b\\"))
	if err := executeFocusedAction(a, termaction.CopySemanticZone{Zone: termaction.SemanticZoneInput}); err != nil || copied != "echo hi" {
		t.Fatalf("input=%q err=%v", copied, err)
	}
	if err := executeFocusedAction(a, termaction.CopySemanticZone{Zone: termaction.SemanticZoneOutput}); err != nil || copied != "out\nmore" {
		t.Fatalf("output=%q err=%v", copied, err)
	}
}

func TestCopySemanticZoneFailsWithoutCurrentCycle(t *testing.T) {
	a := newMuxTestApp(t, 20, 5)
	called := false
	a.clipboardSetter = func(string) { called = true }
	err := executeFocusedAction(a, termaction.CopySemanticZone{Zone: termaction.SemanticZoneOutput})
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != termaction.ErrorMux || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestSelectSemanticZoneProjectsCurrentInputIntoViewport(t *testing.T) {
	a := newMuxTestApp(t, 20, 5)
	feedTestPane(t, a, []byte("\x1b]133;A\x1b\\P> \x1b]133;B\x1b\\echo hi\x1b]133;C\x1b\\\r\nout"))
	if err := executeFocusedAction(a, termaction.SelectSemanticZone{Zone: termaction.SemanticZoneInput}); err != nil {
		t.Fatal(err)
	}
	if got := a.Selection(); got != "echo hi" {
		t.Fatalf("selection=%q state=%#v", got, a.selection)
	}
}

func TestSelectSemanticZoneCrossViewportFailsWithoutPartialMutation(t *testing.T) {
	a := newMuxTestApp(t, 20, 3)
	feedTestPane(t, a, []byte("\x1b]133;A\x1b\\P\x1b]133;B\x1b\\cmd\x1b]133;C\x1b\\\r\n1\r\n2\r\n3\r\n4\r\n5"))
	before := selectionState{active: true, start: termsel.Point{Row: 0, Col: 0}, end: termsel.Point{Row: 0, Col: 0}}
	a.selection = before
	a.ensurePaneUI(a.focusedPane).selection = before
	viewBefore, _ := a.mux.PaneView(a.focusedPane)
	err := executeFocusedAction(a, termaction.SelectSemanticZone{Zone: termaction.SemanticZoneOutput})
	if err == nil || a.selection != before {
		t.Fatalf("err=%v selection=%#v", err, a.selection)
	}
	viewAfter, _ := a.mux.PaneView(a.focusedPane)
	if viewAfter.DisplayOffset != viewBefore.DisplayOffset {
		t.Fatalf("viewport mutated %d -> %d", viewBefore.DisplayOffset, viewAfter.DisplayOffset)
	}
}

func TestSelectSemanticZoneCopyMatchesSoftWrappedSemanticText(t *testing.T) {
	a := newMuxTestApp(t, 4, 3)
	feedTestPane(t, a, []byte("\x1b]133;A\x1b\\P\x1b]133;B\x1b\\abcdef\x1b]133;C\x1b\\"))
	if err := executeFocusedAction(a, termaction.SelectSemanticZone{Zone: termaction.SemanticZoneInput}); err != nil {
		t.Fatal(err)
	}
	if got := a.Selection(); got != "abcdef" {
		t.Fatalf("selection=%q state=%#v", got, a.selection)
	}
}
