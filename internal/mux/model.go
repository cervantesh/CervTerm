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

// Model owns the pure identity, topology, and focus state of the implicit tab.
// It has no terminal, PTY, renderer, or frontend dependencies.
type Model struct {
	tabID       TabID
	nextTabID   TabID
	nextPaneID  PaneID
	nextSplitID SplitID
	root        *node
	focused     PaneID
	allocated   map[PaneID]struct{}
}

// NewModel creates one implicit tab containing one focused root leaf.
func NewModel() *Model {
	const initialID = 1
	pane := PaneID(initialID)
	return &Model{
		tabID:       TabID(initialID),
		nextTabID:   TabID(initialID + 1),
		nextPaneID:  PaneID(initialID + 1),
		nextSplitID: SplitID(initialID),
		root:        leafNode(pane),
		focused:     pane,
		allocated:   map[PaneID]struct{}{pane: {}},
	}
}

func (m *Model) TabID() TabID              { return m.tabID }
func (m *Model) FocusedPane() PaneID       { return m.focused }
func (m *Model) Empty() bool               { return m.root == nil }
func (m *Model) PaneIDs() []PaneID         { return paneIDs(m.root) }
func validAxis(axis SplitAxis) bool        { return axis == SplitColumns || axis == SplitRows }
func validRatio(ratio SplitRatio) bool     { return ratio > 0 && ratio < RatioScale }
func validDirection(d Direction) bool      { return d >= FocusLeft && d <= FocusDown }
func (m *Model) paneExists(id PaneID) bool { return findLeaf(m.root, id) != nil }

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
	if m.root == nil {
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
	newRoot, replaced := replaceLeaf(m.root, pane, replacement)
	if !replaced {
		return 0, invariantError("split target %d disappeared", pane)
	}

	m.root = newRoot
	m.focused = newPane
	m.allocated[newPane] = struct{}{}
	m.nextPaneID++
	m.nextSplitID++
	return newPane, nil
}

// Focus selects an active pane explicitly.
func (m *Model) Focus(pane PaneID) error {
	if !m.paneExists(pane) {
		return ErrPaneNotFound
	}
	m.focused = pane
	return nil
}

// FocusNext moves to the next pane in visual depth-first order and wraps.
func (m *Model) FocusNext() (PaneID, error) {
	ids := m.PaneIDs()
	if len(ids) == 0 {
		return 0, ErrEmptyModel
	}
	for i, id := range ids {
		if id == m.focused {
			m.focused = ids[(i+1)%len(ids)]
			return m.focused, nil
		}
	}
	return 0, invariantError("focused pane %d is not active", m.focused)
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
	if m.root == nil {
		return 0, ErrEmptyModel
	}
	layout, err := m.LayoutWithMetrics(bounds, resolve)
	if err != nil {
		return 0, err
	}

	var source PaneGeometry
	found := false
	for _, geometry := range layout.Panes {
		if geometry.Pane == m.focused {
			source = geometry
			found = true
			break
		}
	}
	if !found {
		return 0, invariantError("focused pane %d has no geometry", m.focused)
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
	m.focused = layout.Panes[bestIndex].Pane
	return m.focused, nil
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
	Pane    PaneID
	Focused PaneID
	Closed  bool
	Empty   bool
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
	if !m.paneExists(pane) {
		return CloseResult{Pane: pane, Focused: m.focused, Empty: m.root == nil}, nil
	}
	visualOrder := m.PaneIDs()
	closedIndex := 0
	for i, id := range visualOrder {
		if id == pane {
			closedIndex = i
			break
		}
	}
	newRoot, removed := removeLeaf(m.root, pane)
	if !removed {
		return CloseResult{}, invariantError("active pane %d could not be removed", pane)
	}
	m.root = newRoot
	if newRoot == nil {
		m.focused = 0
	} else if m.focused == pane {
		remaining := m.PaneIDs()
		if closedIndex >= len(remaining) {
			closedIndex = len(remaining) - 1
		}
		m.focused = remaining[closedIndex]
	}
	return CloseResult{Pane: pane, Focused: m.focused, Closed: true, Empty: m.root == nil}, nil
}

// CheckInvariants verifies tree shape, stable identity state, and the single
// focused-leaf rule. It is intentionally public so randomized and later
// integration tests can check the model after every transition.
func (m *Model) CheckInvariants() error {
	if m.tabID == 0 {
		return invariantError("implicit tab ID is zero")
	}
	if m.nextTabID != 0 && m.nextTabID <= m.tabID {
		return invariantError("next tab ID %d does not follow tab %d", m.nextTabID, m.tabID)
	}
	if m.allocated == nil {
		return invariantError("allocated pane set is nil")
	}
	if m.root == nil {
		if m.focused != 0 {
			return invariantError("empty tree has focused pane %d", m.focused)
		}
		return nil
	}
	if m.focused == 0 {
		return invariantError("non-empty tree has zero focus")
	}

	seenNodes := make(map[*node]struct{})
	seenPanes := make(map[PaneID]struct{})
	seenSplits := make(map[SplitID]struct{})
	focusedLeaves := 0
	var visit func(*node) error
	visit = func(n *node) error {
		if n == nil {
			return invariantError("tree contains a nil child")
		}
		if _, duplicate := seenNodes[n]; duplicate {
			return invariantError("tree contains a cycle or shared node")
		}
		seenNodes[n] = struct{}{}

		if n.isLeaf() {
			if n.first != nil || n.second != nil || n.split != 0 || n.axis != 0 || n.ratio != 0 {
				return invariantError("pane %d leaf carries split state", n.pane)
			}
			if _, duplicate := seenPanes[n.pane]; duplicate {
				return invariantError("pane %d appears more than once", n.pane)
			}
			if _, allocated := m.allocated[n.pane]; !allocated {
				return invariantError("active pane %d was never allocated", n.pane)
			}
			if m.nextPaneID != 0 && n.pane >= m.nextPaneID {
				return invariantError("active pane %d is not below next ID %d", n.pane, m.nextPaneID)
			}
			seenPanes[n.pane] = struct{}{}
			if n.pane == m.focused {
				focusedLeaves++
			}
			return nil
		}

		if n.pane != 0 {
			return invariantError("split carries pane ID %d", n.pane)
		}
		if n.split == 0 {
			return invariantError("branch has zero split ID")
		}
		if _, duplicate := seenSplits[n.split]; duplicate {
			return invariantError("split %d appears more than once", n.split)
		}
		if m.nextSplitID != 0 && n.split >= m.nextSplitID {
			return invariantError("active split %d is not below next ID %d", n.split, m.nextSplitID)
		}
		seenSplits[n.split] = struct{}{}
		if !validAxis(n.axis) {
			return invariantError("split has invalid axis %d", n.axis)
		}
		if !validRatio(n.ratio) {
			return invariantError("split has invalid ratio %d", n.ratio)
		}
		if err := visit(n.first); err != nil {
			return err
		}
		return visit(n.second)
	}
	if err := visit(m.root); err != nil {
		return err
	}
	if focusedLeaves != 1 {
		return invariantError("expected one focused leaf, found %d", focusedLeaves)
	}
	return nil
}

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
