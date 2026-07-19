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
		{Kind: WindowCreated, Window: view.ID, Tab: token.TabID(), Pane: paneID, Text: title, Revision: view.Revision},
		{Kind: WindowActivated, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: view.Revision},
		{Kind: TabSpawned, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: 1},
		{Kind: TabActivated, Window: view.ID, Tab: token.TabID(), Pane: paneID, Revision: 1},
		{Kind: PaneStarted, Window: view.ID, Tab: token.TabID(), Pane: paneID},
		{Kind: PaneFocused, Window: view.ID, Tab: token.TabID(), Pane: paneID},
		{Kind: PaneGeometryChanged, Window: view.ID, Tab: token.TabID(), Pane: paneID, Geometry: p.geometry},
	}
	return view, events, nil
}

func (m *Mux) ActivateWindow(id WindowID) ([]Event, error) {
	if err := m.model.ActivateWindow(id); err != nil {
		return nil, err
	}
	view := m.model.ActiveWindow()
	return []Event{{Kind: WindowActivated, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane(), Revision: view.Revision}, {Kind: TabActivated, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane()}, {Kind: PaneFocused, Window: id, Tab: m.model.TabID(), Pane: m.model.FocusedPane()}}, nil
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
			events = append(events, Event{Kind: PaneCloseFailed, Window: id, Tab: tabID, Pane: paneID, Err: closeErr})
		}
		events = append(events, Event{Kind: PaneClosed, Window: id, Tab: tabID, Pane: paneID})
	}
	for _, tabID := range result.Tabs {
		events = append(events, Event{Kind: TabClosed, Window: id, Tab: tabID})
	}
	events = append(events, Event{Kind: WindowClosed, Window: id})
	if !result.Empty && result.Active != 0 {
		events = append(events, Event{Kind: WindowActivated, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane}, Event{Kind: TabActivated, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane}, Event{Kind: PaneFocused, Window: result.Active, SourceWindow: id, Tab: result.ActiveTab, Pane: result.FocusedPane})
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
