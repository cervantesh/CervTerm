package mux

// TransferResult describes an atomic cross-tab ownership change.
type TransferResult struct {
	Pane                                PaneID
	SourceTab, DestinationTab           TabID
	SourceFocused, DestinationFocused   PaneID
	SourceTabClosed                     bool
	ActiveTab                           TabID
	ActiveFocused                       PaneID
	ActiveChanged, FocusChanged         bool
	SourceRevision, DestinationRevision uint64
	SourceLayout, DestinationLayout     Layout
}

// TransferPane moves an existing pane into a split in another tab without
// creating or closing its process. Both candidate trees are validated first.
func (m *Model) TransferPane(pane PaneID, destinationTab TabID, destinationPane PaneID, axis SplitAxis, ratio SplitRatio, bounds PixelRect, resolve CellMetricsResolver) (TransferResult, error) {
	var result TransferResult
	if !validAxis(axis) {
		return result, ErrInvalidAxis
	}
	if !validRatio(ratio) {
		return result, ErrInvalidRatio
	}
	if err := m.CheckInvariants(); err != nil {
		return result, err
	}
	if m.nextSplitID == 0 {
		return result, ErrIDExhausted
	}
	source := m.tabForPane(pane)
	if source == nil {
		return result, ErrPaneNotFound
	}
	destinationWindow := m.activeWindowState()
	destination := tabByID(destinationWindow, destinationTab)
	if destination == nil {
		return result, ErrTabNotFound
	}
	if source.id == destination.id {
		return result, ErrSameTabTransfer
	}
	if findLeaf(destination.root, destinationPane) == nil {
		return result, ErrPaneNotFound
	}

	sourceOrder := paneIDs(source.root)
	removedIndex := 0
	for i, id := range sourceOrder {
		if id == pane {
			removedIndex = i
			break
		}
	}
	sourceRoot, removed := removeLeaf(source.root, pane)
	if !removed {
		return result, invariantError("transfer pane %d disappeared", pane)
	}
	sourceFocus := source.focused
	if sourceRoot != nil && sourceFocus == pane {
		remaining := paneIDs(sourceRoot)
		if removedIndex >= len(remaining) {
			removedIndex = len(remaining) - 1
		}
		sourceFocus = remaining[removedIndex]
	}
	newSplit := m.nextSplitID
	replacement := branchNode(newSplit, axis, ratio, leafNode(destinationPane), leafNode(pane))
	destinationRoot, replaced := replaceLeaf(destination.root, destinationPane, replacement)
	if !replaced {
		return result, invariantError("transfer destination %d disappeared", destinationPane)
	}

	var sourceLayout Layout
	var err error
	if sourceRoot != nil {
		sourceLayout, err = layoutRoot(sourceRoot, bounds, resolve)
		if err != nil {
			return result, err
		}
		if err = validateTransferLayout(sourceLayout); err != nil {
			return result, err
		}
	}
	destinationLayout, err := layoutRoot(destinationRoot, bounds, resolve)
	if err != nil {
		return result, err
	}
	if err = validateTransferLayout(destinationLayout); err != nil {
		return result, err
	}

	previousWindows := cloneWindowStates(m.windows)
	previousActive, previousFocus := m.TabID(), m.FocusedPane()
	sourceID := source.id
	sourceWindow := m.windowForTab(sourceID)
	sourceIndex := -1
	for i := range sourceWindow.tabs {
		if sourceWindow.tabs[i].id == sourceID {
			sourceIndex = i
			break
		}
	}
	source.root, source.focused, source.revision = sourceRoot, sourceFocus, source.revision+1
	destination.root, destination.focused, destination.revision = destinationRoot, pane, destination.revision+1
	sourceWindow.revision++
	if destinationWindow != sourceWindow {
		destinationWindow.revision++
	}
	m.allocatedSplits[newSplit] = struct{}{}
	m.nextSplitID++
	if sourceRoot == nil {
		sourceWindow.tabs = append(sourceWindow.tabs[:sourceIndex], sourceWindow.tabs[sourceIndex+1:]...)
		if sourceWindow.active == sourceID {
			if len(sourceWindow.tabs) == 0 {
				sourceWindow.active = 0
			} else {
				if sourceIndex >= len(sourceWindow.tabs) {
					sourceIndex = len(sourceWindow.tabs) - 1
				}
				sourceWindow.active = sourceWindow.tabs[sourceIndex].id
			}
		}
	}
	if err := m.CheckInvariants(); err != nil {
		m.windows, m.nextSplitID = previousWindows, newSplit
		delete(m.allocatedSplits, newSplit)
		return result, err
	}

	finalDestination := m.tabByID(destinationTab)
	result = TransferResult{Pane: pane, SourceTab: sourceID, DestinationTab: destinationTab, SourceFocused: sourceFocus, DestinationFocused: pane, SourceTabClosed: sourceRoot == nil, ActiveTab: m.TabID(), ActiveFocused: m.FocusedPane(), ActiveChanged: m.TabID() != previousActive, FocusChanged: m.TabID() != previousActive || m.FocusedPane() != previousFocus, DestinationRevision: finalDestination.revision, SourceLayout: sourceLayout, DestinationLayout: destinationLayout}
	if finalSource := m.tabByID(sourceID); finalSource != nil {
		result.SourceRevision = finalSource.revision
	}
	return result, nil
}

func cloneWindowStates(windows []windowState) []windowState {
	out := append([]windowState(nil), windows...)
	for i := range out {
		out[i].tabs = append([]tabState(nil), windows[i].tabs...)
	}
	return out
}

func validateTransferLayout(layout Layout) error {
	for _, geometry := range layout.Panes {
		if geometry.Cols < TopologyMinPaneCols || geometry.Rows < TopologyMinPaneRows {
			return ErrTopologyTooSmall
		}
	}
	return nil
}
