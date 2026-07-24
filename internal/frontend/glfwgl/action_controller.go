//go:build glfw

package glfwgl

import (
	"fmt"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

// actionExecutionRoute is the narrow per-projection route needed when a
// Multiple action changes the active native projection between children.
type actionExecutionRoute interface {
	refreshFocusedActionContext(termaction.Context) termaction.Context
	executeAction(termaction.Envelope, termaction.Context) error
}

// actionProjectionPort supplies the currently active projection without
// exposing the native window controller or App facade.
type actionProjectionPort interface {
	activeActionRoute() actionExecutionRoute
}

// actionPanePort answers only the target-existence question owned by the
// action coordinator.
type actionPanePort interface {
	actionPaneExists(termmux.PaneID) bool
}

// actionContextPort refreshes the focused portion of an otherwise stable
// action context for the projection selected for a Multiple child.
type actionContextPort interface {
	refreshFocusedActionContext(termaction.Context) termaction.Context
}

// actionCommandPort is a temporary typed delegation seam that keeps concrete
// App effects behind one concern-specific method without exposing a facade.
// TODO(L1-01; expires Slice 6.3d): replace it with final owned collaborators.
type actionCommandPort interface {
	executeActionCommand(termaction.Envelope, termaction.Context, termmux.PaneID) error
}

// actionController coordinates validation, target selection, and sequential
// action routing while App retains concrete effect and mutable-state ownership.
type actionController struct {
	projections actionProjectionPort
	panes       actionPanePort
	contexts    actionContextPort
	commands    actionCommandPort
}

func newActionController(projections actionProjectionPort, panes actionPanePort, contexts actionContextPort, commands actionCommandPort) *actionController {
	return &actionController{
		projections: projections,
		panes:       panes,
		contexts:    contexts,
		commands:    commands,
	}
}

func (c *actionController) refreshFocusedActionContext(context termaction.Context) termaction.Context {
	return c.contexts.refreshFocusedActionContext(context)
}

func (c *actionController) executeAction(envelope termaction.Envelope, context termaction.Context) error {
	if err := envelope.Validate(); err != nil {
		return actionExecutionError(envelope.Action, termaction.ErrorAction, err)
	}
	if err := context.Validate(); err != nil {
		return actionExecutionError(envelope.Action, termaction.ErrorAction, err)
	}
	if multiple, ok := envelope.Action.(termaction.Multiple); ok {
		for _, child := range multiple.Actions() {
			route := actionExecutionRoute(c)
			if c.projections != nil {
				if active := c.projections.activeActionRoute(); active != nil {
					route = active
				}
			}
			context = route.refreshFocusedActionContext(context)
			if err := route.executeAction(child, context); err != nil {
				return err
			}
		}
		return nil
	}

	descriptor, ok := termaction.DefaultRegistry().Lookup(envelope.Action.ID())
	if !ok {
		return actionExecutionError(envelope.Action, termaction.ErrorAction, fmt.Errorf("action is not registered"))
	}

	var pane termmux.PaneID
	if descriptor.Target == termaction.TargetPane {
		resolved, err := context.Resolve(envelope.Target)
		if err != nil || resolved.Kind != termaction.RefPane {
			if err == nil {
				err = termaction.ErrTargetUnavailable
			}
			return actionExecutionError(envelope.Action, termaction.ErrorTarget, err)
		}
		pane = termmux.PaneID(resolved.ID)
		if !c.panes.actionPaneExists(pane) {
			return actionExecutionError(envelope.Action, termaction.ErrorTarget, termaction.ErrTargetUnavailable)
		}
	}

	return c.commands.executeActionCommand(envelope, context, pane)
}
