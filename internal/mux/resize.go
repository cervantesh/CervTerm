package mux

import (
	"errors"
	"fmt"

	"cervterm/internal/pty"
)

func (m *Mux) resolveMetrics(id PaneID) (CellMetrics, bool) {
	metrics, ok := m.paneMetrics[id]
	return metrics, ok
}

// SetSplitRatio updates one branch and immediately reflows pane grids using
// stored per-pane metrics while leaving PTY notification to the frontend's
// drag-settlement boundary.
func (m *Mux) SetSplitRatio(split SplitID, ratio SplitRatio) ([]Event, error) {
	if err := m.model.SetSplitRatioWithMetrics(split, ratio, m.bounds, m.resolveMetrics); err != nil {
		return nil, err
	}
	return m.ResizeBounds(m.bounds)
}

// Resize updates every pane to uniform metrics before attempting PTY resizes.
func (m *Mux) Resize(content PixelRect, metrics CellMetrics) ([]Event, error) {
	events, err := m.ResizeGrid(content, metrics)
	if err != nil {
		return events, err
	}
	resizeEvents, resizeErr := m.applyDesiredResizes()
	return append(events, resizeEvents...), resizeErr
}

// ResizeGrid updates pane geometry, terminal grids, snapshots, desired PTY
// sizes, and all stored pane metrics to one uniform value without notifying
// sessions. It preserves the compatibility semantics of the original API.
func (m *Mux) ResizeGrid(content PixelRect, metrics CellMetrics) ([]Event, error) {
	layout, err := m.model.Layout(content, metrics)
	if err != nil {
		return nil, err
	}
	m.bounds = content
	for _, id := range m.model.PaneIDs() {
		m.paneMetrics[id] = metrics
	}
	return m.applyLayout(layout)
}

// ResizeBounds recomputes layout for new bounds using each pane's stored
// metrics, preserving per-pane zoom and leaving PTY notification deferred.
func (m *Mux) ResizeBounds(content PixelRect) ([]Event, error) {
	layout, err := m.model.LayoutWithMetrics(content, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	m.bounds = content
	return m.applyLayout(layout)
}

// ResizePaneGrid updates one pane's stored metrics and recomputes authoritative
// geometry, terminal grids, snapshots, and desired PTY sizes for the full layout.
// Sessions are not notified until ApplyResize is called.
func (m *Mux) ResizePaneGrid(id PaneID, metrics CellMetrics) ([]Event, error) {
	if !m.model.paneExists(id) || m.panes[id] == nil {
		return nil, ErrPaneNotFound
	}
	if err := validateCellMetrics(metrics); err != nil {
		return nil, err
	}
	resolve := func(pane PaneID) (CellMetrics, bool) {
		if pane == id {
			return metrics, true
		}
		return m.resolveMetrics(pane)
	}
	layout, err := m.model.LayoutWithMetrics(m.bounds, resolve)
	if err != nil {
		return nil, err
	}
	m.paneMetrics[id] = metrics
	return m.applyLayout(layout)
}

func (m *Mux) resizeBoundsAndApply(content PixelRect) ([]Event, error) {
	events, err := m.ResizeBounds(content)
	if err != nil {
		return events, err
	}
	resizeEvents, resizeErr := m.applyDesiredResizes()
	return append(events, resizeEvents...), resizeErr
}

func (m *Mux) applyLayout(layout Layout) ([]Event, error) {
	events := make([]Event, 0, len(layout.Panes)*2)
	for _, geometry := range layout.Panes {
		geometry = effectiveGeometry(geometry)
		p := m.panes[geometry.Pane]
		if p == nil {
			return events, invariantError("layout references unattached pane %d", geometry.Pane)
		}
		if p.geometry == geometry {
			continue
		}
		p.geometry = geometry
		oldOffset := p.terminal.DisplayOffset()
		p.terminal.Resize(geometry.Cols, geometry.Rows)
		p.reflowGen++
		if p.terminal.DisplayOffset() != oldOffset {
			p.viewportGen++
		}
		p.capture()
		rows, cols := terminalSize(geometry)
		p.desiredSize = pty.Size{Rows: rows, Cols: cols}
		events = append(events, Event{Kind: PaneGeometryChanged, Pane: p.id, Geometry: geometry}, Event{Kind: PaneDirty, Pane: p.id})
	}
	return events, nil
}

func (m *Mux) applyDesiredResizes() ([]Event, error) {
	var events []Event
	var resizeErrors []error
	for _, id := range m.model.PaneIDs() {
		paneEvents, paneErr := m.ApplyResize(id)
		events = append(events, paneEvents...)
		if paneErr != nil {
			resizeErrors = append(resizeErrors, paneErr)
		}
	}
	return events, errors.Join(resizeErrors...)
}

// ApplyResize notifies one pane session of the latest desired grid size.
func (m *Mux) ApplyResize(id PaneID) ([]Event, error) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return nil, ErrPaneNotFound
	}
	if p.session == nil || (p.state != PaneStateRunning && p.state != PaneStateExited) {
		return nil, nil
	}
	if p.resizeErr == nil && p.appliedSize == p.desiredSize {
		return nil, nil
	}
	if err := p.session.Resize(p.desiredSize); err != nil {
		p.resizeErr = err
		wrapped := fmt.Errorf("pane %d resize: %w", p.id, err)
		return []Event{{Kind: PaneResizeFailed, Pane: p.id, Err: err}}, wrapped
	}
	p.appliedSize = p.desiredSize
	p.resizeErr = nil
	return nil, nil
}
