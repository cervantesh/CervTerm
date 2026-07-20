package mux

// WindowView is a detached immutable projection of mux-owned window state.
type WindowView struct {
	ID        WindowID
	Workspace WorkspaceID
	Title     string
	Tabs      []TabView
	Active    bool
	Revision  uint64
}

// CloseWindowResult reports detached ownership without creating or closing sessions.
type CloseWindowResult struct {
	Window           WindowID
	Workspace        WorkspaceID
	Tabs             []TabID
	Panes            []PaneID
	Splits           []SplitID
	Active           WindowID
	ActiveWorkspace  WorkspaceID
	ActiveTab        TabID
	FocusedPane      PaneID
	Closed           bool
	ActiveChanged    bool
	WorkspaceChanged bool
	Empty            bool
}

func (m *Model) Windows() []WindowView {
	out := make([]WindowView, len(m.windows))
	for i := range m.windows {
		out[i] = m.windowView(&m.windows[i])
	}
	return out
}

func (m *Model) ActiveWindow() WindowView {
	if w := m.activeWindowState(); w != nil {
		return m.windowView(w)
	}
	return WindowView{}
}

func (m *Model) windowView(w *windowState) WindowView {
	tabs := make([]TabView, len(w.tabs))
	for i := range w.tabs {
		tabs[i] = tabView(&w.tabs[i], w.active)
	}
	return WindowView{ID: w.id, Workspace: w.workspace, Title: w.title, Tabs: tabs, Active: w.id == m.activeWindow, Revision: w.revision}
}

// WindowCreateToken is a non-published reservation proposal. Preparing and
// aborting a token do not mutate the model; only CommitWindow consumes IDs.
type WindowCreateToken struct {
	window    WindowID
	workspace WorkspaceID
	tab       TabID
	pane      PaneID
	title     string
}

func (t WindowCreateToken) WindowID() WindowID { return t.window }
func (t WindowCreateToken) TabID() TabID       { return t.tab }
func (t WindowCreateToken) PaneID() PaneID     { return t.pane }

func (m *Model) PrepareWindow(title string) (WindowCreateToken, error) {
	if err := m.CheckInvariants(); err != nil {
		return WindowCreateToken{}, err
	}
	if len(m.windows) >= MaxWindows {
		return WindowCreateToken{}, ErrWindowLimitReached
	}
	if m.nextWindowID == 0 || m.nextTabID == 0 || m.nextPaneID == 0 {
		return WindowCreateToken{}, ErrIDExhausted
	}
	return WindowCreateToken{window: m.nextWindowID, workspace: m.activeWorkspace, tab: m.nextTabID, pane: m.nextPaneID, title: title}, nil
}

func (m *Model) CommitWindow(token WindowCreateToken) (WindowView, error) {
	if token.window == 0 || token.tab == 0 || token.pane == 0 {
		return WindowView{}, invariantError("invalid window create token")
	}
	if token.window != m.nextWindowID || token.tab != m.nextTabID || token.pane != m.nextPaneID {
		return WindowView{}, invariantError("stale window create token")
	}
	if len(m.windows) >= MaxWindows {
		return WindowView{}, ErrWindowLimitReached
	}
	previousActive := m.activeWindow
	ws := m.workspaceByID(token.workspace)
	if ws == nil || ws.id != m.activeWorkspace {
		return WindowView{}, invariantError("window token workspace %d is not active", token.workspace)
	}
	previousWorkspace := *ws
	previousWorkspace.windows = append([]WindowID(nil), ws.windows...)
	m.windows = append(m.windows, windowState{
		id: token.window, workspace: ws.id, title: token.title, active: token.tab, revision: 1,
		tabs: []tabState{{id: token.tab, root: leafNode(token.pane), focused: token.pane, revision: 1}},
	})
	ws.windows = append(ws.windows, token.window)
	ws.active = token.window
	ws.revision++
	m.activeWindow = token.window
	m.allocatedWindows[token.window] = struct{}{}
	m.allocatedTabs[token.tab] = struct{}{}
	m.allocated[token.pane] = struct{}{}
	m.nextWindowID++
	m.nextTabID++
	m.nextPaneID++
	if err := m.CheckInvariants(); err != nil {
		m.windows = m.windows[:len(m.windows)-1]
		*ws = previousWorkspace
		m.activeWindow = previousActive
		delete(m.allocatedWindows, token.window)
		delete(m.allocatedTabs, token.tab)
		delete(m.allocated, token.pane)
		m.nextWindowID, m.nextTabID, m.nextPaneID = token.window, token.tab, token.pane
		return WindowView{}, err
	}
	return m.windowView(&m.windows[len(m.windows)-1]), nil
}

// AbortWindow documents abandonment of an unpublished proposal. It is
// intentionally a no-op so the model remains byte-identical.
func (m *Model) AbortWindow(WindowCreateToken) {}

func (m *Model) CreateWindow(title string) (WindowView, error) {
	token, err := m.PrepareWindow(title)
	if err != nil {
		return WindowView{}, err
	}
	return m.CommitWindow(token)
}

func (m *Model) ActivateWindow(id WindowID) error {
	w := m.windowByID(id)
	if w == nil {
		return ErrWindowNotFound
	}
	if w.workspace != m.activeWorkspace {
		return ErrWindowNotFound
	}
	ws := m.workspaceByID(w.workspace)
	if ws.active != id {
		ws.revision++
	}
	ws.active = id
	m.activeWindow = id
	return nil
}

func (m *Model) RenameWindow(id WindowID, title string) error {
	w := m.windowByID(id)
	if w == nil {
		return ErrWindowNotFound
	}
	w.title = title
	w.revision++
	return nil
}

func (m *Model) CloseWindow(id WindowID) (CloseWindowResult, error) {
	if id == 0 {
		return CloseWindowResult{}, ErrWindowNotFound
	}
	if _, known := m.allocatedWindows[id]; !known {
		return CloseWindowResult{}, ErrWindowNotFound
	}
	index := -1
	for i := range m.windows {
		if m.windows[i].id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return CloseWindowResult{Window: id, ActiveWorkspace: m.activeWorkspace, Active: m.activeWindow, ActiveTab: m.TabID(), FocusedPane: m.FocusedPane(), Empty: len(m.windows) == 0}, nil
	}
	previousWindows := cloneWindowStates(m.windows)
	previousActive := m.activeWindow
	previousActiveWorkspace := m.activeWorkspace
	previousWorkspaces := cloneWorkspaceStates(m.workspaces)
	w := &m.windows[index]
	ws := m.workspaceByID(w.workspace)
	if ws == nil {
		return CloseWindowResult{}, invariantError("window %d has missing workspace %d", id, w.workspace)
	}
	workspaceIndex := -1
	for i, candidate := range ws.windows {
		if candidate == id {
			workspaceIndex = i
			break
		}
	}
	if workspaceIndex < 0 {
		return CloseWindowResult{}, invariantError("workspace %d does not own window %d", ws.id, id)
	}
	result := CloseWindowResult{Window: id, Workspace: w.workspace, Closed: true}
	for i := range w.tabs {
		result.Tabs = append(result.Tabs, w.tabs[i].id)
		result.Panes = append(result.Panes, paneIDs(w.tabs[i].root)...)
		collectSplitIDs(w.tabs[i].root, &result.Splits)
	}
	m.windows = append(m.windows[:index], m.windows[index+1:]...)
	ws.windows = append(ws.windows[:workspaceIndex], ws.windows[workspaceIndex+1:]...)
	if ws.active == id {
		ws.active = 0
		if len(ws.windows) > 0 {
			if workspaceIndex >= len(ws.windows) {
				workspaceIndex = len(ws.windows) - 1
			}
			ws.active = ws.windows[workspaceIndex]
		}
	}
	ws.revision++
	if m.activeWorkspace == ws.id {
		m.activeWindow = ws.active
	}
	if m.activeWorkspace == ws.id && ws.active == 0 && len(m.windows) > 0 {
		start := 0
		for i := range m.workspaces {
			if m.workspaces[i].id == ws.id {
				start = i + 1
				break
			}
		}
		for offset := 0; offset < len(m.workspaces); offset++ {
			candidate := &m.workspaces[(start+offset)%len(m.workspaces)]
			if len(candidate.windows) > 0 {
				candidate.revision++
				m.activeWorkspace, m.activeWindow = candidate.id, candidate.active
				break
			}
		}
	}
	result.ActiveWorkspace = m.activeWorkspace
	result.WorkspaceChanged = previousActiveWorkspace != m.activeWorkspace
	result.Active, result.ActiveTab, result.FocusedPane = m.activeWindow, m.TabID(), m.FocusedPane()
	result.ActiveChanged = previousActive != m.activeWindow
	result.Empty = len(m.windows) == 0
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.activeWindow, m.activeWorkspace, m.workspaces = previousWindows, previousActive, previousActiveWorkspace, previousWorkspaces
		return CloseWindowResult{}, err
	}
	return result, nil
}

func collectSplitIDs(n *node, out *[]SplitID) {
	if n == nil || n.isLeaf() {
		return
	}
	*out = append(*out, n.split)
	collectSplitIDs(n.first, out)
	collectSplitIDs(n.second, out)
}

// rollbackClosedWindow releases a never-published newest window proposal.
// It is valid only immediately after CloseWindow detached that exact candidate.
func (m *Model) rollbackClosedWindow(result CloseWindowResult) error {
	if !result.Closed || len(result.Tabs) != 1 || len(result.Panes) != 1 || len(result.Splits) != 0 {
		return invariantError("window %d is not a rollback candidate", result.Window)
	}
	tab, pane := result.Tabs[0], result.Panes[0]
	if WindowID(m.nextWindowID-1) != result.Window || TabID(m.nextTabID-1) != tab || PaneID(m.nextPaneID-1) != pane {
		return invariantError("window %d rollback is not newest", result.Window)
	}
	delete(m.allocatedWindows, result.Window)
	delete(m.allocatedTabs, tab)
	delete(m.allocated, pane)
	m.nextWindowID, m.nextTabID, m.nextPaneID = result.Window, tab, pane
	return m.CheckInvariants()
}
