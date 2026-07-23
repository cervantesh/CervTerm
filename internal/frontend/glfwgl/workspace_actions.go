//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"
	"cervterm/internal/modal"
	termmux "cervterm/internal/mux"
	"fmt"
)

type workspaceSwitcherActivation struct {
	workspace termmux.WorkspaceID
	revision  uint64
}

func (a *App) executeCreateWorkspace(command termaction.CreateWorkspace) error {
	_, events, err := a.mux.CreateWorkspace(command.Name)
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeSwitchWorkspace(command termaction.SwitchWorkspace) error {
	events, err := a.mux.SwitchWorkspace(termmux.WorkspaceID(command.WorkspaceID))
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeRenameWorkspace(command termaction.RenameWorkspace) error {
	events, err := a.mux.RenameWorkspace(termmux.WorkspaceID(command.WorkspaceID), command.Name)
	a.handleMuxEvents(events)
	return err
}

func (a *App) executeMoveWindowToWorkspace(command termaction.MoveWindowToWorkspace) error {
	if err := a.requireWindowTarget(command.WindowID); err != nil {
		return err
	}
	events, err := a.mux.MoveWindowToWorkspace(termmux.WindowID(command.WindowID), termmux.WorkspaceID(command.WorkspaceID))
	a.handleMuxEvents(events)
	return err
}

func (a *App) openWorkspaceSwitcher() error {
	pane, ok := a.mux.FocusedPane()
	if !ok {
		return termaction.ErrTargetUnavailable
	}
	workspaces := a.mux.Workspaces()
	if len(workspaces) == 0 {
		return fmt.Errorf("workspace switcher has no workspaces")
	}
	entries := make([]modal.Entry, 0, len(workspaces))
	activations := make(map[string]workspaceSwitcherActivation, len(workspaces))
	for _, workspace := range workspaces {
		id := fmt.Sprintf("workspace:%d", workspace.ID)
		detail := fmt.Sprintf("%d window(s)", len(workspace.Windows))
		if workspace.Active {
			detail += " • active"
		}
		entries = append(entries, modal.Entry{ID: id, Label: workspace.Name, Detail: detail, Category: "workspace"})
		activations[id] = workspaceSwitcherActivation{workspace: workspace.ID, revision: workspace.Revision}
	}
	if !a.openModal(modal.ModeWorkspaceSwitcher, modal.PaneIdentity(pane), modal.FocusIdentity(pane), entries) {
		return fmt.Errorf("workspace switcher could not open")
	}
	a.workspaceSwitcher = activations
	a.requestRedraw()
	return nil
}

func (a *App) acceptWorkspaceSwitcher(entry modal.Entry) error {
	activation, ok := a.workspaceSwitcher[entry.ID]
	if !ok {
		return fmt.Errorf("workspace switcher entry %q is unavailable", entry.ID)
	}
	var current termmux.WorkspaceView
	found := false
	for _, workspace := range a.mux.Workspaces() {
		if workspace.ID == activation.workspace {
			current, found = workspace, true
			break
		}
	}
	if !found {
		return termmux.ErrWorkspaceNotFound
	}
	if current.Revision != activation.revision {
		return fmt.Errorf("workspace %d changed while switcher was open", activation.workspace)
	}
	events, err := a.mux.SwitchWorkspace(activation.workspace)
	a.handleMuxEvents(events)
	return err
}
