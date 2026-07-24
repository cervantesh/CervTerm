//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"time"

	termaction "cervterm/internal/action"
	"cervterm/internal/core"
	"cervterm/internal/input"
	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
)

func (a *App) executeAction(envelope termaction.Envelope, context termaction.Context) error {
	return a.ensureActionController().executeAction(envelope, context)
}

func (a *App) executeActionCommand(envelope termaction.Envelope, context termaction.Context, pane termmux.PaneID) error {
	switch command := envelope.Action.(type) {
	case termaction.CopySelection:
		text := newPaneHost(a, pane).Selection()
		if text != "" {
			a.SetClipboard(text)
		}
	case termaction.CopySemanticZone:
		if err := a.executeCopySemanticZone(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.SelectSemanticZone:
		if err := a.executeSelectSemanticZone(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.PasteClipboard:
		view, _ := a.mux.PaneView(pane)
		a.writePaneInputBytes(pane, input.EncodePaste(a.Clipboard(), view.BracketedPaste))
	case termaction.ToggleSearch:
		if err := a.focusActionPane(pane); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
		if a.search.active {
			a.search.close()
		} else {
			if !a.search.open() {
				return actionExecutionError(command, termaction.ErrorAction, errTextTargetExhausted)
			}
		}
	case termaction.ActivateCommandPalette:
		if err := a.openCommandPalette(); err != nil {
			return actionExecutionError(command, termaction.ErrorAction, err)
		}
	case termaction.ActivateQuickSelect:
		if err := a.focusActionPane(pane); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
		if err := a.openQuickSelect(pane); err != nil {
			return actionExecutionError(command, termaction.ErrorAction, err)
		}
	case termaction.ActivateLaunchMenu:
		if err := a.focusActionPane(pane); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
		if err := a.openLaunchMenu(pane); err != nil {
			return actionExecutionError(command, termaction.ErrorAction, err)
		}
	case termaction.ToggleStats:
		a.showStats = !a.showStats
		a.requestRedraw()
	case termaction.Scroll:
		if err := a.executeScrollAction(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ScrollToPrompt:
		if err := a.executeScrollToPrompt(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.Zoom:
		a.executeZoomAction(pane, command)
	case termaction.ReloadConfig:
		if !a.requestConfigReload() {
			return actionExecutionError(command, termaction.ErrorAction, errors.New("no config source to reload"))
		}
	case termaction.NewTab:
		_, _, events, err := a.mux.SpawnTab(a.desiredShellSpawnSpec(), termmux.CellMetrics{CellWidth: max(1, int(a.cellW)), CellHeight: max(1, int(a.cellH))}, "")
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.NewWindow:
		if a.controller == nil {
			return actionExecutionError(command, termaction.ErrorTarget, termaction.ErrTargetUnavailable)
		}
		if _, err := a.controller.createRuntimeProjection(); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.CloseWindow:
		if err := a.requireWindowTarget(command.WindowID); err != nil {
			return actionExecutionError(command, termaction.ErrorTarget, err)
		}
		if _, err := a.controller.closeRuntimeProjection(termmux.WindowID(command.WindowID)); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.FocusWindow:
		if err := a.requireWindowTarget(command.WindowID); err != nil {
			return actionExecutionError(command, termaction.ErrorTarget, err)
		}
		if err := a.controller.activateRuntimeProjection(termmux.WindowID(command.WindowID)); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.MoveTabToWindow:
		if err := a.executeMoveTabToWindow(context, command); err != nil {
			return actionExecutionError(command, termaction.ErrorTarget, err)
		}
	case termaction.MovePaneToWindow:
		if err := a.executeMovePaneToWindow(context, command); err != nil {
			return actionExecutionError(command, termaction.ErrorTarget, err)
		}
	case termaction.CreateWorkspace:
		if err := a.executeCreateWorkspace(command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.SwitchWorkspace:
		if err := a.executeSwitchWorkspace(command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.RenameWorkspace:
		if err := a.executeRenameWorkspace(command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.MoveWindowToWorkspace:
		if err := a.requireWindowTarget(command.WindowID); err != nil {
			return actionExecutionError(command, termaction.ErrorTarget, err)
		}
		if err := a.executeMoveWindowToWorkspace(command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ActivateWorkspaceSwitcher:
		if err := a.openWorkspaceSwitcher(); err != nil {
			return actionExecutionError(command, termaction.ErrorAction, err)
		}
	case termaction.ActivateTab:
		events, err := a.mux.ActivateTab(termmux.TabID(command.TabID))
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ActivateTabRelative:
		if err := a.executeRelativeTabAction(envelope, context, command.Delta); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.MoveTab:
		events, err := a.mux.MoveTab(termmux.TabID(command.TabID), command.Position)
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.RenameTab:
		events, err := a.mux.RenameTab(termmux.TabID(command.TabID), command.Title)
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.CloseTab:
		events, err := a.mux.CloseTab(termmux.TabID(command.TabID))
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.MovePaneToTab:
		if err := a.executeMovePaneToTab(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ActivateTabSwitcher:
		if err := a.openTabSwitcher(); err != nil {
			return actionExecutionError(command, termaction.ErrorAction, err)
		}
	case termaction.SplitPane:
		if err := a.executeSplitAction(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.FocusPane:
		if err := a.executeFocusAction(pane, command); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ResizePane:
		if err := a.executeTopologyAction(pane, command.Direction, func(direction termmux.Direction) ([]termmux.Event, error) {
			return a.mux.ResizeCurrentPane(direction, command.Delta)
		}); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.SwapPane:
		if err := a.executeTopologyAction(pane, command.Direction, a.mux.SwapCurrentPane); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.MovePane:
		if err := a.executeTopologyAction(pane, command.Direction, a.mux.MoveCurrentPane); err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.ClosePane:
		events, err := a.mux.ClosePane(pane)
		a.handleMuxEvents(events)
		if err != nil {
			return actionExecutionError(command, termaction.ErrorMux, err)
		}
	case termaction.Callback:
		if a.scriptRT == nil {
			return actionExecutionError(command, termaction.ErrorScript, errors.New("script runtime is unavailable"))
		}
		if err := a.scriptRT.Dispatch(command.BindingIndex, newPaneHost(a, pane)); err != nil {
			return actionExecutionError(command, termaction.ErrorScript, err)
		}
	default:
		return actionExecutionError(command, termaction.ErrorAction, fmt.Errorf("unsupported concrete action %T", command))
	}
	return nil
}

func (a *App) focusedActionRef() termaction.Ref {
	if a.mux == nil {
		return termaction.Ref{}
	}
	pane, ok := a.mux.FocusedPane()
	if !ok {
		return termaction.Ref{}
	}
	return termaction.Ref{Kind: termaction.RefPane, ID: uint64(pane)}
}

func (a *App) actionContext(source termaction.Source) termaction.Context {
	focused := a.focusedActionRef()
	window := a.windowActionRef()
	return termaction.Context{Source: source, Origin: focused, Focused: focused, OriginWindow: window, FocusedWindow: window}
}

func (a *App) focusActionPane(pane termmux.PaneID) error {
	focused, ok := a.mux.FocusedPane()
	if ok && focused == pane {
		return nil
	}
	events, err := a.mux.FocusPane(pane)
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeScrollAction(pane termmux.PaneID, command termaction.Scroll) error {
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	lines := command.Amount
	switch command.Unit {
	case termaction.ScrollPage:
		page := max(1, view.Snapshot.Rows-1)
		lines *= page
	case termaction.ScrollBuffer:
		lines = view.ScrollbackLines
		if command.Amount < 0 {
			lines = -lines
		}
	}
	moved, err := a.mux.ScrollViewport(pane, lines)
	if err != nil {
		return err
	}
	if moved {
		a.recordPaneScroll(pane)
		if pane == a.focusedPane && a.window != nil && a.cfg.Scrollbar.Enabled {
			a.scrollbar.lastActivity = time.Now()
		}
		a.requestAccessibilityRedraw()
	}
	return nil
}

func (a *App) executeScrollToPrompt(pane termmux.PaneID, command termaction.ScrollToPrompt) error {
	snapshot, ok := a.mux.SemanticSnapshot(pane)
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	target := -1
	if command.Delta < 0 {
		reference := snapshot.ViewportTopGlobalRow
		for index := len(snapshot.Ranges) - 1; index >= 0; index-- {
			rangeValue := snapshot.Ranges[index]
			if rangeValue.Kind == core.SemanticPrompt && rangeValue.Start.GlobalRow < reference {
				target = rangeValue.Start.GlobalRow
				break
			}
		}
	} else {
		for _, rangeValue := range snapshot.Ranges {
			if rangeValue.Kind == core.SemanticPrompt && rangeValue.Start.GlobalRow > snapshot.ViewportTopGlobalRow {
				target = rangeValue.Start.GlobalRow
				break
			}
		}
	}
	if target < 0 {
		return errors.New("semantic prompt is unavailable")
	}
	if !a.mux.SemanticSnapshotCurrent(snapshot) {
		return errors.New("semantic prompt snapshot is stale")
	}
	moved, err := a.mux.ScrollViewportToGlobalRow(pane, target)
	if err != nil {
		return err
	}
	if moved {
		a.recordPaneScroll(pane)
		a.requestAccessibilityRedraw()
	}
	return nil
}

func (a *App) executeZoomAction(pane termmux.PaneID, command termaction.Zoom) {
	target := a.zoom.base
	if command.Mode == termaction.ZoomDelta {
		target = a.paneZoomTarget(pane) + command.Amount
	}
	state := a.ensurePaneUI(pane)
	state.font.pendingTarget = clampZoomFontSize(target)
	state.font.pending = true
	state.font.resizeAttempt = 0
	state.font.deadline = time.Now().Add(zoomDebounce)
	a.requestRedraw()
}

func (a *App) executeSplitAction(source termmux.PaneID, command termaction.SplitPane) error {
	axis := termmux.SplitColumns
	if command.Axis == termaction.SplitRows {
		axis = termmux.SplitRows
	}
	spawn := a.desiredShellSpawnSpec()
	created, events, err := a.mux.Split(source, axis, spawn)
	if created != 0 {
		a.inheritPaneFontState(created, source)
	}
	a.handleMuxEvents(events)
	return err
}

func (a *App) desiredShellSpawnSpec() termmux.SpawnSpec {
	a.ensureConfigState()
	shell := a.desiredCfg.Shell
	return termmux.SpawnSpec{Options: pty.Options{ShellProgram: shell.Program, ShellArgs: append([]string(nil), shell.Args...), WorkingDirectory: shell.WorkingDirectory, Env: cloneStringMap(shell.Env)}}
}

func (a *App) executeFocusAction(source termmux.PaneID, command termaction.FocusPane) error {
	if err := a.focusActionPane(source); err != nil {
		return err
	}
	direction := termmux.FocusLeft
	switch command.Direction {
	case termaction.FocusRight:
		direction = termmux.FocusRight
	case termaction.FocusUp:
		direction = termmux.FocusUp
	case termaction.FocusDown:
		direction = termmux.FocusDown
	}
	events, err := a.mux.FocusDirection(direction)
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeTopologyAction(source termmux.PaneID, direction termaction.Direction, execute func(termmux.Direction) ([]termmux.Event, error)) error {
	if err := a.focusActionPane(source); err != nil {
		return err
	}
	muxDirection := termmux.FocusLeft
	switch direction {
	case termaction.FocusRight:
		muxDirection = termmux.FocusRight
	case termaction.FocusUp:
		muxDirection = termmux.FocusUp
	case termaction.FocusDown:
		muxDirection = termmux.FocusDown
	}
	events, err := execute(muxDirection)
	a.handleMuxEvents(events)
	return err
}

func actionExecutionError(command termaction.Action, class termaction.ErrorClass, err error) error {
	id := termaction.ID("")
	if command != nil {
		id = command.ID()
	}
	return &termaction.ExecutionError{ActionID: id, Class: class, Err: err}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func (a *App) executeRelativeTabAction(envelope termaction.Envelope, context termaction.Context, delta int) error {
	ref, err := context.Resolve(envelope.Target)
	if err != nil || ref.Kind != termaction.RefPane {
		return termaction.ErrTargetUnavailable
	}
	owner, ok := a.mux.TabForPane(termmux.PaneID(ref.ID))
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	tabs := a.mux.Tabs()
	index := -1
	for i := range tabs {
		if tabs[i].ID == owner {
			index = i
			break
		}
	}
	if index < 0 || len(tabs) == 0 {
		return termaction.ErrTargetUnavailable
	}
	next := ((index+delta)%len(tabs) + len(tabs)) % len(tabs)
	events, err := a.mux.ActivateTab(tabs[next].ID)
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeMovePaneToTab(pane termmux.PaneID, command termaction.MovePaneToTab) error {
	var target termmux.TabView
	found := false
	for _, tab := range a.mux.Tabs() {
		if tab.ID == termmux.TabID(command.TabID) {
			target = tab
			found = true
			break
		}
	}
	if !found {
		return termmux.ErrTabNotFound
	}
	axis := termmux.SplitColumns
	if command.Axis == termaction.SplitRows {
		axis = termmux.SplitRows
	}
	events, err := a.mux.TransferPane(pane, target.ID, target.Focused, axis)
	a.handleMuxEvents(events)
	return err
}
