package mux

import (
	"errors"
	"fmt"

	"cervterm/internal/pty"
)

// CreateWindow constructs one complete runtime-owned window before publishing
// its topology. Every failed proposal leaves model IDs and registry ownership
// reusable; reader ingress carries owner identity so stale records cannot be
// delivered to a later pane that receives the same proposed ID.
func (m *Mux) CreateWindow(spec SpawnSpec, content PixelRect, metrics CellMetrics, title string) (WindowView, []Event, error) {
	if !m.bootstrapped {
		return WindowView{}, nil, ErrEmptyModel
	}
	if err := validateGeometry(content, metrics); err != nil {
		return WindowView{}, nil, err
	}
	token, err := m.model.PrepareWindow(title)
	if err != nil {
		return WindowView{}, nil, err
	}
	cols, rows := cellGeometry(content, metrics)
	if cols < MinPaneCols || rows < MinPaneRows {
		m.model.AbortWindow(token)
		return WindowView{}, nil, ErrSplitTooSmall
	}
	paneID := token.PaneID()
	if err := m.windowLifecycleFailure("reserve"); err != nil {
		m.model.AbortWindow(token)
		return WindowView{}, nil, err
	}
	if err := m.sessions.reserve(paneID); err != nil {
		m.model.AbortWindow(token)
		return WindowView{}, nil, err
	}
	reserved := true
	var p *pane
	rollback := func() {
		if p != nil {
			if detached := m.sessions.abort(paneID, p); detached.owned {
				_ = detached.pane.close()
			}
		}
		if reserved {
			m.sessions.release(paneID)
		}
		delete(m.paneMetrics, paneID)
		m.model.AbortWindow(token)
	}

	p = newPane(paneID, cols, rows, m.options.ScrollbackCapacity, m.options.HideCursorWhenScrolled)
	p.setFreshLaunch(spec)
	p.terminal.SetPaletteBase(m.paletteBase)
	p.geometry = effectiveGeometry(PaneGeometry{Pane: paneID, Pixels: content, Cols: cols, Rows: rows})
	if m.options.SetClipboard != nil {
		p.parser.SetClipboard = func(text string) { m.options.SetClipboard(p.id, text) }
	}
	ptyRows, ptyCols := terminalSize(p.geometry)
	if err := m.windowLifecycleFailure("spawn"); err != nil {
		rollback()
		return WindowView{}, nil, err
	}
	session, spawnErr := m.sessions.spawn(ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		rollback()
		return WindowView{}, nil, fmt.Errorf("spawn window pane: %w", spawnErr)
	}
	p.session = session
	p.state = PaneStateRunning
	p.desiredSize = pty.Size{Rows: ptyRows, Cols: ptyCols}
	p.appliedSize = p.desiredSize
	if err := m.windowLifecycleFailure("register"); err != nil {
		_ = p.close()
		p = nil
		rollback()
		return WindowView{}, nil, err
	}
	if err := m.sessions.register(p); err != nil {
		_ = p.close()
		p = nil
		rollback()
		return WindowView{}, nil, err
	}
	reserved = false
	if err := m.windowLifecycleFailure("start"); err != nil {
		rollback()
		return WindowView{}, nil, err
	}
	if err := m.sessions.start(paneID); err != nil {
		rollback()
		return WindowView{}, nil, err
	}
	if err := m.windowLifecycleFailure("commit"); err != nil {
		rollback()
		return WindowView{}, nil, err
	}
	view, err := m.model.CommitWindow(token)
	if err != nil {
		rollback()
		return WindowView{}, nil, err
	}
	m.paneMetrics[paneID] = metrics
	p.capture()
	events := []Event{
		{Kind: WindowCreated, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID, Text: title, Revision: view.Revision},
		{Kind: WindowActivated, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: view.Revision},
		{Kind: TabSpawned, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: 1},
		{Kind: TabActivated, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: 1},
		{Kind: PaneStarted, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID},
		{Kind: PaneFocused, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID},
		{Kind: PaneGeometryChanged, Workspace: view.Workspace, Window: view.ID, Tab: token.TabID(), Pane: paneID, Geometry: p.geometry},
	}
	return view, events, nil
}

func (m *Mux) ActivateWindow(id WindowID) ([]Event, error) {
	if err := m.model.ActivateWindow(id); err != nil {
		return nil, err
	}
	view := m.model.ActiveWindow()
	return []Event{{Kind: WindowActivated, Workspace: view.Workspace, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane(), Revision: view.Revision}, {Kind: TabActivated, Workspace: view.Workspace, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane()}, {Kind: PaneFocused, Workspace: view.Workspace, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane()}}, nil
}

// CloseWindow publishes detachment first, then closes each detached session at
// most once. Repeated closes of an already detached WindowID are successful and
// produce no events while preserving the current final-window result.
func (m *Mux) CloseWindow(id WindowID) (CloseWindowResult, []Event, error) {
	view := m.model.windowByID(id)
	if view != nil {
		for i := range view.tabs {
			for _, paneID := range paneIDs(view.tabs[i].root) {
				if _, owned := m.sessions.lookup(paneID); !owned {
					return CloseWindowResult{}, nil, invariantError("window %d pane %d is not registry-owned", id, paneID)
				}
			}
		}
	}
	workspace := WorkspaceID(0)
	if view != nil {
		workspace = view.workspace
	}
	result, err := m.model.CloseWindow(id)
	if err != nil || !result.Closed {
		return result, nil, err
	}
	events := make([]Event, 0, len(result.Panes)*2+5)
	var closeErrs []error
	for _, paneID := range result.Panes {
		detached := m.sessions.detach(paneID)
		delete(m.paneMetrics, paneID)
		if !detached.owned {
			return result, events, invariantError("window %d pane %d lost registry ownership", id, paneID)
		}
		tabID, _ := tabForPaneInResult(view, paneID)
		if closeErr := detached.pane.close(); closeErr != nil {
			closeErrs = append(closeErrs, fmt.Errorf("pane %d close: %w", paneID, closeErr))
			events = append(events, Event{Kind: PaneCloseFailed, Workspace: workspace, Window: id, Tab: tabID, Pane: paneID, Err: closeErr})
		}
		events = append(events, Event{Kind: PaneClosed, Workspace: workspace, Window: id, Tab: tabID, Pane: paneID})
	}
	for _, tabID := range result.Tabs {
		events = append(events, Event{Kind: TabClosed, Workspace: workspace, Window: id, Tab: tabID})
	}
	events = append(events, Event{Kind: WindowClosed, Workspace: workspace, Window: id})
	if result.WorkspaceChanged {
		activeWorkspace := m.model.ActiveWorkspace()
		events = append(events, Event{Kind: WorkspaceActivated, Workspace: result.ActiveWorkspace, SourceWorkspace: workspace, Window: result.Active, Revision: activeWorkspace.Revision})
	}
	if result.ActiveChanged && !result.Empty && result.Active != 0 {
		sourceWorkspace := WorkspaceID(0)
		if result.WorkspaceChanged {
			sourceWorkspace = workspace
		}
		events = append(events, Event{Kind: WindowActivated, Workspace: result.ActiveWorkspace, SourceWorkspace: sourceWorkspace, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane}, Event{Kind: TabActivated, Workspace: result.ActiveWorkspace, SourceWorkspace: sourceWorkspace, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane}, Event{Kind: PaneFocused, Workspace: result.ActiveWorkspace, SourceWorkspace: sourceWorkspace, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane})
	}
	return result, events, errors.Join(closeErrs...)
}

func tabForPaneInResult(w *windowState, paneID PaneID) (TabID, bool) {
	if w == nil {
		return 0, false
	}
	for i := range w.tabs {
		if findLeaf(w.tabs[i].root, paneID) != nil {
			return w.tabs[i].id, true
		}
	}
	return 0, false
}

func (m *Mux) windowLifecycleFailure(stage string) error {
	if m.windowFault != nil {
		return m.windowFault(stage)
	}
	return nil
}

// RollbackWindow aborts a newest runtime window that was never published to a frontend.
// Unlike CloseWindow it does not tombstone the proposed IDs, so a later candidate may reuse them.
func (m *Mux) RollbackWindow(id WindowID) error {
	w := m.model.windowByID(id)
	if w == nil {
		return ErrWindowNotFound
	}
	panes := make(map[PaneID]*pane)
	for i := range w.tabs {
		for _, paneID := range paneIDs(w.tabs[i].root) {
			p, ok := m.sessions.lookup(paneID)
			if !ok {
				return invariantError("window %d pane %d is not registry-owned", id, paneID)
			}
			panes[paneID] = p
		}
	}
	result, err := m.model.CloseWindow(id)
	if err != nil {
		return err
	}
	var closeErrs []error
	for paneID, p := range panes {
		detached := m.sessions.abort(paneID, p)
		if !detached.owned {
			return invariantError("window %d pane %d rollback lost ownership", id, paneID)
		}
		delete(m.paneMetrics, paneID)
		if closeErr := detached.pane.close(); closeErr != nil {
			closeErrs = append(closeErrs, closeErr)
		}
	}
	if err := m.model.rollbackClosedWindow(result); err != nil {
		return errors.Join(err, errors.Join(closeErrs...))
	}
	return errors.Join(closeErrs...)
}

func (m *Mux) Windows() []WindowView { return m.model.Windows() }

func (m *Mux) WindowForPane(id PaneID) (WindowID, bool) {
	tab := m.model.tabForPane(id)
	if tab == nil {
		return 0, false
	}
	w := m.model.windowForTab(tab.id)
	if w == nil {
		return 0, false
	}
	return w.id, true
}

func (m *Mux) WindowForTab(id TabID) (WindowID, bool) {
	w := m.model.windowForTab(id)
	if w == nil {
		return 0, false
	}
	return w.id, true
}

func (m *Mux) WorkspaceForWindow(id WindowID) (WorkspaceID, bool) {
	w := m.model.windowByID(id)
	if w == nil {
		return 0, false
	}
	return w.workspace, true
}
