package mux

// PaneID is a stable, process-local pane identity. Zero is never valid.
type PaneID uint64

// SplitID is a stable, process-local identity for one branch divider.
type SplitID uint64

// TabID is a stable, process-local tab identity. Phase 1 creates one implicit
// tab while keeping the identity explicit for later phases.
type TabID uint64

// SplitAxis describes the arrangement of a split's children.
type SplitAxis uint8

const (
	SplitColumns SplitAxis = iota + 1
	SplitRows
)

// Direction selects a geometric focus movement.
type Direction uint8

const (
	FocusLeft Direction = iota + 1
	FocusRight
	FocusUp
	FocusDown
)

type node struct {
	pane   PaneID
	split  SplitID
	axis   SplitAxis
	ratio  SplitRatio
	first  *node
	second *node
}

func leafNode(pane PaneID) *node { return &node{pane: pane} }

func branchNode(split SplitID, axis SplitAxis, ratio SplitRatio, first, second *node) *node {
	return &node{split: split, axis: axis, ratio: ratio, first: first, second: second}
}

func (n *node) isLeaf() bool { return n != nil && n.pane != 0 }

const MaxTabs = 256

type tabState struct {
	id       TabID
	title    string
	root     *node
	focused  PaneID
	revision uint64
}

// Model owns pure identity, ordered tab topology, and focus state.
type Model struct {
	tabs            []tabState
	active          TabID
	nextTabID       TabID
	nextPaneID      PaneID
	nextSplitID     SplitID
	allocated       map[PaneID]struct{}
	allocatedSplits map[SplitID]struct{}
	allocatedTabs   map[TabID]struct{}
}

// NewModel creates one active tab containing one focused root leaf.
func NewModel() *Model {
	pane, tab := PaneID(1), TabID(1)
	return &Model{
		tabs: []tabState{{id: tab, root: leafNode(pane), focused: pane, revision: 1}}, active: tab,
		nextTabID: 2, nextPaneID: 2, nextSplitID: 1,
		allocated:       map[PaneID]struct{}{pane: {}},
		allocatedSplits: map[SplitID]struct{}{}, allocatedTabs: map[TabID]struct{}{tab: {}},
	}
}

func (m *Model) activeTab() *tabState { return m.tabByID(m.active) }
func (m *Model) tabByID(id TabID) *tabState {
	for i := range m.tabs {
		if m.tabs[i].id == id {
			return &m.tabs[i]
		}
	}
	return nil
}
func (m *Model) tabForPane(id PaneID) *tabState {
	for i := range m.tabs {
		if findLeaf(m.tabs[i].root, id) != nil {
			return &m.tabs[i]
		}
	}
	return nil
}
func (m *Model) tabForSplit(id SplitID) *tabState {
	for i := range m.tabs {
		if findSplit(m.tabs[i].root, id) != nil {
			return &m.tabs[i]
		}
	}
	return nil
}
func (m *Model) TabID() TabID { return m.active }
func (m *Model) FocusedPane() PaneID {
	if t := m.activeTab(); t != nil {
		return t.focused
	}
	return 0
}
func (m *Model) Empty() bool { return len(m.tabs) == 0 }
func (m *Model) PaneIDs() []PaneID {
	if t := m.activeTab(); t != nil {
		return paneIDs(t.root)
	}
	return nil
}
func validAxis(axis SplitAxis) bool    { return axis == SplitColumns || axis == SplitRows }
func validRatio(ratio SplitRatio) bool { return ratio > 0 && ratio < RatioScale }
func validDirection(d Direction) bool  { return d >= FocusLeft && d <= FocusDown }
func (m *Model) paneExists(id PaneID) bool {
	t := m.activeTab()
	return t != nil && findLeaf(t.root, id) != nil
}

// Split adds a new second child at the default 50/50 ratio and focuses it.
func (m *Model) Split(pane PaneID, axis SplitAxis, bounds PixelRect, metrics CellMetrics) (PaneID, error) {
	return m.SplitWithRatio(pane, axis, DefaultSplitRatio, bounds, metrics)
}

// SplitWithRatio adds a new second child using uniform metrics.
func (m *Model) SplitWithRatio(pane PaneID, axis SplitAxis, ratio SplitRatio, bounds PixelRect, metrics CellMetrics) (PaneID, error) {
	if err := validateGeometry(bounds, metrics); err != nil {
		return 0, err
	}
	return m.SplitWithRatioAndMetrics(pane, axis, ratio, bounds, UniformCellMetrics(metrics))
}

// SplitWithMetrics adds a new second child using metrics resolved per pane.
func (m *Model) SplitWithMetrics(pane PaneID, axis SplitAxis, bounds PixelRect, resolve CellMetricsResolver) (PaneID, error) {
	return m.SplitWithRatioAndMetrics(pane, axis, DefaultSplitRatio, bounds, resolve)
}

// SplitWithRatioAndMetrics adds a new second child if both resulting leaves
// meet the minimum cell geometry. The resolver must include the next pane ID.
// Every rejection leaves topology, focus, and ID allocation unchanged.
func (m *Model) SplitWithRatioAndMetrics(pane PaneID, axis SplitAxis, ratio SplitRatio, bounds PixelRect, resolve CellMetricsResolver) (PaneID, error) {
	if !validAxis(axis) {
		return 0, ErrInvalidAxis
	}
	if !validRatio(ratio) {
		return 0, ErrInvalidRatio
	}
	if err := m.CheckInvariants(); err != nil {
		return 0, err
	}
	tab := m.activeTab()
	if tab == nil {
		return 0, ErrEmptyModel
	}
	if m.nextPaneID == 0 {
		return 0, ErrIDExhausted
	}
	if m.nextSplitID == 0 {
		return 0, ErrIDExhausted
	}

	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		return 0, err
	}
	var target PaneGeometry
	found := false
	for _, geometry := range layout.Panes {
		if geometry.Pane == pane {
			target = geometry
			found = true
			break
		}
	}
	if !found {
		return 0, ErrPaneNotFound
	}

	firstMetrics, ok := resolve(pane)
	if !ok {
		return 0, ErrPaneNotFound
	}
	if err := validateCellMetrics(firstMetrics); err != nil {
		return 0, err
	}
	secondMetrics, ok := resolve(m.nextPaneID)
	if !ok {
		return 0, ErrPaneNotFound
	}
	if err := validateCellMetrics(secondMetrics); err != nil {
		return 0, err
	}
	firstRect, _, secondRect := splitPixelRect(target.Pixels, axis, ratio)
	firstCols, firstRows := cellGeometry(firstRect, firstMetrics)
	secondCols, secondRows := cellGeometry(secondRect, secondMetrics)
	if firstCols < MinPaneCols || firstRows < MinPaneRows || secondCols < MinPaneCols || secondRows < MinPaneRows {
		return 0, ErrSplitTooSmall
	}

	newPane := m.nextPaneID
	newSplit := m.nextSplitID
	replacement := branchNode(newSplit, axis, ratio, leafNode(pane), leafNode(newPane))
	newRoot, replaced := replaceLeaf(tab.root, pane, replacement)
	if !replaced {
		return 0, invariantError("split target %d disappeared", pane)
	}
	tab.root = newRoot
	tab.focused = newPane
	tab.revision++
	m.allocated[newPane] = struct{}{}
	m.allocatedSplits[newSplit] = struct{}{}
	m.nextPaneID++
	m.nextSplitID++
	return newPane, nil
}

// Focus selects an active pane explicitly.
func (m *Model) Focus(pane PaneID) error {
	if !m.paneExists(pane) {
		return ErrPaneNotFound
	}
	m.activeTab().focused = pane
	return nil
}

// FocusNext moves to the next pane in visual depth-first order and wraps.
func (m *Model) FocusNext() (PaneID, error) {
	ids := m.PaneIDs()
	if len(ids) == 0 {
		return 0, ErrEmptyModel
	}
	tab := m.activeTab()
	for i, id := range ids {
		if id == tab.focused {
			tab.focused = ids[(i+1)%len(ids)]
			return tab.focused, nil
		}
	}
	return 0, invariantError("focused pane %d is not active", tab.focused)
}

type focusScore struct {
	overlap   int
	primary   int
	secondary int
}

// FocusDirection moves to the nearest pane whose rectangle lies in the
// requested direction using uniform metrics.
func (m *Model) FocusDirection(direction Direction, bounds PixelRect, metrics CellMetrics) (PaneID, error) {
	if err := validateGeometry(bounds, metrics); err != nil {
		return 0, err
	}
	return m.FocusDirectionWithMetrics(direction, bounds, UniformCellMetrics(metrics))
}

// FocusDirectionWithMetrics moves focus using a per-pane metric projection.
func (m *Model) FocusDirectionWithMetrics(direction Direction, bounds PixelRect, resolve CellMetricsResolver) (PaneID, error) {
	if !validDirection(direction) {
		return 0, ErrInvalidDirection
	}
	tab := m.activeTab()
	if tab == nil {
		return 0, ErrEmptyModel
	}
	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		return 0, err
	}
	var source PaneGeometry
	found := false
	for _, geometry := range layout.Panes {
		if geometry.Pane == tab.focused {
			source, found = geometry, true
			break
		}
	}
	if !found {
		return 0, invariantError("focused pane %d has no geometry", tab.focused)
	}

	bestIndex := -1
	var best focusScore
	for i, candidate := range layout.Panes {
		if candidate.Pane == source.Pane {
			continue
		}
		candidateScore, eligible := directionalScore(direction, source.Pixels, candidate.Pixels)
		if !eligible {
			continue
		}
		if bestIndex < 0 || candidateScore.overlap < best.overlap ||
			candidateScore.overlap == best.overlap && candidateScore.primary < best.primary ||
			candidateScore.overlap == best.overlap && candidateScore.primary == best.primary && candidateScore.secondary < best.secondary ||
			candidateScore == best && candidate.Pane < layout.Panes[bestIndex].Pane {
			bestIndex = i
			best = candidateScore
		}
	}
	if bestIndex < 0 {
		return 0, ErrNoPaneInDirection
	}
	tab.focused = layout.Panes[bestIndex].Pane
	return tab.focused, nil
}

func directionalScore(direction Direction, source, candidate PixelRect) (focusScore, bool) {
	var primary int
	var sourceStart, sourceEnd, candidateStart, candidateEnd int
	switch direction {
	case FocusLeft:
		if candidate.Right() > source.X {
			return focusScore{}, false
		}
		primary = source.X - candidate.Right()
		sourceStart, sourceEnd = source.Y, source.Bottom()
		candidateStart, candidateEnd = candidate.Y, candidate.Bottom()
	case FocusRight:
		if candidate.X < source.Right() {
			return focusScore{}, false
		}
		primary = candidate.X - source.Right()
		sourceStart, sourceEnd = source.Y, source.Bottom()
		candidateStart, candidateEnd = candidate.Y, candidate.Bottom()
	case FocusUp:
		if candidate.Bottom() > source.Y {
			return focusScore{}, false
		}
		primary = source.Y - candidate.Bottom()
		sourceStart, sourceEnd = source.X, source.Right()
		candidateStart, candidateEnd = candidate.X, candidate.Right()
	case FocusDown:
		if candidate.Y < source.Bottom() {
			return focusScore{}, false
		}
		primary = candidate.Y - source.Bottom()
		sourceStart, sourceEnd = source.X, source.Right()
		candidateStart, candidateEnd = candidate.X, candidate.Right()
	default:
		return focusScore{}, false
	}

	overlap := 0
	if candidateEnd <= sourceStart || candidateStart >= sourceEnd {
		overlap = 1
	}
	secondary := absInt((sourceStart + sourceEnd) - (candidateStart + candidateEnd))
	return focusScore{overlap: overlap, primary: primary, secondary: secondary}, true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// CloseResult describes a topology close transition. Closed is false for an
// idempotent repeated close of an ID that this model previously allocated.
type CloseResult struct {
	Pane      PaneID
	Focused   PaneID
	Closed    bool
	Empty     bool
	Tab       TabID
	TabClosed bool
}

// Close removes one leaf, collapses its parent split, and reports final-empty.
// Re-closing a previously allocated pane is an idempotent no-op.
func (m *Model) Close(pane PaneID) (CloseResult, error) {
	if pane == 0 {
		return CloseResult{}, ErrPaneNotFound
	}
	if _, known := m.allocated[pane]; !known {
		return CloseResult{}, ErrPaneNotFound
	}
	tab := m.tabForPane(pane)
	if tab == nil {
		return CloseResult{Pane: pane, Focused: m.FocusedPane(), Empty: len(m.tabs) == 0}, nil
	}
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
		for i := range m.tabs {
			if m.tabs[i].id == closedTab {
				index = i
				m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
				break
			}
		}
		if len(m.tabs) == 0 {
			m.active = 0
		} else if m.active == closedTab {
			if index >= len(m.tabs) {
				index = len(m.tabs) - 1
			}
			m.active = m.tabs[index].id
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
	return CloseResult{Pane: pane, Tab: closedTab, TabClosed: tabClosed, Focused: m.FocusedPane(), Closed: true, Empty: len(m.tabs) == 0}, nil
}

// CheckInvariants verifies ordered tab, ownership, tree, and monotonic ID state.
func (m *Model) CheckInvariants() error { return checkModelInvariants(m) }

func findLeaf(n *node, pane PaneID) *node {
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		if n.pane == pane {
			return n
		}
		return nil
	}
	if found := findLeaf(n.first, pane); found != nil {
		return found
	}
	return findLeaf(n.second, pane)

}
func findSplit(n *node, split SplitID) *node {
	if n == nil || n.isLeaf() {
		return nil
	}
	if n.split == split {
		return n
	}
	if found := findSplit(n.first, split); found != nil {
		return found
	}
	return findSplit(n.second, split)
}

func paneIDs(root *node) []PaneID {
	var ids []PaneID
	var visit func(*node)
	visit = func(n *node) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			ids = append(ids, n.pane)
			return
		}
		visit(n.first)
		visit(n.second)
	}
	visit(root)
	return ids
}

func firstPane(n *node) PaneID {
	for n != nil && !n.isLeaf() {
		n = n.first
	}
	if n == nil {
		return 0
	}
	return n.pane
}
