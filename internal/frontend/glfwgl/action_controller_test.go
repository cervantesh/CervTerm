//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

type fakeActionProjectionPort struct {
	routes []actionExecutionRoute
	calls  int
}

func (f *fakeActionProjectionPort) activeActionRoute() actionExecutionRoute {
	var route actionExecutionRoute
	if f.calls < len(f.routes) {
		route = f.routes[f.calls]
	}
	f.calls++
	return route
}

type fakeActionPanePort struct {
	panes []termmux.PaneID
	calls []termmux.PaneID
}

func (f *fakeActionPanePort) actionPaneExists(pane termmux.PaneID) bool {
	f.calls = append(f.calls, pane)
	for _, candidate := range f.panes {
		if candidate == pane {
			return true
		}
	}
	return false
}

type fakeActionContextPort struct {
	focused []uint64
	calls   int
}

func (f *fakeActionContextPort) refreshFocusedActionContext(context termaction.Context) termaction.Context {
	if f.calls < len(f.focused) {
		context.Focused = termaction.Ref{Kind: termaction.RefPane, ID: f.focused[f.calls]}
	}
	f.calls++
	return context
}

type actionCommandCall struct {
	envelope termaction.Envelope
	context  termaction.Context
	pane     termmux.PaneID
}

type fakeActionCommandPort struct {
	calls []actionCommandCall
	errAt int
	err   error
}

func (f *fakeActionCommandPort) executeActionCommand(envelope termaction.Envelope, context termaction.Context, pane termmux.PaneID) error {
	f.calls = append(f.calls, actionCommandCall{envelope: envelope, context: context, pane: pane})
	if f.err != nil && len(f.calls) == f.errAt {
		return f.err
	}
	return nil
}

type fakeActionExecutionRoute struct {
	name      string
	focusedID uint64
	log       *[]string
	err       error
}

func (f *fakeActionExecutionRoute) refreshFocusedActionContext(context termaction.Context) termaction.Context {
	*f.log = append(*f.log, "refresh-"+f.name)
	context.Focused = termaction.Ref{Kind: termaction.RefPane, ID: f.focusedID}
	return context
}

func (f *fakeActionExecutionRoute) executeAction(_ termaction.Envelope, context termaction.Context) error {
	*f.log = append(*f.log, "execute-"+f.name)
	if context.Focused.ID != f.focusedID {
		*f.log = append(*f.log, "stale-"+f.name)
	}
	return f.err
}

func validActionContext() termaction.Context {
	return termaction.Context{
		Source:  termaction.SourceKeyboard,
		Origin:  termaction.Ref{Kind: termaction.RefPane, ID: 7},
		Focused: termaction.Ref{Kind: termaction.RefPane, ID: 8},
	}
}

func newFakeActionController() (*actionController, *fakeActionProjectionPort, *fakeActionPanePort, *fakeActionContextPort, *fakeActionCommandPort) {
	projections := &fakeActionProjectionPort{}
	panes := &fakeActionPanePort{}
	contexts := &fakeActionContextPort{}
	commands := &fakeActionCommandPort{}
	return newActionController(projections, panes, contexts, commands), projections, panes, contexts, commands
}

func assertActionErrorClass(t *testing.T, err error, class termaction.ErrorClass) {
	t.Helper()
	var execution *termaction.ExecutionError
	if !errors.As(err, &execution) || execution.Class != class {
		t.Fatalf("error = %v, want execution class %q", err, class)
	}
}

func TestActionControllerRejectsInvalidEnvelopeAndContextBeforePorts(t *testing.T) {
	t.Run("envelope", func(t *testing.T) {
		controller, projections, panes, contexts, commands := newFakeActionController()
		err := controller.executeAction(termaction.Envelope{}, validActionContext())
		assertActionErrorClass(t, err, termaction.ErrorAction)
		if projections.calls != 0 || len(panes.calls) != 0 || contexts.calls != 0 || len(commands.calls) != 0 {
			t.Fatalf("invalid envelope reached ports: projections=%d panes=%v contexts=%d commands=%d", projections.calls, panes.calls, contexts.calls, len(commands.calls))
		}
	})

	t.Run("context", func(t *testing.T) {
		controller, projections, panes, contexts, commands := newFakeActionController()
		envelope := termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused}
		err := controller.executeAction(envelope, termaction.Context{})
		assertActionErrorClass(t, err, termaction.ErrorAction)
		if projections.calls != 0 || len(panes.calls) != 0 || contexts.calls != 0 || len(commands.calls) != 0 {
			t.Fatalf("invalid context reached ports: projections=%d panes=%v contexts=%d commands=%d", projections.calls, panes.calls, contexts.calls, len(commands.calls))
		}
	})
}

func TestActionControllerResolvesPaneTargetsAndDelegatesTypedCommand(t *testing.T) {
	tests := []struct {
		name   string
		target termaction.TargetSelector
		want   termmux.PaneID
	}{
		{name: "origin", target: termaction.TargetOrigin, want: 7},
		{name: "focused", target: termaction.TargetFocused, want: 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller, _, panes, _, commands := newFakeActionController()
			panes.panes = []termmux.PaneID{7, 8}
			envelope := termaction.Envelope{Action: termaction.CopySelection{}, Target: test.target}
			context := validActionContext()
			if err := controller.executeAction(envelope, context); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(panes.calls, []termmux.PaneID{test.want}) {
				t.Fatalf("pane checks = %v, want %d", panes.calls, test.want)
			}
			if len(commands.calls) != 1 || commands.calls[0].pane != test.want || commands.calls[0].envelope != envelope || commands.calls[0].context != context {
				t.Fatalf("command calls = %#v", commands.calls)
			}
		})
	}
}

func TestActionControllerRejectsWrongKindAndMissingPaneTargets(t *testing.T) {
	t.Run("wrong kind", func(t *testing.T) {
		controller, _, panes, _, commands := newFakeActionController()
		context := validActionContext()
		context.Origin = termaction.Ref{Kind: termaction.RefTab, ID: 7}
		envelope := termaction.Envelope{Action: termaction.CopySelection{}, Target: termaction.TargetOrigin}
		err := controller.executeAction(envelope, context)
		assertActionErrorClass(t, err, termaction.ErrorTarget)
		if len(panes.calls) != 0 || len(commands.calls) != 0 {
			t.Fatalf("wrong-kind target reached ports: panes=%v commands=%d", panes.calls, len(commands.calls))
		}
	})

	t.Run("missing pane", func(t *testing.T) {
		controller, _, panes, _, commands := newFakeActionController()
		envelope := termaction.Envelope{Action: termaction.CopySelection{}, Target: termaction.TargetOrigin}
		err := controller.executeAction(envelope, validActionContext())
		assertActionErrorClass(t, err, termaction.ErrorTarget)
		if !errors.Is(err, termaction.ErrTargetUnavailable) || !reflect.DeepEqual(panes.calls, []termmux.PaneID{7}) || len(commands.calls) != 0 {
			t.Fatalf("missing pane err=%v panes=%v commands=%d", err, panes.calls, len(commands.calls))
		}
	})
}

func TestActionControllerNonPaneCommandSkipsPaneLookup(t *testing.T) {
	controller, _, panes, _, commands := newFakeActionController()
	envelope := termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused}
	if err := controller.executeAction(envelope, validActionContext()); err != nil {
		t.Fatal(err)
	}
	if len(panes.calls) != 0 || len(commands.calls) != 1 || commands.calls[0].pane != 0 {
		t.Fatalf("panes=%v commands=%#v", panes.calls, commands.calls)
	}
}

func TestActionControllerMultipleRefreshesActiveProjectionPerChildAndStopsOnFirstError(t *testing.T) {
	stop := errors.New("stop")
	var log []string
	first := &fakeActionExecutionRoute{name: "first", focusedID: 11, log: &log}
	second := &fakeActionExecutionRoute{name: "second", focusedID: 22, log: &log, err: stop}
	third := &fakeActionExecutionRoute{name: "third", focusedID: 33, log: &log}
	controller, projections, _, _, commands := newFakeActionController()
	projections.routes = []actionExecutionRoute{first, second, third}
	multiple, err := termaction.NewMultiple(
		termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused},
		termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused},
		termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = controller.executeAction(termaction.Envelope{Action: multiple, Target: termaction.TargetFocused}, validActionContext())
	if !errors.Is(err, stop) {
		t.Fatalf("error = %v, want stop", err)
	}
	want := []string{"refresh-first", "execute-first", "refresh-second", "execute-second"}
	if !reflect.DeepEqual(log, want) || projections.calls != 2 || len(commands.calls) != 0 {
		t.Fatalf("log=%v projections=%d commands=%d", log, projections.calls, len(commands.calls))
	}
}

func TestActionControllerMultipleFallsBackToLocalRouteAndCarriesRefreshedContext(t *testing.T) {
	controller, projections, _, contexts, commands := newFakeActionController()
	contexts.focused = []uint64{41, 42}
	multiple, err := termaction.NewMultiple(
		termaction.Envelope{Action: termaction.ToggleStats{}, Target: termaction.TargetFocused},
		termaction.Envelope{Action: termaction.ReloadConfig{}, Target: termaction.TargetFocused},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.executeAction(termaction.Envelope{Action: multiple, Target: termaction.TargetFocused}, validActionContext()); err != nil {
		t.Fatal(err)
	}
	if projections.calls != 2 || contexts.calls != 2 || len(commands.calls) != 2 {
		t.Fatalf("projections=%d contexts=%d commands=%d", projections.calls, contexts.calls, len(commands.calls))
	}
	if commands.calls[0].context.Focused.ID != 41 || commands.calls[1].context.Focused.ID != 42 {
		t.Fatalf("focused contexts = %d, %d", commands.calls[0].context.Focused.ID, commands.calls[1].context.Focused.ID)
	}
}
