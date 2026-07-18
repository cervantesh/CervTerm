package mux

func checkModelInvariants(m *Model) error {
	if m == nil || m.allocated == nil || m.allocatedSplits == nil || m.allocatedTabs == nil {
		return invariantError("allocation ownership sets are nil")
	}
	if len(m.tabs) > MaxTabs {
		return invariantError("tab count %d exceeds maximum %d", len(m.tabs), MaxTabs)
	}
	if len(m.tabs) == 0 {
		if m.active != 0 {
			return invariantError("empty model has active tab %d", m.active)
		}
		return nil
	}
	if m.active == 0 {
		return invariantError("non-empty model has zero active tab")
	}
	seenTabs := make(map[TabID]struct{}, len(m.tabs))
	seenPanes := make(map[PaneID]TabID)
	seenSplits := make(map[SplitID]TabID)
	activeCount := 0
	for i := range m.tabs {
		tab := &m.tabs[i]
		if tab.id == 0 {
			return invariantError("ordered tab %d has zero ID", i)
		}
		if _, ok := seenTabs[tab.id]; ok {
			return invariantError("tab %d appears more than once", tab.id)
		}
		seenTabs[tab.id] = struct{}{}
		if _, ok := m.allocatedTabs[tab.id]; !ok {
			return invariantError("tab %d was never allocated", tab.id)
		}
		if m.nextTabID != 0 && tab.id >= m.nextTabID {
			return invariantError("tab %d is not below next ID %d", tab.id, m.nextTabID)
		}
		if tab.id == m.active {
			activeCount++
		}
		if tab.root == nil {
			return invariantError("tab %d has empty root", tab.id)
		}
		if tab.focused == 0 {
			return invariantError("tab %d has zero focus", tab.id)
		}
		if tab.revision == 0 {
			return invariantError("tab %d has zero revision", tab.id)
		}
		seenNodes := make(map[*node]struct{})
		focused := 0
		var visit func(*node) error
		visit = func(n *node) error {
			if n == nil {
				return invariantError("tab %d tree contains nil child", tab.id)
			}
			if _, ok := seenNodes[n]; ok {
				return invariantError("tab %d tree contains cycle or shared node", tab.id)
			}
			seenNodes[n] = struct{}{}
			if n.isLeaf() {
				if n.first != nil || n.second != nil || n.split != 0 || n.axis != 0 || n.ratio != 0 {
					return invariantError("pane %d leaf carries split state", n.pane)
				}
				if owner, ok := seenPanes[n.pane]; ok {
					return invariantError("pane %d belongs to tabs %d and %d", n.pane, owner, tab.id)
				}
				if _, ok := m.allocated[n.pane]; !ok {
					return invariantError("pane %d was never allocated", n.pane)
				}
				if m.nextPaneID != 0 && n.pane >= m.nextPaneID {
					return invariantError("pane %d is not below next ID %d", n.pane, m.nextPaneID)
				}
				seenPanes[n.pane] = tab.id
				if n.pane == tab.focused {
					focused++
				}
				return nil
			}
			if n.pane != 0 || n.split == 0 || !validAxis(n.axis) || !validRatio(n.ratio) {
				return invariantError("split %d has invalid branch state", n.split)
			}
			if owner, ok := seenSplits[n.split]; ok {
				return invariantError("split %d belongs to tabs %d and %d", n.split, owner, tab.id)
			}
			if _, ok := m.allocatedSplits[n.split]; !ok {
				return invariantError("split %d was never allocated", n.split)
			}
			if m.nextSplitID != 0 && n.split >= m.nextSplitID {
				return invariantError("split %d is not below next ID %d", n.split, m.nextSplitID)
			}
			seenSplits[n.split] = tab.id
			if err := visit(n.first); err != nil {
				return err
			}
			return visit(n.second)
		}
		if err := visit(tab.root); err != nil {
			return err
		}
		if focused != 1 {
			return invariantError("tab %d expected one focused leaf, found %d", tab.id, focused)
		}
	}
	if activeCount != 1 {
		return invariantError("expected one active tab, found %d", activeCount)
	}
	return nil
}
