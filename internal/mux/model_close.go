package mux

// CloseResult describes one pane topology close transition.
type CloseResult struct {
	Pane      PaneID
	Focused   PaneID
	Closed    bool
	Empty     bool
	Tab       TabID
	TabClosed bool
}

// Close removes one leaf, collapses its parent split, and reports window-empty.
func (m *Model) Close(pane PaneID) (CloseResult, error) {
	if pane == 0 {
		return CloseResult{}, ErrPaneNotFound
	}
	if _, known := m.allocated[pane]; !known {
		return CloseResult{}, ErrPaneNotFound
	}
	tab := m.tabForPane(pane)
	if tab == nil {
		return CloseResult{Pane: pane, Focused: m.FocusedPane(), Empty: m.Empty()}, nil
	}
	window := m.windowForTab(tab.id)
	visualOrder := paneIDs(tab.root)
	closedIndex := 0
	for i, id := range visualOrder {
		if id == pane {
			closedIndex = i
			break
		}
	}
	newRoot, removed := removeLeaf(tab.root, pane)
	if !removed {
		return CloseResult{}, invariantError("active pane %d could not be removed", pane)
	}
	tab.root = newRoot
	closedTab := tab.id
	tabClosed := newRoot == nil
	if tabClosed {
		index := 0
		for i := range window.tabs {
			if window.tabs[i].id == closedTab {
				index = i
				window.tabs = append(window.tabs[:i], window.tabs[i+1:]...)
				break
			}
		}
		if len(window.tabs) == 0 {
			window.active = 0
		} else if window.active == closedTab {
			if index >= len(window.tabs) {
				index = len(window.tabs) - 1
			}
			window.active = window.tabs[index].id
		}
	} else if tab.focused == pane {
		remaining := paneIDs(tab.root)
		if closedIndex >= len(remaining) {
			closedIndex = len(remaining) - 1
		}
		tab.focused = remaining[closedIndex]
	}
	if !tabClosed {
		tab.revision++
	}
	window.revision++
	return CloseResult{Pane: pane, Tab: closedTab, TabClosed: tabClosed, Focused: m.FocusedPane(), Closed: true, Empty: len(window.tabs) == 0}, nil
}
