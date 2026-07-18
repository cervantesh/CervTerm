package mux

// TabView is a detached immutable projection of mux-owned tab state.
type TabView struct {
	ID       TabID
	Title    string
	Focused  PaneID
	Panes    []PaneID
	Active   bool
	Revision uint64
}

func (m *Model) Tabs() []TabView {
	out := make([]TabView, len(m.tabs))
	for i := range m.tabs {
		t := &m.tabs[i]
		out[i] = TabView{ID: t.id, Title: t.title, Focused: t.focused, Panes: append([]PaneID(nil), paneIDs(t.root)...), Active: t.id == m.active, Revision: t.revision}
	}
	return out
}

func (m *Model) prepareTab() (TabID, PaneID, error) {
	if len(m.tabs) >= MaxTabs {
		return 0, 0, ErrTabLimitReached
	}
	if m.nextTabID == 0 || m.nextPaneID == 0 {
		return 0, 0, ErrIDExhausted
	}
	return m.nextTabID, m.nextPaneID, nil
}

func (m *Model) commitTab(tab TabID, pane PaneID, title string) error {
	predictedTab, predictedPane, err := m.prepareTab()
	if err != nil {
		return err
	}
	if tab != predictedTab || pane != predictedPane {
		return invariantError("tab commit IDs changed: predicted %d/%d, got %d/%d", predictedTab, predictedPane, tab, pane)
	}
	previousActive := m.active
	m.tabs = append(m.tabs, tabState{id: tab, title: title, root: leafNode(pane), focused: pane, revision: 1})
	m.active = tab
	m.allocatedTabs[tab] = struct{}{}
	m.allocated[pane] = struct{}{}
	m.nextTabID++
	m.nextPaneID++
	if err := m.CheckInvariants(); err != nil {
		m.tabs = m.tabs[:len(m.tabs)-1]
		m.active = previousActive
		delete(m.allocatedTabs, tab)
		delete(m.allocated, pane)
		m.nextTabID, m.nextPaneID = tab, pane
		return err
	}
	return nil
}

func (m *Model) ActivateTab(id TabID) error {
	if m.tabByID(id) == nil {
		return ErrTabNotFound
	}
	m.active = id
	return nil
}

func (m *Model) RenameTab(id TabID, title string) error {
	t := m.tabByID(id)
	if t == nil {
		return ErrTabNotFound
	}
	t.title = title
	t.revision++
	return nil
}

func (m *Model) MoveTab(id TabID, position int) error {
	if position < 0 || position >= len(m.tabs) {
		return ErrInvalidTabPosition
	}
	from := -1
	for i := range m.tabs {
		if m.tabs[i].id == id {
			from = i
			break
		}
	}
	if from < 0 {
		return ErrTabNotFound
	}
	if from == position {
		return nil
	}
	t := m.tabs[from]
	m.tabs = append(m.tabs[:from], m.tabs[from+1:]...)
	m.tabs = append(m.tabs, tabState{})
	copy(m.tabs[position+1:], m.tabs[position:])
	m.tabs[position] = t
	for i := range m.tabs {
		m.tabs[i].revision++
	}
	return nil
}

type detachedTab struct {
	id      TabID
	panes   []PaneID
	active  TabID
	focused PaneID
}

func (m *Model) detachTab(id TabID) (detachedTab, error) {
	index := -1
	for i := range m.tabs {
		if m.tabs[i].id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return detachedTab{}, ErrTabNotFound
	}
	t := m.tabs[index]
	d := detachedTab{id: id, panes: append([]PaneID(nil), paneIDs(t.root)...)}
	m.tabs = append(m.tabs[:index], m.tabs[index+1:]...)
	if len(m.tabs) == 0 {
		m.active = 0
	} else if m.active == id {
		if index >= len(m.tabs) {
			index = len(m.tabs) - 1
		}
		m.active = m.tabs[index].id
	}
	d.active = m.active
	if active := m.activeTab(); active != nil {
		d.focused = active.focused
	}
	return d, m.CheckInvariants()
}
