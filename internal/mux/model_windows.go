package mux

// WindowView is a detached immutable projection of mux-owned window state.
type WindowView struct {
	ID       WindowID
	Title    string
	Tabs     []TabView
	Active   bool
	Revision uint64
}

// CloseWindowResult reports detached ownership without creating or closing sessions.
type CloseWindowResult struct {
	Window      WindowID
	Tabs        []TabID
	Panes       []PaneID
	Splits      []SplitID
	Active      WindowID
	ActiveTab   TabID
	FocusedPane PaneID
	Closed      bool
	Empty       bool
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
	return WindowView{ID: w.id, Title: w.title, Tabs: tabs, Active: w.id == m.activeWindow, Revision: w.revision}
}

func (m *Model) CreateWindow(title string) (WindowView, error) {
	if err := m.CheckInvariants(); err != nil {
		return WindowView{}, err
	}
	if len(m.windows) >= MaxWindows {
		return WindowView{}, ErrWindowLimitReached
	}
	if m.nextWindowID == 0 || m.nextTabID == 0 || m.nextPaneID == 0 {
		return WindowView{}, ErrIDExhausted
	}
	window, tab, pane := m.nextWindowID, m.nextTabID, m.nextPaneID
	previousActive := m.activeWindow
	m.windows = append(m.windows, windowState{
		id: window, title: title, active: tab, revision: 1,
		tabs: []tabState{{id: tab, root: leafNode(pane), focused: pane, revision: 1}},
	})
	m.activeWindow = window
	m.allocatedWindows[window] = struct{}{}
	m.allocatedTabs[tab] = struct{}{}
	m.allocated[pane] = struct{}{}
	m.nextWindowID++
	m.nextTabID++
	m.nextPaneID++
	if err := m.CheckInvariants(); err != nil {
		m.windows = m.windows[:len(m.windows)-1]
		m.activeWindow = previousActive
		delete(m.allocatedWindows, window)
		delete(m.allocatedTabs, tab)
		delete(m.allocated, pane)
		m.nextWindowID, m.nextTabID, m.nextPaneID = window, tab, pane
		return WindowView{}, err
	}
	return m.windowView(&m.windows[len(m.windows)-1]), nil
}

func (m *Model) ActivateWindow(id WindowID) error {
	if m.windowByID(id) == nil {
		return ErrWindowNotFound
	}
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
		return CloseWindowResult{Window: id, Active: m.activeWindow, ActiveTab: m.TabID(), FocusedPane: m.FocusedPane(), Empty: len(m.windows) == 0}, nil
	}
	previousWindows := cloneWindowStates(m.windows)
	previousActive := m.activeWindow
	w := &m.windows[index]
	result := CloseWindowResult{Window: id, Closed: true}
	for i := range w.tabs {
		result.Tabs = append(result.Tabs, w.tabs[i].id)
		result.Panes = append(result.Panes, paneIDs(w.tabs[i].root)...)
		collectSplitIDs(w.tabs[i].root, &result.Splits)
	}
	m.windows = append(m.windows[:index], m.windows[index+1:]...)
	if len(m.windows) == 0 {
		m.activeWindow = 0
	} else if m.activeWindow == id {
		if index >= len(m.windows) {
			index = len(m.windows) - 1
		}
		m.activeWindow = m.windows[index].id
	}
	result.Active, result.ActiveTab, result.FocusedPane = m.activeWindow, m.TabID(), m.FocusedPane()
	result.Empty = len(m.windows) == 0
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.activeWindow = previousWindows, previousActive
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
