//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	"cervterm/internal/modal"
	"fmt"
)

func (a *App) openTabSwitcher() error {
	pane, ok := a.mux.FocusedPane()
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	tabs := a.mux.Tabs()
	if len(tabs) == 0 {
		return fmt.Errorf("tab switcher has no tabs")
	}
	entries := make([]modal.Entry, 0, len(tabs))
	activations := make(map[string]commandPaletteActivation, len(tabs))
	for i, tab := range tabs {
		title := tab.Title
		if title == "" {
			title = fmt.Sprintf("Tab %d", tab.ID)
		}
		id := fmt.Sprintf("tab:%d", tab.ID)
		entries = append(entries, modal.Entry{ID: id, Label: title, Detail: fmt.Sprintf("%d pane(s)", len(tab.Panes)), Category: "tab"})
		activations[id] = commandPaletteActivation{envelope: termaction.Envelope{Action: termaction.ActivateTab{TabID: uint64(tab.ID)}, Target: termaction.TargetFocused}, generation: a.scriptGeneration}
		_ = i
	}
	if !a.modal.Open(modal.ModeCommandPalette, modal.PaneIdentity(pane), modal.FocusIdentity(pane), entries) {
		return fmt.Errorf("tab switcher could not open")
	}
	a.commandPalette = activations
	a.requestRedraw()
	return nil
}
