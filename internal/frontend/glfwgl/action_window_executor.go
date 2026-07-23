//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func (a *App) requireWindowTarget(id uint64) error {
	if id == 0 || a.controller == nil || a.controller.projectionApp(termmux.WindowID(id)) == nil {
		return termaction.ErrTargetUnavailable
	}
	return nil
}

func (a *App) transferGeometry(source, destination termmux.WindowID) (termmux.PixelRect, termmux.PixelRect, termmux.CellMetricsResolver, error) {
	if a.controller == nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	src, dst := a.controller.projectionApp(source), a.controller.projectionApp(destination)
	if src == nil || dst == nil {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	sw, sh := src.lastFBW, src.lastFBH
	dw, dh := dst.lastFBW, dst.lastFBH
	if (sw <= 0 || sh <= 0) && src.window != nil {
		sw, sh = src.window.GetFramebufferSize()
	}
	if (dw <= 0 || dh <= 0) && dst.window != nil {
		dw, dh = dst.window.GetFramebufferSize()
	}
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return termmux.PixelRect{}, termmux.PixelRect{}, nil, termaction.ErrTargetUnavailable
	}
	resolve := func(id termmux.PaneID) (termmux.CellMetrics, bool) {
		windowID, ok := a.mux.WindowForPane(id)
		if !ok {
			return termmux.CellMetrics{}, false
		}
		projection := a.controller.projectionApp(windowID)
		if projection == nil {
			return termmux.CellMetrics{}, false
		}
		cellW, cellH := projection.cellW, projection.cellH
		if state := projection.paneUI[id]; state != nil && state.font.cellW > 0 && state.font.cellH > 0 {
			cellW, cellH = state.font.cellW, state.font.cellH
		}
		return termmux.CellMetrics{CellWidth: max(1, int(cellW)), CellHeight: max(1, int(cellH))}, true
	}
	return src.muxContentBounds(sw, sh), dst.muxContentBounds(dw, dh), resolve, nil
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
	sourceProjection := a.controller.projectionApp(source)
	cancelSource := sourceProjection != nil && sourceProjection.compositionTargetsTab(termmux.TabID(command.TabID))
	events, err := a.mux.TransferTabBetweenWindows(termmux.TabTransferRequest{SourceWindow: source, DestinationWindow: destination, Tab: termmux.TabID(command.TabID), Position: command.Position, SourceBounds: sb, DestinationBounds: db, Resolve: resolve})
	if err != nil {
		return err
	}
	if cancelSource {
		_ = sourceProjection.cancelComposition(ime.CancelTargetChanged)
	}
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
	for _, window := range a.mux.Windows() {
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
	sourceProjection := a.controller.projectionApp(source)
	cancelSource := sourceProjection != nil && sourceProjection.compositionTargetsPane(termmux.PaneID(command.PaneID))
	events, err := a.mux.TransferPaneBetweenWindows(termmux.PaneTransferRequest{SourceWindow: source, DestinationWindow: destination, Pane: termmux.PaneID(command.PaneID), DestinationTab: active.ID, DestinationPane: active.Focused, Axis: axis, Ratio: termmux.DefaultSplitRatio, SourceBounds: sb, DestinationBounds: db, Resolve: resolve})
	if err != nil {
		return err
	}
	if cancelSource {
		_ = sourceProjection.cancelComposition(ime.CancelTargetChanged)
	}
	a.controller.dispatch(events)
	return nil
}
