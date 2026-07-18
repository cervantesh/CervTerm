package mux

import (
	"errors"
	"fmt"

	"cervterm/internal/pty"
)

func (m *Mux) Tabs() []TabView  { return m.model.Tabs() }
func (m *Mux) ActiveTab() TabID { return m.model.TabID() }

// SpawnTab prepares and spawns the initial pane before committing any identity.
func (m *Mux) SpawnTab(spec SpawnSpec, metrics CellMetrics, title string) (TabID, PaneID, []Event, error) {
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
	pane := newPane(paneID, cols, rows, m.options.ScrollbackCapacity, m.options.HideCursorWhenScrolled)
	pane.terminal.SetPaletteBase(m.paletteBase)
	if m.options.SetClipboard != nil {
		pane.parser.SetClipboard = func(text string) { m.options.SetClipboard(pane.id, text) }
	}
	ptyRows, ptyCols := terminalSize(PaneGeometry{Pane: paneID, Pixels: m.bounds, Cols: cols, Rows: rows})
	session, spawnErr := m.factory.Spawn(ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		return 0, 0, nil, fmt.Errorf("spawn tab: %w", spawnErr)
	}
	pane.session = session
	pane.state = PaneStateRunning
	pane.geometry = effectiveGeometry(PaneGeometry{Pane: paneID, Pixels: m.bounds, Cols: cols, Rows: rows})
	pane.desiredSize = pty.Size{Rows: ptyRows, Cols: ptyCols}
	pane.appliedSize = pane.desiredSize
	if err := m.model.commitTab(tabID, paneID, title); err != nil {
		_ = pane.close()
		return 0, 0, nil, err
	}
	m.panes[paneID] = pane
	m.paneMetrics[paneID] = metrics
	pane.capture()
	pane.startReader(m.ctx, m.incoming, m.options.Wake, &m.readers)
	return tabID, paneID, []Event{{Kind: TabSpawned, Tab: tabID, Pane: paneID, Text: title, Revision: 1}, {Kind: TabActivated, Tab: tabID, Pane: paneID, Revision: 1}, {Kind: PaneStarted, Tab: tabID, Pane: paneID}, {Kind: PaneFocused, Tab: tabID, Pane: paneID}}, nil
}

func (m *Mux) ActivateTab(id TabID) ([]Event, error) {
	if err := m.model.ActivateTab(id); err != nil {
		return nil, err
	}
	focused := m.model.FocusedPane()
	return []Event{{Kind: TabActivated, Tab: id, Pane: focused}, {Kind: PaneFocused, Tab: id, Pane: focused}}, nil
}
func (m *Mux) RenameTab(id TabID, title string) ([]Event, error) {
	if err := m.model.RenameTab(id, title); err != nil {
		return nil, err
	}
	revision := m.model.tabByID(id).revision
	return []Event{{Kind: TabRenamed, Tab: id, Text: title, Revision: revision}, {Kind: TabRevisionChanged, Tab: id, Revision: revision}}, nil
}
func (m *Mux) MoveTab(id TabID, position int) ([]Event, error) {
	if err := m.model.MoveTab(id, position); err != nil {
		return nil, err
	}
	revision := m.model.tabByID(id).revision
	return []Event{{Kind: TabMoved, Tab: id, Data: []byte(fmt.Sprintf("%d", position)), Revision: revision}, {Kind: TabRevisionChanged, Tab: id, Revision: revision}}, nil
}

// CloseTab atomically detaches ownership before closing each session once.
func (m *Mux) CloseTab(id TabID) ([]Event, error) {
	detached, err := m.model.detachTab(id)
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(detached.panes)+3)
	var closeErrs []error
	for _, paneID := range detached.panes {
		p := m.panes[paneID]
		delete(m.panes, paneID)
		delete(m.paneMetrics, paneID)
		m.closed[paneID] = struct{}{}
		if p != nil {
			if err := p.close(); err != nil {
				closeErrs = append(closeErrs, fmt.Errorf("pane %d close: %w", paneID, err))
				events = append(events, Event{Kind: PaneCloseFailed, Tab: id, Pane: paneID, Err: err})
			}
		}
		events = append(events, Event{Kind: PaneClosed, Tab: id, Pane: paneID})
	}
	events = append(events, Event{Kind: TabClosed, Tab: id})
	if detached.active == 0 {
		events = append(events, Event{Kind: WindowTabsEmpty, Tab: id}, Event{Kind: TabEmpty, Tab: id})
	} else {
		events = append(events, Event{Kind: TabActivated, Tab: detached.active, Pane: detached.focused}, Event{Kind: PaneFocused, Tab: detached.active, Pane: detached.focused})
	}
	return events, errors.Join(closeErrs...)
}
