package mux

// WindowID is a stable, process-local mux-window identity. Zero is invalid.
type WindowID uint64

// PaneID is a stable, process-local pane identity. Zero is never valid.
type PaneID uint64

// SplitID is a stable, process-local identity for one branch divider.
type SplitID uint64

// TabID is a stable, process-local tab identity.
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

const (
	MaxWindows = 32
	MaxTabs    = 256
)

type tabState struct {
	id       TabID
	title    string
	root     *node
	focused  PaneID
	revision uint64
}

type windowState struct {
	id       WindowID
	title    string
	tabs     []tabState
	active   TabID
	revision uint64
}

// Model owns pure, ordered mux-window/tab topology, identity, and focus state.
type Model struct {
	windows          []windowState
	activeWindow     WindowID
	nextWindowID     WindowID
	nextTabID        TabID
	nextPaneID       PaneID
	nextSplitID      SplitID
	allocatedWindows map[WindowID]struct{}
	allocated        map[PaneID]struct{}
	allocatedSplits  map[SplitID]struct{}
	allocatedTabs    map[TabID]struct{}
}

// NewModel creates WindowID 1 with the compatibility TabID 1 / PaneID 1.
func NewModel() *Model {
	pane, tab, window := PaneID(1), TabID(1), WindowID(1)
	return &Model{
		windows:      []windowState{{id: window, tabs: []tabState{{id: tab, root: leafNode(pane), focused: pane, revision: 1}}, active: tab, revision: 1}},
		activeWindow: window, nextWindowID: 2, nextTabID: 2, nextPaneID: 2, nextSplitID: 1,
		allocatedWindows: map[WindowID]struct{}{window: {}}, allocated: map[PaneID]struct{}{pane: {}},
		allocatedSplits: map[SplitID]struct{}{}, allocatedTabs: map[TabID]struct{}{tab: {}},
	}
}

func (m *Model) activeWindowState() *windowState { return m.windowByID(m.activeWindow) }
func (m *Model) activeTab() *tabState {
	w := m.activeWindowState()
	if w == nil {
		return nil
	}
	return tabByID(w, w.active)
}
func (m *Model) windowByID(id WindowID) *windowState {
	for i := range m.windows {
		if m.windows[i].id == id {
			return &m.windows[i]
		}
	}
	return nil
}
func tabByID(w *windowState, id TabID) *tabState {
	if w == nil {
		return nil
	}
	for i := range w.tabs {
		if w.tabs[i].id == id {
			return &w.tabs[i]
		}
	}
	return nil
}
func (m *Model) tabByID(id TabID) *tabState {
	for i := range m.windows {
		if t := tabByID(&m.windows[i], id); t != nil {
			return t
		}
	}
	return nil
}
func (m *Model) windowForTab(id TabID) *windowState {
	for i := range m.windows {
		if tabByID(&m.windows[i], id) != nil {
			return &m.windows[i]
		}
	}
	return nil
}
func (m *Model) tabForPane(id PaneID) *tabState {
	for i := range m.windows {
		for j := range m.windows[i].tabs {
			if findLeaf(m.windows[i].tabs[j].root, id) != nil {
				return &m.windows[i].tabs[j]
			}
		}
	}
	return nil
}
func (m *Model) tabForSplit(id SplitID) *tabState {
	for i := range m.windows {
		for j := range m.windows[i].tabs {
			if findSplit(m.windows[i].tabs[j].root, id) != nil {
				return &m.windows[i].tabs[j]
			}
		}
	}
	return nil
}
func (m *Model) TabID() TabID {
	if w := m.activeWindowState(); w != nil {
		return w.active
	}
	return 0
}
func (m *Model) FocusedPane() PaneID {
	if t := m.activeTab(); t != nil {
		return t.focused
	}
	return 0
}
func (m *Model) Empty() bool {
	for i := range m.windows {
		if len(m.windows[i].tabs) != 0 {
			return false
		}
	}
	return true
}
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

// CheckInvariants verifies ordered tab, ownership, tree, and monotonic ID state.
func (m *Model) CheckInvariants() error { return checkModelInvariants(m) }
