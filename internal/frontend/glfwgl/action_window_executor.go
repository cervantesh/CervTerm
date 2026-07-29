//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

func (a *App) requireWindowTarget(id uint64) error {
	if id == 0 || a.controller == nil || !a.controller.projectionAvailable(termmux.WindowID(id)) {
		return termaction.ErrTargetUnavailable
	}
	return nil
}

func (a *App) transferGeometry(source, destination termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error) {
	if a.controller == nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	return a.controller.transferProjectionGeometry(a.windowIdentity, source, destination)
}

func (a *App) executeMoveTabToWindow(context termaction.Context, command termaction.MoveTabToWindow) error {
	if !context.OriginWindow.Valid() || context.OriginWindow.Kind != termaction.RefWindow {
		return termaction.ErrTargetUnavailable
	}
	source, destination := termmux.WindowID(context.OriginWindow.ID), termmux.WindowID(command.WindowID)
	sb, db, resolve, err := a.transferGeometry(source, destination)
	if err != nil {
		return err
	}
	events, err := a.controller.transferTabBetweenWindows(a.windowIdentity, termmux.TabTransferRequest{SourceWindow: source, DestinationWindow: destination, Tab: termmux.TabID(command.TabID), Position: command.Position, SourceBounds: sb, DestinationBounds: db, Resolve: resolve})
	if err != nil {
		return err
	}
	a.controller.cancelProjectionTabComposition(source, termmux.TabID(command.TabID))
	a.controller.dispatch(events)
	return nil
}

func (a *App) executeMovePaneToWindow(context termaction.Context, command termaction.MovePaneToWindow) error {
	if !context.OriginWindow.Valid() || context.OriginWindow.Kind != termaction.RefWindow {
		return termaction.ErrTargetUnavailable
	}
	source, destination := termmux.WindowID(context.OriginWindow.ID), termmux.WindowID(command.WindowID)
	sb, db, resolve, err := a.transferGeometry(source, destination)
	if err != nil {
		return err
	}
	var active termmux.TabView
	found := false
	windows, err := a.controller.processWindows(a.windowIdentity)
	if err != nil {
		return err
	}
	for _, window := range windows {
		if window.ID == destination {
			for _, tab := range window.Tabs {
				if tab.Active {
					active, found = tab, true
					break
				}
			}
			break
		}
	}
	if !found {
		return termaction.ErrTargetUnavailable
	}
	axis := termmux.SplitColumns
	if command.Axis == termaction.SplitRows {
		axis = termmux.SplitRows
	}
	events, err := a.controller.transferPaneBetweenWindows(a.windowIdentity, termmux.PaneTransferRequest{SourceWindow: source, DestinationWindow: destination, Pane: termmux.PaneID(command.PaneID), DestinationTab: active.ID, DestinationPane: active.Focused, Axis: axis, Ratio: termmux.DefaultSplitRatio, SourceBounds: sb, DestinationBounds: db, Resolve: resolve})
	if err != nil {
		return err
	}
	a.controller.cancelProjectionPaneComposition(source, termmux.PaneID(command.PaneID))
	a.controller.dispatch(events)
	return nil
}
