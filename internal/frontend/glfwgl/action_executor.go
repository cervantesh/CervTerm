//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"time"

	termaction "cervterm/internal/action"
	"cervterm/internal/input"
	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
)

func (a *App) executeAction(envelope termaction.Envelope, context termaction.Context) error {
	if err := envelope.Validate(); err != nil {
		return actionExecutionError(envelope.Action, termaction.ErrorAction, err)
	}
	if err := context.Validate(); err != nil {
		return actionExecutionError(envelope.Action, termaction.ErrorAction, err)
	}
	if multiple, ok := envelope.Action.(termaction.Multiple); ok {
		for _, child := range multiple.Actions() {
			context.Focused = a.focusedActionRef()
			if err := a.executeAction(child, context); err != nil {
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
		if _, exists := a.mux.PaneView(pane); !exists {
			return actionExecutionError(envelope.Action, termaction.ErrorTarget, termaction.ErrTargetUnavailable)
		}
	}

	switch command := envelope.Action.(type) {
	case termaction.CopySelection:
		text := (paneHost{app: a, pane: pane}).Selection()
		if text != "" {
			a.SetClipboard(text)
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
			a.search.open()
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
	case termaction.Zoom:
		a.executeZoomAction(pane, command)
	case termaction.ReloadConfig:
		if !a.requestConfigReload() {
			return actionExecutionError(command, termaction.ErrorAction, errors.New("no config source to reload"))
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
		if err := a.scriptRT.Dispatch(command.BindingIndex, paneHost{app: a, pane: pane}); err != nil {
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
	return termaction.Context{Source: source, Origin: focused, Focused: focused}
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
		a.requestRedraw()
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
	a.ensureConfigState()
	shell := a.desiredCfg.Shell
	spawn := termmux.SpawnSpec{Options: pty.Options{
		ShellProgram:     shell.Program,
		ShellArgs:        append([]string(nil), shell.Args...),
		WorkingDirectory: shell.WorkingDirectory,
		Env:              cloneStringMap(shell.Env),
	}}
	created, events, err := a.mux.Split(source, axis, spawn)
	if created != 0 {
		a.inheritPaneFontState(created, source)
	}
	a.handleMuxEvents(events)
	return err
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
