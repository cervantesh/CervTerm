//go:build glfw

package glfwgl

import (
	"errors"

	termaction "cervterm/internal/action"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
	termsel "cervterm/internal/selection"
)

func semanticRangeForCurrentCycle(snapshot termmux.SemanticSnapshot, zone termaction.SemanticZone) (core.SemanticRange, bool) {
	reference := snapshot.ViewportTopGlobalRow
	if reference == snapshot.TotalRows-snapshot.Rows {
		reference = snapshot.CursorGlobalRow
	}
	promptIndex := -1
	for index, rangeValue := range snapshot.Ranges {
		if rangeValue.Kind == core.SemanticPrompt && rangeValue.Start.GlobalRow <= reference {
			promptIndex = index
		}
	}
	if promptIndex < 0 {
		return core.SemanticRange{}, false
	}
	end := len(snapshot.Ranges)
	for index := promptIndex + 1; index < len(snapshot.Ranges); index++ {
		if snapshot.Ranges[index].Kind == core.SemanticPrompt {
			end = index
			break
		}
	}
	kind := core.SemanticInput
	if zone == termaction.SemanticZoneOutput {
		kind = core.SemanticOutput
	}
	for index := promptIndex + 1; index < end; index++ {
		if snapshot.Ranges[index].Kind == kind {
			return snapshot.Ranges[index], true
		}
	}
	return core.SemanticRange{}, false
}

func (a *App) executeCopySemanticZone(pane termmux.PaneID, command termaction.CopySemanticZone) error {
	snapshot, ok := a.mux.SemanticSnapshot(pane)
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	target, ok := semanticRangeForCurrentCycle(snapshot, command.Zone)
	if !ok {
		return errors.New("semantic zone is unavailable")
	}
	text, err := a.mux.SemanticRangeText(snapshot, target)
	if err != nil {
		return err
	}
	a.SetClipboard(text)
	return nil
}

func (a *App) executeSelectSemanticZone(pane termmux.PaneID, command termaction.SelectSemanticZone) error {
	snapshot, ok := a.mux.SemanticSnapshot(pane)
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	target, ok := semanticRangeForCurrentCycle(snapshot, command.Zone)
	if !ok {
		return errors.New("semantic zone is unavailable")
	}
	if err := a.mux.ValidateSemanticRange(snapshot, target); err != nil {
		return err
	}
	span := target.End.GlobalRow - target.Start.GlobalRow + 1
	if span > snapshot.Rows {
		return errors.New("semantic zone does not fit in the viewport")
	}
	maxTop := snapshot.TotalRows - snapshot.Rows
	top := target.Start.GlobalRow
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	if target.Start.GlobalRow < top || target.End.GlobalRow >= top+snapshot.Rows {
		return errors.New("semantic zone cannot be projected into the viewport")
	}
	moved, err := a.mux.ScrollViewportToGlobalRow(pane, target.Start.GlobalRow)
	if err != nil {
		return err
	}
	selection := selectionState{active: true, start: termsel.Point{Row: target.Start.GlobalRow - top, Col: target.Start.Col}, end: termsel.Point{Row: target.End.GlobalRow - top, Col: max(0, target.End.Col-1)}}
	if pane == a.focusedPane {
		a.saveActivePaneUI()
	}
	state := a.ensurePaneUI(pane)
	state.selection = selection
	if pane == a.focusedPane {
		a.selection = selection
	}
	if moved {
		a.recordPaneScroll(pane)
	}
	a.requestAccessibilityRedraw()
	return nil
}
