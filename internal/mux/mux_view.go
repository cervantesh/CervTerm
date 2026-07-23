package mux

import "cervterm/internal/core"

func (m *Mux) SearchUpward(id PaneID, query string, hasPrev bool, prevRow int) (row, col int, ok bool, err error) {
	p, exists := m.sessions.lookup(id)
	if !exists || !m.model.paneExists(id) {
		return 0, 0, false, ErrPaneNotFound
	}
	from := p.terminal.ScrollbackLines() + p.terminal.Rows()
	if hasPrev {
		from = prevRow
	}
	row, col, ok = p.terminal.SearchBackward(query, from)
	if ok {
		oldOffset := p.terminal.DisplayOffset()
		scrollGlobalRowIntoView(p.terminal, row)
		if p.terminal.DisplayOffset() != oldOffset {
			p.viewportGen++
		}
		p.capture()
	}
	return row, col, ok, nil
}

func scrollGlobalRowIntoView(t *core.Terminal, row int) {
	if _, ok := t.GlobalRowToViewport(row); ok {
		return
	}
	targetTop := max(0, row-t.Rows()/2)
	t.ScrollViewport((t.ScrollbackLines() - targetTop) - t.DisplayOffset())
}

func (m *Mux) GlobalRowToViewport(id PaneID, row int) (int, bool) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return 0, false
	}
	return p.terminal.GlobalRowToViewport(row)
}

func (m *Mux) SetTitle(id PaneID, title string) (bool, error) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return false, ErrPaneNotFound
	}
	if p.terminal.Title() == title {
		return false, nil
	}
	p.terminal.SetTitle(title)
	p.capture()
	return true, nil
}

func (m *Mux) Line(id PaneID, row int) (string, bool) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) || row < 0 || row >= p.terminal.Rows() {
		return "", false
	}
	cols, rows := p.terminal.Cols(), p.terminal.Rows()
	cells := make([]core.Cell, cols*rows)
	p.terminal.CopyView(cells)
	start := row * cols
	return core.RowText(cells[start : start+cols]), true
}

func (m *Mux) LineWrapped(id PaneID, row int) (bool, bool) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return false, false
	}
	return p.terminal.LineWrapped(row)
}
