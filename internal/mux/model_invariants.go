package mux

func checkModelInvariants(m *Model) error {
	if m == nil || m.allocatedWindows == nil || m.allocated == nil || m.allocatedSplits == nil || m.allocatedTabs == nil {
		return invariantError("allocation ownership sets are nil")
	}
	if len(m.windows) > MaxWindows {
		return invariantError("window count %d exceeds maximum %d", len(m.windows), MaxWindows)
	}
	if len(m.windows) == 0 {
		if m.activeWindow != 0 {
			return invariantError("empty model has active window %d", m.activeWindow)
		}
		return nil
	}
	if m.activeWindow == 0 {
		return invariantError("non-empty model has zero active window")
	}
	seenWindows := make(map[WindowID]struct{}, len(m.windows))
	seenTabs := make(map[TabID]WindowID)
	seenPanes := make(map[PaneID]TabID)
	seenSplits := make(map[SplitID]TabID)
	seenNodes := make(map[*node]TabID)
	activeWindows := 0
	for wi := range m.windows {
		w := &m.windows[wi]
		if w.id == 0 {
			return invariantError("ordered window %d has zero ID", wi)
		}
		if _, ok := seenWindows[w.id]; ok {
			return invariantError("window %d appears more than once", w.id)
		}
		seenWindows[w.id] = struct{}{}
		if _, ok := m.allocatedWindows[w.id]; !ok {
			return invariantError("window %d was never allocated", w.id)
		}
		if m.nextWindowID != 0 && w.id >= m.nextWindowID {
			return invariantError("window %d is not below next ID %d", w.id, m.nextWindowID)
		}
		if w.revision == 0 {
			return invariantError("window %d has zero revision", w.id)
		}
		if w.id == m.activeWindow {
			activeWindows++
		}
		if len(w.tabs) > MaxTabs {
			return invariantError("window %d tab count %d exceeds maximum %d", w.id, len(w.tabs), MaxTabs)
		}
		if len(w.tabs) == 0 {
			if w.active != 0 {
				return invariantError("empty window %d has active tab %d", w.id, w.active)
			}
			continue
		}
		if w.active == 0 {
			return invariantError("non-empty window %d has zero active tab", w.id)
		}
		activeTabs := 0
		for ti := range w.tabs {
			tab := &w.tabs[ti]
			if tab.id == 0 {
				return invariantError("window %d tab %d has zero ID", w.id, ti)
			}
			if owner, ok := seenTabs[tab.id]; ok {
				return invariantError("tab %d appears more than once (windows %d and %d)", tab.id, owner, w.id)
			}
			seenTabs[tab.id] = w.id
			if _, ok := m.allocatedTabs[tab.id]; !ok {
				return invariantError("tab %d was never allocated", tab.id)
			}
			if m.nextTabID != 0 && tab.id >= m.nextTabID {
				return invariantError("tab %d is not below next ID %d", tab.id, m.nextTabID)
			}
			if tab.id == w.active {
				activeTabs++
			}
			if tab.root == nil || tab.focused == 0 || tab.revision == 0 {
				return invariantError("tab %d has invalid root, focus, or revision", tab.id)
			}
			focused := 0
			var visit func(*node) error
			visit = func(n *node) error {
				if n == nil {
					return invariantError("tab %d tree contains nil child", tab.id)
				}
				if owner, ok := seenNodes[n]; ok {
					return invariantError("tabs %d and %d contain a cycle or shared node", owner, tab.id)
				}
				seenNodes[n] = tab.id
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
		if activeTabs != 1 {
			return invariantError("window %d expected one active tab, found %d", w.id, activeTabs)
		}
	}
	if activeWindows != 1 {
		return invariantError("expected one active window, found %d", activeWindows)
	}
	return nil
}
