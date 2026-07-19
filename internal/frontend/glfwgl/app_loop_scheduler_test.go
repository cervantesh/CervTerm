//go:build glfw

package glfwgl

import (
	"fmt"
	"reflect"
	"testing"

	termmux "cervterm/internal/mux"
)

type fakeProjectionCycleScheduler struct {
	ids    []termmux.WindowID
	closed map[termmux.WindowID]bool
	log    []string
}

func (s *fakeProjectionCycleScheduler) projectionIDs() []termmux.WindowID {
	return append([]termmux.WindowID(nil), s.ids...)
}

func (s *fakeProjectionCycleScheduler) shouldClose(id termmux.WindowID) bool { return s.closed[id] }

func (s *fakeProjectionCycleScheduler) closeRuntimeProjection(id termmux.WindowID) (termmux.CloseWindowResult, error) {
	s.log = append(s.log, "close:"+fmt.Sprint(id))
	return termmux.CloseWindowResult{Closed: true, Empty: len(s.ids) == 1}, nil
}

func TestRunProjectionCycleClosesExactWindowAndFramesSurvivingSiblings(t *testing.T) {
	scheduler := &fakeProjectionCycleScheduler{
		ids:    []termmux.WindowID{1, 2, 3},
		closed: map[termmux.WindowID]bool{2: true},
	}
	err := runProjectionCycle(scheduler, func(id termmux.WindowID) error {
		scheduler.log = append(scheduler.log, "frame:"+fmt.Sprint(id))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"frame:1", "close:2", "frame:3"}
	if !reflect.DeepEqual(scheduler.log, want) {
		t.Fatalf("log=%v want=%v", scheduler.log, want)
	}
}

func TestProductionCandidateFactoryIsInstalledOnLiveController(t *testing.T) {
	app := &App{}
	app.controller = newWindowController(processServices{}, fakeNativePump{log: &[]string{}})
	app.syncProcessServices()
	factory, ok := app.controller.candidateFactory.(*glfwProjectionFactory)
	if !ok || factory.owner != app {
		t.Fatalf("candidate factory=%T owner=%p want=%p", app.controller.candidateFactory, factory.owner, app)
	}
}

func TestInitialProjectionBundleTransfersResourceOwnershipOnce(t *testing.T) {
	var log []string
	host := &fakeNativeWindow{id: "initial", log: &log}
	app := &App{}
	controller := newWindowController(processServices{}, fakeNativePump{log: &log})
	if err := controller.attachApp(initialWindowID, host, app, func([]termmux.Event) bool { return false }); err != nil {
		t.Fatal(err)
	}
	closed := 0
	bundle := &nativeProjectionBundle{
		host:      host,
		app:       app,
		handle:    func([]termmux.Event) bool { return false },
		resources: []projectionResource{projectionResourceFunc(func() error { closed++; return nil })},
	}
	if err := controller.adoptProjectionBundle(initialWindowID, bundle); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	if err := controller.closeProjection(initialWindowID); err != nil {
		t.Fatal(err)
	}
	if err := controller.closeProjection(initialWindowID); err != nil {
		t.Fatal(err)
	}
	if closed != 1 || host.destroyed != 1 {
		t.Fatalf("resource closes=%d host destroys=%d", closed, host.destroyed)
	}
}
