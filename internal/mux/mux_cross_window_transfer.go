package mux

import "fmt"

// TransferPaneBetweenWindows is a non-UI transactional ownership foundation.
// It projects both candidate layouts but deliberately does not resize PTYs.
func (m *Mux) TransferPaneBetweenWindows(req PaneTransferRequest) ([]Event, error) {
	if err := m.validateTransferRuntime(req.Resolve); err != nil {
		return nil, err
	}
	result, err := m.model.TransferPaneBetweenWindows(req)
	if err != nil {
		return nil, err
	}
	events := []Event{{Kind: PaneTransferred, Window: result.Window, SourceWindow: result.SourceWindow, Pane: result.Pane, Tab: result.Tab, SourceTab: result.SourceTab}}
	if !result.SourceTabClosed {
		events = append(events, Event{Kind: TabRevisionChanged, Window: result.SourceWindow, SourceWindow: result.SourceWindow, Tab: result.SourceTab, SourceTab: result.SourceTab, Revision: result.SourceTabRevision})
	}
	events = append(events, Event{Kind: TabRevisionChanged, Window: result.Window, SourceWindow: result.SourceWindow, Tab: result.Tab, SourceTab: result.SourceTab, Revision: result.DestinationTabRevision})
	if result.SourceTabClosed {
		events = append(events, Event{Kind: TabClosed, Window: result.SourceWindow, SourceWindow: result.SourceWindow, Tab: result.SourceTab, SourceTab: result.SourceTab})
	}
	if result.SourceWindowEmpty {
		events = append(events, Event{Kind: WindowTabsEmpty, Window: result.SourceWindow, SourceWindow: result.SourceWindow, Tab: result.SourceTab, SourceTab: result.SourceTab})
	}
	if result.WindowActivated {
		events = append(events, Event{Kind: WindowActivated, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab})
	}
	if result.TabActivated {
		events = append(events, Event{Kind: TabActivated, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab, Pane: result.ActiveFocused})
	}
	if result.FocusChanged {
		events = append(events, Event{Kind: PaneFocused, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab, Pane: result.ActiveFocused})
	}
	return m.ResolveEventAddresses(events), nil
}

// TransferTabBetweenWindows moves an existing tabState as a whole. It allocates
// no tab, pane, or split identity and deliberately does not resize PTYs.
func (m *Mux) TransferTabBetweenWindows(req TabTransferRequest) ([]Event, error) {
	if err := m.validateTransferRuntime(req.Resolve); err != nil {
		return nil, err
	}
	result, err := m.model.TransferTabBetweenWindows(req)
	if err != nil {
		return nil, err
	}
	events := []Event{{Kind: TabMoved, Window: result.Window, SourceWindow: result.SourceWindow, Tab: result.Tab, SourceTab: result.SourceTab, Data: []byte(fmt.Sprintf("%d", req.Position)), Revision: result.DestinationTabRevision}}
	for _, update := range result.RevisionUpdates {
		events = append(events, Event{Kind: TabRevisionChanged, Window: update.Window, SourceWindow: result.SourceWindow, Tab: update.Tab, SourceTab: result.SourceTab, Revision: update.Revision})
	}
	if result.SourceWindowEmpty {
		events = append(events, Event{Kind: WindowTabsEmpty, Window: result.SourceWindow, SourceWindow: result.SourceWindow, Tab: result.SourceTab, SourceTab: result.SourceTab})
	}
	if result.WindowActivated {
		events = append(events, Event{Kind: WindowActivated, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab})
	}
	if result.TabActivated {
		events = append(events, Event{Kind: TabActivated, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab, Pane: result.ActiveFocused})
	}
	if result.FocusChanged {
		events = append(events, Event{Kind: PaneFocused, Window: result.ActiveWindow, SourceWindow: result.SourceWindow, Tab: result.ActiveTab, SourceTab: result.SourceTab, Pane: result.ActiveFocused})
	}
	return m.ResolveEventAddresses(events), nil
}

func (m *Mux) validateTransferRuntime(resolve CellMetricsResolver) error {
	if resolve == nil {
		return ErrInvalidGeometry
	}
	for _, window := range m.model.Windows() {
		for _, tab := range window.Tabs {
			for _, paneID := range tab.Panes {
				if _, owned := m.sessions.lookup(paneID); !owned {
					return invariantError("pane %d is not registry-owned", paneID)
				}
				metrics, ok := resolve(paneID)
				if !ok {
					return ErrPaneNotFound
				}
				if err := validateCellMetrics(metrics); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
