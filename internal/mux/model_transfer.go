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
	source := m.tabForPane(pane)
	if source == nil {
		return TransferResult{}, ErrPaneNotFound
	}
	sourceWindow := m.windowForTab(source.id)
	destinationWindow := m.windowForTab(destinationTab)
	if destinationWindow == nil || destinationWindow != m.activeWindowState() {
		return TransferResult{}, ErrTabNotFound
	}
	previousActive, previousFocus := m.TabID(), m.FocusedPane()
	windowResult, err := m.TransferPaneBetweenWindows(PaneTransferRequest{
		SourceWindow: sourceWindow.id, DestinationWindow: destinationWindow.id, Pane: pane,
		DestinationTab: destinationTab, DestinationPane: destinationPane, Axis: axis, Ratio: ratio,
		SourceBounds: bounds, DestinationBounds: bounds, Resolve: resolve,
	})
	if err != nil {
		return TransferResult{}, err
	}
	activeChanged := windowResult.ActiveTab != previousActive
	focusChanged := activeChanged || windowResult.ActiveFocused != previousFocus
	return TransferResult{
		Pane: pane, SourceTab: windowResult.SourceTab, DestinationTab: destinationTab,
		SourceFocused: transferResultSourceFocus(m, windowResult), DestinationFocused: pane,
		SourceTabClosed: windowResult.SourceTabClosed, ActiveTab: windowResult.ActiveTab,
		ActiveFocused: windowResult.ActiveFocused, ActiveChanged: activeChanged,
		FocusChanged:   focusChanged,
		SourceRevision: windowResult.SourceTabRevision, DestinationRevision: windowResult.DestinationTabRevision,
		SourceLayout: windowResult.SourceLayout, DestinationLayout: windowResult.DestinationLayout,
	}, nil
}

func transferResultSourceFocus(m *Model, result WindowTransferResult) PaneID {
	if source := m.tabByID(result.SourceTab); source != nil {
		return source.focused
	}
	return 0
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
