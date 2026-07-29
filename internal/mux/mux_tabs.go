package mux

import (
	"errors"
	"fmt"

	"cervterm/internal/pty"
)

func (m *Mux) Tabs() []TabView  { return m.model.Tabs() }
func (m *Mux) ActiveTab() TabID { return m.model.TabID() }
func (m *Mux) TabForPane(pane PaneID) (TabID, bool) {
	tab := m.model.tabForPane(pane)
	if tab == nil {
		return 0, false
	}
	return tab.id, true
}

// SpawnTab prepares and spawns the initial pane before committing any identity.
func (m *Mux) spawnTab(scope mutationScope, spec SpawnSpec, metrics CellMetrics, title string) (TabID, PaneID, []Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return 0, 0, nil, err
	}
	if !m.bootstrapped {
		return 0, 0, nil, ErrEmptyModel
	}
	if err := validateGeometry(m.bounds, metrics); err != nil {
		return 0, 0, nil, err
	}
	tabID, paneID, err := m.model.prepareTab()
	if err != nil {
		return 0, 0, nil, err
	}
	cols, rows := cellGeometry(m.bounds, metrics)
	if cols < MinPaneCols || rows < MinPaneRows {
		return 0, 0, nil, ErrSplitTooSmall
	}
	if err := m.sessions.reserveScoped(scope, paneID); err != nil {
		return 0, 0, nil, err
	}
	defer m.sessions.releaseScoped(scope, paneID)
	pane := m.createPane(scope, paneID, cols, rows)
	pane.setFreshLaunch(spec)
	pane.terminal.SetPaletteBase(*m.paletteBase)
	if m.options.SetClipboard != nil {
		pane.parser.SetClipboard = func(text string) { m.options.SetClipboard(pane.id, text) }
	}
	ptyRows, ptyCols := terminalSize(PaneGeometry{Pane: paneID, Pixels: m.bounds, Cols: cols, Rows: rows})
	session, spawnErr := m.sessions.spawnScoped(scope, ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		return 0, 0, nil, errors.Join(fmt.Errorf("spawn tab: %w", spawnErr), pane.close())
	}
	pane.session = session
	pane.state = PaneStateRunning
	pane.geometry = effectiveGeometry(PaneGeometry{Pane: paneID, Pixels: m.bounds, Cols: cols, Rows: rows})
	pane.desiredSize = pty.Size{Rows: ptyRows, Cols: ptyCols}
	pane.appliedSize = pane.desiredSize
	if err := m.sessions.registerScoped(scope, pane); err != nil {
		return 0, 0, nil, errors.Join(err, pane.close())
	}
	if err := m.sessions.startScoped(scope, pane.id); err != nil {
		return 0, 0, nil, errors.Join(err, m.detachAndClosePane(scope, pane.id, pane, true))
	}
	preparedClose, err := pane.prepareClose()
	if err != nil {
		return 0, 0, nil, err
	}
	if err := m.model.commitTab(tabID, paneID, title); err != nil {
		detached := m.sessions.detachScoped(scope, pane.id)
		if !detached.owned || detached.pane != pane {
			return 0, 0, nil, errors.Join(err, invariantError("tab %d pane %d changed ownership before rollback", tabID, paneID), preparedClose.abort())
		}
		return 0, 0, nil, errors.Join(err, preparedClose.commit())
	}
	if err := preparedClose.abort(); err != nil {
		return 0, 0, nil, err
	}
	m.paneMetrics[paneID] = metrics
	pane.capture()
	return tabID, paneID, m.ResolveEventAddresses([]Event{{Kind: TabSpawned, Tab: tabID, Pane: paneID, Text: title, Revision: 1}, {Kind: TabActivated, Tab: tabID, Pane: paneID, Revision: 1}, {Kind: PaneStarted, Tab: tabID, Pane: paneID}, {Kind: PaneFocused, Tab: tabID, Pane: paneID}}), nil
}

func (m *Mux) activateTab(scope mutationScope, id TabID) ([]Event, error) {
	if err := scope.validTabOrigin(m, id); err != nil {
		return nil, err
	}
	tab := m.model.tabByID(id)
	if tab == nil {
		return nil, ErrTabNotFound
	}
	layout, err := layoutRoot(tab.root, m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	if err := m.model.ActivateTab(id); err != nil {
		return nil, err
	}
	focused := m.model.FocusedPane()
	events := []Event{{Kind: TabActivated, Tab: id, Pane: focused}, {Kind: PaneFocused, Tab: id, Pane: focused}}
	layoutEvents, err := m.applyLayout(scope, layout)
	for i := range layoutEvents {
		layoutEvents[i].Tab = id
	}
	return m.ResolveEventAddresses(append(events, layoutEvents...)), err
}
func (m *Mux) renameTab(scope mutationScope, id TabID, title string) ([]Event, error) {
	if err := scope.validTabOrigin(m, id); err != nil {
		return nil, err
	}
	if err := m.model.RenameTab(id, title); err != nil {
		return nil, err
	}
	revision := m.model.tabByID(id).revision
	return m.ResolveEventAddresses([]Event{{Kind: TabRenamed, Tab: id, Text: title, Revision: revision}, {Kind: TabRevisionChanged, Tab: id, Revision: revision}}), nil
}
func (m *Mux) moveTab(scope mutationScope, id TabID, position int) ([]Event, error) {
	if err := scope.validTabOrigin(m, id); err != nil {
		return nil, err
	}
	if err := m.model.MoveTab(id, position); err != nil {
		return nil, err
	}
	revision := m.model.tabByID(id).revision
	return m.ResolveEventAddresses([]Event{{Kind: TabMoved, Tab: id, Data: []byte(fmt.Sprintf("%d", position)), Revision: revision}, {Kind: TabRevisionChanged, Tab: id, Revision: revision}}), nil
}

// CloseTab atomically detaches ownership before closing each session once.
func (m *Mux) closeTab(scope mutationScope, id TabID) ([]Event, error) {
	if err := scope.validTabOrigin(m, id); err != nil {
		return nil, err
	}
	tab := m.model.tabByID(id)
	if tab == nil {
		return nil, ErrTabNotFound
	}
	paneIDsToClose := paneIDs(tab.root)
	panes := make([]*pane, 0, len(paneIDsToClose))
	for _, paneID := range paneIDsToClose {
		owned, ok := m.sessions.lookup(paneID)
		if !ok {
			return nil, invariantError("tab %d pane %d is not registry-owned", id, paneID)
		}
		panes = append(panes, owned)
	}
	preparedCloses, err := preparePaneClosures(panes)
	if err != nil {
		return nil, err
	}
	closeByID := make(map[PaneID]*preparedPaneClose, len(panes))
	for index, owned := range panes {
		closeByID[owned.id] = preparedCloses[index]
	}
	window, _ := m.WindowForTab(id)
	workspace, _ := m.WorkspaceForWindow(window)
	detached, err := m.model.detachTab(id)
	if err != nil {
		return nil, errors.Join(err, abortPaneClosures(preparedCloses))
	}
	events := make([]Event, 0, len(detached.panes)+3)
	var closeErrs []error
	for _, paneID := range detached.panes {
		result := m.sessions.detachScoped(scope, paneID)
		delete(m.paneMetrics, paneID)
		if !result.owned || result.pane != closeByID[paneID].pane {
			return events, errors.Join(invariantError("tab %d pane %d lost registry ownership", id, paneID), abortPaneClosures(preparedCloses))
		}
		if err := closeByID[paneID].commit(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("pane %d close: %w", paneID, err))
			events = append(events, Event{Kind: PaneCloseFailed, Tab: id, Pane: paneID, Err: err})
		}
		events = append(events, Event{Kind: PaneClosed, Tab: id, Pane: paneID})
	}
	events = append(events, Event{Kind: TabClosed, Tab: id})
	if detached.active == 0 {
		events = append(events, Event{Kind: WindowTabsEmpty, Tab: id}, Event{Kind: TabEmpty, Tab: id})
	} else {
		events = append(events, Event{Kind: TabActivated, Tab: detached.active, Pane: detached.focused}, Event{Kind: PaneFocused, Tab: detached.active, Pane: detached.focused})
	}
	for i := range events {
		if events[i].Window == 0 {
			events[i].Window = window
		}
		if events[i].Workspace == 0 {
			events[i].Workspace = workspace
		}
	}
	return m.ResolveEventAddresses(events), errors.Join(closeErrs...)
}

// TransferPane atomically changes tree ownership without spawning, closing, or resizing a PTY.
func (m *Mux) transferPane(scope mutationScope, pane PaneID, destinationTab TabID, destinationPane PaneID, axis SplitAxis) ([]Event, error) {
	if err := scope.validPaneOrigin(m, pane); err != nil {
		return nil, err
	}
	if err := scope.validTabOrigin(m, destinationTab); err != nil {
		return nil, err
	}
	if err := scope.validPaneOrigin(m, destinationPane); err != nil {
		return nil, err
	}
	result, err := m.model.TransferPane(pane, destinationTab, destinationPane, axis, DefaultSplitRatio, m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	events := []Event{{Kind: PaneTransferred, Pane: pane, Tab: destinationTab, SourceTab: result.SourceTab}}
	if !result.SourceTabClosed {
		events = append(events, Event{Kind: TabRevisionChanged, Tab: result.SourceTab, Revision: result.SourceRevision})
	}
	events = append(events, Event{Kind: TabRevisionChanged, Tab: destinationTab, Revision: result.DestinationRevision})
	if result.SourceTabClosed {
		events = append(events, Event{Kind: TabClosed, Tab: result.SourceTab})
	}
	if result.ActiveChanged {
		events = append(events, Event{Kind: TabActivated, Tab: result.ActiveTab, Pane: result.ActiveFocused})
	}
	if result.FocusChanged {
		events = append(events, Event{Kind: PaneFocused, Tab: result.ActiveTab, Pane: result.ActiveFocused})
	}
	var activeLayout Layout
	if result.ActiveTab == result.SourceTab && !result.SourceTabClosed {
		activeLayout = result.SourceLayout
	}
	if result.ActiveTab == result.DestinationTab {
		activeLayout = result.DestinationLayout
	}
	if len(activeLayout.Panes) > 0 {
		layoutEvents, applyErr := m.applyLayout(scope, activeLayout)
		if applyErr != nil {
			return m.ResolveEventAddresses(events), applyErr
		}
		for i := range layoutEvents {
			layoutEvents[i].Tab = result.ActiveTab
		}
		events = append(events, layoutEvents...)
	}
	return m.ResolveEventAddresses(events), nil
}
