//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/ime"
)

func TestDrawFinishesCandidateGeometryFrameOnPanic(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	cleared := 0
	if err := app.setCandidateGeometryCallbacks(func(nativeCandidateRect) error { return nil }, func() error {
		cleared++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	app.composition.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
	if _, err := app.composition.start(); err != nil {
		t.Fatal(err)
	}
	if err := app.candidateGeometry.publishChanged(nativeCandidateRect{Width: 1, Height: 1}); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("draw without a native window did not panic")
		}
		if cleared != 1 || app.candidateGeometry.wasVisible {
			t.Fatalf("panic cleanup: cleared=%d state=%#v", cleared, app.candidateGeometry)
		}
		if app.renderFlow == nil {
			t.Fatal("draw did not route through the App render controller")
		}
	}()
	app.draw()
}
