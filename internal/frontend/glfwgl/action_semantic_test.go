//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	termaction "cervterm/internal/action"
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
