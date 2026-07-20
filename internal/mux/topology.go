package mux

// TopologyMinPaneCols and TopologyMinPaneRows are the hard geometry floor for
// mutations of an existing split tree. They are intentionally lower than the
// usability minimum applied when creating a new split.
const (
	TopologyMinPaneCols = 2
	TopologyMinPaneRows = 2
)

// DirectionalNeighbor returns the pane selected by the same deterministic
// geometry rule used by directional focus, without changing focus.
func (m *Model) DirectionalNeighbor(pane PaneID, direction Direction, bounds PixelRect, resolve CellMetricsResolver) (PaneID, error) {
	if !validDirection(direction) {
		return 0, ErrInvalidDirection
	}
	if !m.paneExists(pane) {
		return 0, ErrPaneNotFound
	}
	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		return 0, err
	}
	return directionalNeighbor(layout, pane, direction)
}

func directionalNeighbor(layout Layout, pane PaneID, direction Direction) (PaneID, error) {
	var source PaneGeometry
	found := false
	for _, geometry := range layout.Panes {
		if geometry.Pane == pane {
			source, found = geometry, true
			break
		}
	}
	if !found {
		return 0, ErrPaneNotFound
	}
	bestIndex := -1
	var best focusScore
	for i, candidate := range layout.Panes {
		if candidate.Pane == pane {
			continue
		}
		score, eligible := directionalScore(direction, source.Pixels, candidate.Pixels)
		if !eligible {
			continue
		}
		if bestIndex < 0 || score.overlap < best.overlap ||
			score.overlap == best.overlap && score.primary < best.primary ||
			score.overlap == best.overlap && score.primary == best.primary && score.secondary < best.secondary ||
			score == best && candidate.Pane < layout.Panes[bestIndex].Pane {
			bestIndex, best = i, score
		}
	}
	if bestIndex < 0 {
		return 0, ErrNoPaneInDirection
	}
	return layout.Panes[bestIndex].Pane, nil
}

// ResizePaneDirection transactionally grows the pane toward direction by delta
// cells. A rejected candidate leaves topology and focus byte-for-byte unchanged.
func (m *Model) ResizePaneDirection(pane PaneID, direction Direction, delta int, bounds PixelRect, resolve CellMetricsResolver) error {
	if delta <= 0 {
		return ErrInvalidResizeDelta
	}
	neighbor, err := m.DirectionalNeighbor(pane, direction, bounds, resolve)
	if err != nil {
		return err
	}
	axis := SplitColumns
	if direction == FocusUp || direction == FocusDown {
		axis = SplitRows
	}
	path, ok := separatingSplit(m.activeTab().root, pane, neighbor, axis, bounds)
	if !ok {
		return ErrNoPaneInDirection
	}
	metrics, ok := resolve(pane)
	if !ok {
		return ErrPaneNotFound
	}
	cellPixels := metrics.CellWidth
	containerPixels := path.container.Width - DividerPixels
	if axis == SplitRows {
		cellPixels = metrics.CellHeight
		containerPixels = path.container.Height - DividerPixels
	}
	if containerPixels <= 0 {
		return ErrTopologyTooSmall
	}
	change := SplitRatio((delta*cellPixels*RatioScale + containerPixels - 1) / containerPixels)
	if change <= 0 {
		change = 1
	}
	ratio := path.node.ratio
	growFirst := path.paneInFirst && (direction == FocusRight || direction == FocusDown)
	if !path.paneInFirst && (direction == FocusLeft || direction == FocusUp) {
		growFirst = false
	} else if path.paneInFirst != (direction == FocusRight || direction == FocusDown) {
		return ErrNoPaneInDirection
	}
	if growFirst {
		ratio += change
	} else {
		ratio -= change
	}
	tab := m.activeTab()
	candidate, replaced := replaceSplitRatio(tab.root, path.node.split, ratio)
	if !replaced || !validRatio(ratio) {
		return ErrTopologyTooSmall
	}
	return m.commitTopology(candidate, tab.focused, bounds, resolve)
}

// SwapPaneDirection exchanges the focused pane identity with its directional
// neighbor and transfers focus to that neighbor, preserving the focused slot.
func (m *Model) SwapPaneDirection(pane PaneID, direction Direction, bounds PixelRect, resolve CellMetricsResolver) (PaneID, error) {
	neighbor, err := m.DirectionalNeighbor(pane, direction, bounds, resolve)
	if err != nil {
		return 0, err
	}
	tab := m.activeTab()
	candidate, ok := swapLeaves(tab.root, pane, neighbor)
	if !ok {
		return 0, invariantError("could not swap panes %d and %d", pane, neighbor)
	}
	focus := tab.focused
	if focus == pane {
		focus = neighbor
	} else if focus == neighbor {
		focus = pane
	}
	if err := m.commitTopology(candidate, focus, bounds, resolve); err != nil {
		return 0, err
	}
	return neighbor, nil
}

// MovePaneDirection reorders the pane with its directional neighbor while focus
// follows the moved pane identity.
func (m *Model) MovePaneDirection(pane PaneID, direction Direction, bounds PixelRect, resolve CellMetricsResolver) error {
	neighbor, err := m.DirectionalNeighbor(pane, direction, bounds, resolve)
	if err != nil {
		return err
	}
	tab := m.activeTab()
	candidate, ok := swapLeaves(tab.root, pane, neighbor)
	if !ok {
		return invariantError("could not move pane %d toward %d", pane, neighbor)
	}
	return m.commitTopology(candidate, tab.focused, bounds, resolve)
}

func (m *Model) commitTopology(candidate *node, focus PaneID, bounds PixelRect, resolve CellMetricsResolver) error {
	tab := m.activeTab()
	previousRoot, previousFocus := tab.root, tab.focused
	tab.root, tab.focused = candidate, focus
	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err == nil {
		for _, geometry := range layout.Panes {
			if geometry.Cols < TopologyMinPaneCols || geometry.Rows < TopologyMinPaneRows {
				err = ErrTopologyTooSmall
				break
			}
		}
	}
	if err == nil {
		err = m.CheckInvariants()
	}
	if err != nil {
		tab.root, tab.focused = previousRoot, previousFocus
	}
	return err
}

type splitPath struct {
	node        *node
	container   PixelRect
	paneInFirst bool
}

func separatingSplit(root *node, pane, neighbor PaneID, axis SplitAxis, bounds PixelRect) (splitPath, bool) {
	var visit func(*node, PixelRect) (splitPath, bool)
	visit = func(n *node, rect PixelRect) (splitPath, bool) {
		if n == nil || n.isLeaf() {
			return splitPath{}, false
		}
		firstHasPane, firstHasNeighbor := findLeaf(n.first, pane) != nil, findLeaf(n.first, neighbor) != nil
		secondHasPane, secondHasNeighbor := findLeaf(n.second, pane) != nil, findLeaf(n.second, neighbor) != nil
		if n.axis == axis && ((firstHasPane && secondHasNeighbor) || (secondHasPane && firstHasNeighbor)) {
			return splitPath{node: n, container: rect, paneInFirst: firstHasPane}, true
		}
		firstRect, _, secondRect := splitPixelRect(rect, n.axis, n.ratio)
		if firstHasPane && firstHasNeighbor {
			return visit(n.first, firstRect)
		}
		if secondHasPane && secondHasNeighbor {
			return visit(n.second, secondRect)
		}
		return splitPath{}, false
	}
	return visit(root, bounds)
}

func swapLeaves(root *node, first, second PaneID) (*node, bool) {
	if findLeaf(root, first) == nil || findLeaf(root, second) == nil || first == second {
		return root, false
	}
	var clone func(*node) *node
	clone = func(n *node) *node {
		if n.isLeaf() {
			switch n.pane {
			case first:
				return leafNode(second)
			case second:
				return leafNode(first)
			default:
				return leafNode(n.pane)
			}
		}
		return branchNode(n.split, n.axis, n.ratio, clone(n.first), clone(n.second))
	}
	return clone(root), true
}
