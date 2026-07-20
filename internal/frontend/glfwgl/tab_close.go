//go:build glfw

package glfwgl

import (
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"fmt"
)

type tabCloseConfirmation struct {
	Tab      termmux.TabID
	Revision uint64
}

func (a *App) requestTabClose(id termmux.TabID) ([]termmux.Event, error) {
	var target termmux.TabView
	found := false
	for _, tab := range a.mux.Tabs() {
		if tab.ID == id {
			target = tab
			found = true
			break
		}
	}
	if !found {
		return nil, termmux.ErrTabNotFound
	}
	running := false
	for _, pane := range target.Panes {
		if view, ok := a.mux.PaneView(pane); ok && (view.State == termmux.PaneStateRunning || view.State == termmux.PaneStateStarting) {
			running = true
			break
		}
	}
	if !running {
		return a.mux.CloseTab(id)
	}
	pane, _ := a.mux.FocusedPane()
	entry := modal.Entry{ID: "close-tab", Label: fmt.Sprintf("Close %s", tabDisplayTitle(target)), Detail: fmt.Sprintf("%d running pane(s)", len(target.Panes)), Category: "tab"}
	if !a.modal.Open(modal.ModeTabCloseConfirmation, modal.PaneIdentity(pane), modal.FocusIdentity(pane), []modal.Entry{entry}) {
		return nil, fmt.Errorf("close confirmation could not open")
	}
	a.tabClose = tabCloseConfirmation{Tab: id, Revision: target.Revision}
	a.requestRedraw()
	return nil, nil
}
func (a *App) acceptTabClose(entry modal.Entry) error {
	if entry.ID != "close-tab" || a.tabClose.Tab == 0 {
		return fmt.Errorf("close target is unavailable")
	}
	var target termmux.TabView
	found := false
	for _, tab := range a.mux.Tabs() {
		if tab.ID == a.tabClose.Tab {
			target = tab
			found = true
			break
		}
	}
	if !found {
		return termmux.ErrTabNotFound
	}
	if target.Revision != a.tabClose.Revision {
		return fmt.Errorf("tab changed while confirmation was open")
	}
	events, err := a.mux.CloseTab(target.ID)
	a.handleMuxEvents(events)
	if err == nil {
		a.tabClose = tabCloseConfirmation{}
	}
	return err
}
func tabDisplayTitle(tab termmux.TabView) string {
	if tab.Title != "" {
		return tab.Title
	}
	return fmt.Sprintf("Tab %d", tab.ID)
}
