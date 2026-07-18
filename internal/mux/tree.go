package mux

// SetSplitRatio transactionally changes one active branch ratio using uniform
// metrics. The candidate is rejected when any descendant pane would fall below
// the established minimum.
func (m *Model) SetSplitRatio(split SplitID, ratio SplitRatio, bounds PixelRect, metrics CellMetrics) error {
	return m.SetSplitRatioWithMetrics(split, ratio, bounds, UniformCellMetrics(metrics))
}

// SetSplitRatioWithMetrics transactionally changes one active branch ratio
// using metrics resolved per pane.
func (m *Model) SetSplitRatioWithMetrics(split SplitID, ratio SplitRatio, bounds PixelRect, resolve CellMetricsResolver) error {
	if split == 0 {
		return ErrSplitNotFound
	}
	if !validRatio(ratio) {
		return ErrInvalidRatio
	}
	if err := m.CheckInvariants(); err != nil {
		return err
	}
	tab := m.tabForSplit(split)
	if tab == nil || tab.id != m.active {
		return ErrSplitNotFound
	}
	candidate, found := replaceSplitRatio(tab.root, split, ratio)
	if !found {
		return ErrSplitNotFound
	}
	previous := tab.root
	tab.root = candidate
	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		tab.root = previous
		return err
	}
	for _, geometry := range layout.Panes {
		if geometry.Cols < MinPaneCols || geometry.Rows < MinPaneRows {
			tab.root = previous
			return ErrSplitTooSmall
		}
	}
	if err := m.CheckInvariants(); err != nil {
		tab.root = previous
		return err
	}
	return nil
}

func replaceLeaf(n *node, pane PaneID, replacement *node) (*node, bool) {
	if n == nil {
		return nil, false
	}
	if n.isLeaf() {
		if n.pane == pane {
			return replacement, true
		}
		return n, false
	}
	if first, replaced := replaceLeaf(n.first, pane, replacement); replaced {
		return branchNode(n.split, n.axis, n.ratio, first, n.second), true
	}
	if second, replaced := replaceLeaf(n.second, pane, replacement); replaced {
		return branchNode(n.split, n.axis, n.ratio, n.first, second), true
	}
	return n, false
}

func removeLeaf(n *node, pane PaneID) (*node, bool) {
	if n == nil {
		return nil, false
	}
	if n.isLeaf() {
		if n.pane == pane {
			return nil, true
		}
		return n, false
	}
	if first, removed := removeLeaf(n.first, pane); removed {
		if first == nil {
			return n.second, true
		}
		return branchNode(n.split, n.axis, n.ratio, first, n.second), true
	}
	if second, removed := removeLeaf(n.second, pane); removed {
		if second == nil {
			return n.first, true
		}
		return branchNode(n.split, n.axis, n.ratio, n.first, second), true
	}
	return n, false
}

func replaceSplitRatio(n *node, split SplitID, ratio SplitRatio) (*node, bool) {
	if n == nil || n.isLeaf() {
		return n, false
	}
	if n.split == split {
		return branchNode(n.split, n.axis, ratio, n.first, n.second), true
	}
	if first, replaced := replaceSplitRatio(n.first, split, ratio); replaced {
		return branchNode(n.split, n.axis, n.ratio, first, n.second), true
	}
	if second, replaced := replaceSplitRatio(n.second, split, ratio); replaced {
		return branchNode(n.split, n.axis, n.ratio, n.first, second), true
	}
	return n, false
}
