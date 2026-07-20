//go:build glfw

package glfwgl

import (
	"errors"

	termaction "cervterm/internal/action"
	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
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
