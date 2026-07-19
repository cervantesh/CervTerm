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

func tabView(t *tabState, active TabID) TabView {
	return TabView{ID: t.id, Title: t.title, Focused: t.focused, Panes: append([]PaneID(nil), paneIDs(t.root)...), Active: t.id == active, Revision: t.revision}
}

func (m *Model) Tabs() []TabView {
	w := m.activeWindowState()
	if w == nil {
		return nil
	}
	out := make([]TabView, len(w.tabs))
	for i := range w.tabs {
		out[i] = tabView(&w.tabs[i], w.active)
	}
	return out
}

func (m *Model) prepareTab() (TabID, PaneID, error) {
	w := m.activeWindowState()
	if w == nil {
		return 0, 0, ErrEmptyModel
	}
	if len(w.tabs) >= MaxTabs {
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
	w := m.activeWindowState()
	previousActive := w.active
	w.tabs = append(w.tabs, tabState{id: tab, title: title, root: leafNode(pane), focused: pane, revision: 1})
	w.active = tab
	w.revision++
	m.allocatedTabs[tab] = struct{}{}
	m.allocated[pane] = struct{}{}
	m.nextTabID++
	m.nextPaneID++
	if err := m.CheckInvariants(); err != nil {
		w.tabs = w.tabs[:len(w.tabs)-1]
		w.active = previousActive
		w.revision--
		delete(m.allocatedTabs, tab)
		delete(m.allocated, pane)
		m.nextTabID, m.nextPaneID = tab, pane
		return err
	}
	return nil
}

func (m *Model) ActivateTab(id TabID) error {
	w := m.activeWindowState()
	if tabByID(w, id) == nil {
		return ErrTabNotFound
	}
	w.active = id
	return nil
}

func (m *Model) RenameTab(id TabID, title string) error {
	w := m.activeWindowState()
	t := tabByID(w, id)
	if t == nil {
		return ErrTabNotFound
	}
	t.title = title
	t.revision++
	w.revision++
	return nil
}

func (m *Model) MoveTab(id TabID, position int) error {
	w := m.activeWindowState()
	if w == nil || position < 0 || position >= len(w.tabs) {
		return ErrInvalidTabPosition
	}
	from := -1
	for i := range w.tabs {
		if w.tabs[i].id == id {
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
	t := w.tabs[from]
	w.tabs = append(w.tabs[:from], w.tabs[from+1:]...)
	w.tabs = append(w.tabs, tabState{})
	copy(w.tabs[position+1:], w.tabs[position:])
	w.tabs[position] = t
	for i := range w.tabs {
		w.tabs[i].revision++
	}
	w.revision++
	return nil
}

type detachedTab struct {
	id      TabID
	panes   []PaneID
	active  TabID
	focused PaneID
}

func (m *Model) detachTab(id TabID) (detachedTab, error) {
	w := m.activeWindowState()
	if w == nil {
		return detachedTab{}, ErrTabNotFound
	}
	index := -1
	for i := range w.tabs {
		if w.tabs[i].id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return detachedTab{}, ErrTabNotFound
	}
	t := w.tabs[index]
	d := detachedTab{id: id, panes: append([]PaneID(nil), paneIDs(t.root)...)}
	w.tabs = append(w.tabs[:index], w.tabs[index+1:]...)
	if len(w.tabs) == 0 {
		w.active = 0
	} else if w.active == id {
		if index >= len(w.tabs) {
			index = len(w.tabs) - 1
		}
		w.active = w.tabs[index].id
	}
	w.revision++
	d.active = w.active
	if active := tabByID(w, w.active); active != nil {
		d.focused = active.focused
	}
	return d, m.CheckInvariants()
}
