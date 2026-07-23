//go:build glfw

package glfwgl

import (
	"errors"
	"testing"

	"cervterm/internal/ime"
)

func TestCandidateGeometryLifecycleClearsOnCompositionEnd(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	published, cleared := 0, 0
	if err := app.setCandidateGeometryCallbacks(func(nativeCandidateRect) error { published++; return nil }, func() error { cleared++; return nil }); err != nil {
		t.Fatal(err)
	}
	target := ime.Target{Kind: ime.TargetPane, ID: 1, Activation: 1}
	app.composition.bind(func() (ime.Target, error) { return target, nil }, func(ime.Target, string) error { return nil })
	_, err := app.composition.start()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.candidateGeometry.publishChanged(nativeCandidateRect{X: 1, Y: 2, Width: 1, Height: 10}); err != nil {
		t.Fatal(err)
	}
	app.needsRedraw = false
	if err := app.composition.cancel(ime.CancelExplicit); err != nil {
		t.Fatal(err)
	}
	if published != 1 || cleared != 1 || !app.needsRedraw || app.candidateGeometry.wasVisible {
		t.Fatalf("published=%d cleared=%d redraw=%v state=%#v", published, cleared, app.needsRedraw, app.candidateGeometry)
	}
	if err := app.composition.cancel(ime.CancelExplicit); err != nil || cleared != 1 {
		t.Fatalf("idempotent cancel err=%v cleared=%d", err, cleared)
	}
}

func TestCandidateGeometryClearFailureRetriesOnRequestedFrame(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	attempts := 0
	if err := app.setCandidateGeometryCallbacks(func(nativeCandidateRect) error { return nil }, func() error {
		attempts++
		if attempts == 1 {
			return errors.New("clear failed")
		}
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
	if err := app.composition.cancel(ime.CancelExplicit); err != nil || attempts != 1 || !app.candidateGeometry.wasVisible || !app.needsRedraw {
		t.Fatalf("cancel err=%v attempts=%d visible=%v redraw=%v", err, attempts, app.candidateGeometry.wasVisible, app.needsRedraw)
	}
	app.beginCandidateGeometryFrame()
	app.finishCandidateGeometryFrame()
	if attempts != 2 || app.candidateGeometry.wasVisible {
		t.Fatalf("retry attempts=%d visible=%v", attempts, app.candidateGeometry.wasVisible)
	}
}

func TestCandidateGeometryActiveCompositionWithoutPresenterClearsStaleRect(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	cleared := 0
	if err := app.setCandidateGeometryCallbacks(func(nativeCandidateRect) error { return nil }, func() error { cleared++; return nil }); err != nil {
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
	app.beginCandidateGeometryFrame()
	app.finishCandidateGeometryFrame()
	if cleared != 1 || app.candidateGeometry.wasVisible {
		t.Fatalf("cleared=%d state=%#v", cleared, app.candidateGeometry)
	}
}
