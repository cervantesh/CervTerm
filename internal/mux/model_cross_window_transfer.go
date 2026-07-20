package mux

// PaneTransferRequest fully identifies and sizes both sides of a pane move.
type PaneTransferRequest struct {
	SourceWindow, DestinationWindow WindowID
	Pane                            PaneID
	DestinationTab                  TabID
	DestinationPane                 PaneID
	Axis                            SplitAxis
	Ratio                           SplitRatio
	SourceBounds, DestinationBounds PixelRect
	Resolve                         CellMetricsResolver
}

// TabTransferRequest fully identifies and sizes both sides of a whole-tab move.
type TabTransferRequest struct {
	SourceWindow, DestinationWindow WindowID
	Tab                             TabID
	Position                        int
	SourceBounds, DestinationBounds PixelRect
	Resolve                         CellMetricsResolver
}

type TabRevisionUpdate struct {
	Window   WindowID
	Tab      TabID
	Revision uint64
}

// WindowTransferResult describes one committed cross-window ownership change.
type WindowTransferResult struct {
	Pane, Focused                               PaneID
	Tab, SourceTab                              TabID
	Window, SourceWindow                        WindowID
	SourceTabClosed, SourceWindowEmpty          bool
	ActiveWindow                                WindowID
	ActiveTab                                   TabID
	ActiveFocused                               PaneID
	WindowActivated, TabActivated, FocusChanged bool
	SourceRevision, DestinationRevision         uint64
	SourceTabRevision, DestinationTabRevision   uint64
	RevisionUpdates                             []TabRevisionUpdate
	SourceLayout, DestinationLayout             Layout
}

func (m *Model) TransferPaneBetweenWindows(req PaneTransferRequest) (WindowTransferResult, error) {
	var result WindowTransferResult
	if !validAxis(req.Axis) {
		return result, ErrInvalidAxis
	}
	if !validRatio(req.Ratio) {
		return result, ErrInvalidRatio
	}
	if err := m.CheckInvariants(); err != nil {
		return result, err
	}
	if m.nextSplitID == 0 {
		return result, ErrIDExhausted
	}
	sourceWindow := m.windowByID(req.SourceWindow)
	destinationWindow := m.windowByID(req.DestinationWindow)
	if sourceWindow == nil || destinationWindow == nil {
		return result, ErrWindowNotFound
	}
	source := m.tabForPane(req.Pane)
	if source == nil || tabByID(sourceWindow, source.id) == nil {
		return result, ErrPaneNotFound
	}
	destination := tabByID(destinationWindow, req.DestinationTab)
	if destination == nil {
		return result, ErrTabNotFound
	}
	if source.id == destination.id {
		return result, ErrSameTabTransfer
	}
	if findLeaf(destination.root, req.DestinationPane) == nil {
		return result, ErrPaneNotFound
	}

	sourceRoot, removed := removeLeaf(source.root, req.Pane)
	if !removed {
		return result, invariantError("transfer pane %d disappeared", req.Pane)
	}
	sourceFocus := transferFallbackFocus(source.root, sourceRoot, source.focused, req.Pane)
	newSplit := m.nextSplitID
	replacement := branchNode(newSplit, req.Axis, req.Ratio, leafNode(req.DestinationPane), leafNode(req.Pane))
	destinationRoot, replaced := replaceLeaf(destination.root, req.DestinationPane, replacement)
	if !replaced {
		return result, invariantError("transfer destination %d disappeared", req.DestinationPane)
	}

	var sourceLayout Layout
	var err error
	if sourceRoot != nil {
		sourceLayout, err = validatedTransferLayout(sourceRoot, req.SourceBounds, req.Resolve)
		if err != nil {
			return result, err
		}
	}
	destinationLayout, err := validatedTransferLayout(destinationRoot, req.DestinationBounds, req.Resolve)
	if err != nil {
		return result, err
	}

	beforeWindow, beforeTab, beforeFocus := m.activeIdentity()
	previous := cloneWindowStates(m.windows)
	previousWorkspaces := cloneWorkspaceStates(m.workspaces)
	previousActive, previousNext := m.activeWindow, m.nextSplitID
	sourceID, destinationID := source.id, destination.id
	sourceIndex := tabIndex(sourceWindow, sourceID)
	source.root, source.focused, source.revision = sourceRoot, sourceFocus, source.revision+1
	destination.root, destination.focused, destination.revision = destinationRoot, req.Pane, destination.revision+1
	sourceWindow.revision++
	if destinationWindow != sourceWindow {
		destinationWindow.revision++
	}
	m.allocatedSplits[newSplit] = struct{}{}
	m.nextSplitID++
	if sourceRoot == nil {
		removeTabAt(sourceWindow, sourceIndex)
		if sourceWindow.active == sourceID {
			sourceWindow.active = adjacentTabID(sourceWindow, sourceIndex)
		}
		if len(sourceWindow.tabs) == 0 && m.activeWindow == sourceWindow.id {
			m.activeWindow = destinationWindow.id
			destinationWindow.active = destinationID
		}
	}
	m.reconcileActiveWorkspaceWindow()
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.activeWindow, m.nextSplitID, m.workspaces = previous, previousActive, previousNext, previousWorkspaces
		delete(m.allocatedSplits, newSplit)
		return result, err
	}
	result = m.windowTransferResult(req.Pane, sourceID, destinationID, sourceWindow, destinationWindow, sourceRoot == nil, sourceLayout, destinationLayout)
	result.setActiveChanges(beforeWindow, beforeTab, beforeFocus)
	return result, nil
}

func (m *Model) TransferTabBetweenWindows(req TabTransferRequest) (WindowTransferResult, error) {
	var result WindowTransferResult
	if err := m.CheckInvariants(); err != nil {
		return result, err
	}
	sourceWindow := m.windowByID(req.SourceWindow)
	destinationWindow := m.windowByID(req.DestinationWindow)
	if sourceWindow == nil || destinationWindow == nil {
		return result, ErrWindowNotFound
	}
	if sourceWindow == destinationWindow {
		return result, ErrSameWindowTransfer
	}
	if req.Position < 0 || req.Position > len(destinationWindow.tabs) {
		return result, ErrInvalidTabPosition
	}
	if len(destinationWindow.tabs) >= MaxTabs {
		return result, ErrTabLimitReached
	}
	sourceIndex := tabIndex(sourceWindow, req.Tab)
	if sourceIndex < 0 {
		return result, ErrTabNotFound
	}
	moved := sourceWindow.tabs[sourceIndex]
	destinationLayout, err := validatedTransferLayout(moved.root, req.DestinationBounds, req.Resolve)
	if err != nil {
		return result, err
	}

	candidateSource := append([]tabState(nil), sourceWindow.tabs...)
	candidateSource = append(candidateSource[:sourceIndex], candidateSource[sourceIndex+1:]...)
	candidateActive := sourceWindow.active
	if candidateActive == moved.id {
		candidateActive = adjacentTabIDFrom(candidateSource, sourceIndex)
	}
	var sourceLayout Layout
	if active := tabByID(&windowState{tabs: candidateSource}, candidateActive); active != nil {
		sourceLayout, err = validatedTransferLayout(active.root, req.SourceBounds, req.Resolve)
		if err != nil {
			return result, err
		}
	}

	beforeWindow, beforeTab, beforeFocus := m.activeIdentity()
	previous := cloneWindowStates(m.windows)
	previousWorkspaces := cloneWorkspaceStates(m.workspaces)
	previousActive := m.activeWindow
	moved.revision++
	for i := range sourceWindow.tabs {
		if i != sourceIndex {
			sourceWindow.tabs[i].revision++
		}
	}
	for i := range destinationWindow.tabs {
		destinationWindow.tabs[i].revision++
	}
	removeTabAt(sourceWindow, sourceIndex)
	sourceWindow.active = candidateActive
	insertTabAt(destinationWindow, req.Position, moved)
	destinationWindow.active = moved.id
	sourceWindow.revision++
	destinationWindow.revision++
	m.activeWindow = destinationWindow.id
	m.reconcileActiveWorkspaceWindow()
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.activeWindow, m.workspaces = previous, previousActive, previousWorkspaces
		return result, err
	}
	result = m.windowTransferResult(0, moved.id, moved.id, sourceWindow, destinationWindow, false, sourceLayout, destinationLayout)
	for _, tab := range sourceWindow.tabs {
		result.RevisionUpdates = append(result.RevisionUpdates, TabRevisionUpdate{Window: sourceWindow.id, Tab: tab.id, Revision: tab.revision})
	}
	for _, tab := range destinationWindow.tabs {
		result.RevisionUpdates = append(result.RevisionUpdates, TabRevisionUpdate{Window: destinationWindow.id, Tab: tab.id, Revision: tab.revision})
	}
	result.setActiveChanges(beforeWindow, beforeTab, beforeFocus)
	return result, nil
}

func validatedTransferLayout(root *node, bounds PixelRect, resolve CellMetricsResolver) (Layout, error) {
	layout, err := layoutRoot(root, bounds, resolve)
	if err != nil {
		return Layout{}, err
	}
	if err := validateTransferLayout(layout); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func transferFallbackFocus(oldRoot, newRoot *node, focused, removed PaneID) PaneID {
	if newRoot == nil || focused != removed {
		return focused
	}
	oldOrder, remaining := paneIDs(oldRoot), paneIDs(newRoot)
	index := 0
	for i, id := range oldOrder {
		if id == removed {
			index = i
			break
		}
	}
	if index >= len(remaining) {
		index = len(remaining) - 1
	}
	return remaining[index]
}

func tabIndex(w *windowState, id TabID) int {
	for i := range w.tabs {
		if w.tabs[i].id == id {
			return i
		}
	}
	return -1
}

func removeTabAt(w *windowState, index int) { w.tabs = append(w.tabs[:index], w.tabs[index+1:]...) }
func insertTabAt(w *windowState, index int, tab tabState) {
	w.tabs = append(w.tabs, tabState{})
	copy(w.tabs[index+1:], w.tabs[index:])
	w.tabs[index] = tab
}
func adjacentTabID(w *windowState, removed int) TabID { return adjacentTabIDFrom(w.tabs, removed) }
func adjacentTabIDFrom(tabs []tabState, removed int) TabID {
	if len(tabs) == 0 {
		return 0
	}
	if removed >= len(tabs) {
		removed = len(tabs) - 1
	}
	return tabs[removed].id
}

func (m *Model) windowTransferResult(pane PaneID, sourceTab, destinationTab TabID, sourceWindow, destinationWindow *windowState, sourceTabClosed bool, sourceLayout, destinationLayout Layout) WindowTransferResult {
	result := WindowTransferResult{Pane: pane, Tab: destinationTab, SourceTab: sourceTab, Window: destinationWindow.id, SourceWindow: sourceWindow.id, SourceTabClosed: sourceTabClosed, SourceWindowEmpty: len(sourceWindow.tabs) == 0, ActiveWindow: m.activeWindow, SourceRevision: sourceWindow.revision, DestinationRevision: destinationWindow.revision, SourceLayout: sourceLayout, DestinationLayout: destinationLayout}
	if source := tabByID(sourceWindow, sourceTab); source != nil {
		result.SourceTabRevision = source.revision
	}
	if destination := tabByID(destinationWindow, destinationTab); destination != nil {
		result.DestinationTabRevision, result.Focused = destination.revision, destination.focused
	}
	if activeWindow := m.activeWindowState(); activeWindow != nil {
		result.ActiveTab = activeWindow.active
		if active := tabByID(activeWindow, activeWindow.active); active != nil {
			result.ActiveFocused = active.focused
		}
	}
	return result
}

func (m *Model) activeIdentity() (WindowID, TabID, PaneID) {
	w := m.activeWindowState()
	if w == nil {
		return 0, 0, 0
	}
	t := tabByID(w, w.active)
	if t == nil {
		return w.id, 0, 0
	}
	return w.id, t.id, t.focused
}

func (r *WindowTransferResult) setActiveChanges(beforeWindow WindowID, beforeTab TabID, beforeFocus PaneID) {
	r.WindowActivated = r.ActiveWindow != beforeWindow
	r.TabActivated = r.WindowActivated || r.ActiveTab != beforeTab
	r.FocusChanged = r.TabActivated || r.ActiveFocused != beforeFocus
}
