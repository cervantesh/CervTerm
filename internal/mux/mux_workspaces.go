package mux

func (m *Mux) Workspaces() []WorkspaceView { return m.model.Workspaces() }

func (m *Mux) ActiveWorkspace() WorkspaceView { return m.model.ActiveWorkspace() }

func (m *Mux) CreateWorkspace(name string) (WorkspaceView, []Event, error) {
	view, err := m.model.CreateWorkspace(name)
	if err != nil {
		return WorkspaceView{}, nil, err
	}
	return view, []Event{{Kind: WorkspaceCreated, Workspace: view.ID, Text: view.Name, Revision: view.Revision}}, nil
}

func (m *Mux) RenameWorkspace(id WorkspaceID, name string) ([]Event, error) {
	current := m.model.workspaceByID(id)
	normalized, normalizeErr := normalizeWorkspaceName(name)
	if current == nil {
		return nil, ErrWorkspaceNotFound
	}
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if current.name == normalized {
		return nil, nil
	}
	if err := m.model.RenameWorkspace(id, name); err != nil {
		return nil, err
	}
	view := m.model.workspaceByID(id)
	return []Event{{Kind: WorkspaceRenamed, Workspace: id, Text: view.name, Revision: view.revision}}, nil
}

func (m *Mux) SwitchWorkspace(id WorkspaceID) ([]Event, error) {
	if m.model.activeWorkspace == id {
		return nil, nil
	}
	source := m.model.activeWorkspace
	if err := m.model.SwitchWorkspace(id); err != nil {
		return nil, err
	}
	view := m.model.ActiveWorkspace()
	events := []Event{{Kind: WorkspaceActivated, Workspace: id, SourceWorkspace: source, Window: view.Focused, Revision: view.Revision}}
	if view.Focused != 0 {
		w := m.model.windowByID(view.Focused)
		events = append(events, Event{Kind: WindowActivated, Workspace: id, SourceWorkspace: source, Window: w.id, Tab: w.active, Pane: m.model.FocusedPane(), Revision: w.revision})
	}
	return events, nil
}

func (m *Mux) MoveWindowToWorkspace(window WindowID, target WorkspaceID) ([]Event, error) {
	w := m.model.windowByID(window)
	if w == nil {
		return nil, ErrWindowNotFound
	}
	source := w.workspace
	if source == target {
		return nil, nil
	}
	previousActive := m.model.activeWindow
	if err := m.model.MoveWindowToWorkspace(window, target); err != nil {
		return nil, err
	}
	view := m.model.windowByID(window)
	events := []Event{{Kind: WindowWorkspaceChanged, Window: window, Workspace: target, SourceWorkspace: source, Revision: view.revision}}
	active := m.model.ActiveWorkspace()
	if active.Focused != 0 && active.Focused != previousActive {
		aw := m.model.windowByID(active.Focused)
		events = append(events, Event{Kind: WindowActivated, Window: aw.id, Workspace: active.ID, SourceWorkspace: source, Tab: aw.active, Pane: m.model.FocusedPane(), Revision: aw.revision})
	}
	return events, nil
}
