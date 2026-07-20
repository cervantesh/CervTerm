package mux

// ScrollViewport moves one pane's viewport and refreshes its immutable snapshot.
func (m *Mux) ScrollViewport(id PaneID, lines int) (bool, error) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return false, ErrPaneNotFound
	}
	moved := p.terminal.ScrollViewport(lines)
	if moved {
		p.viewportGen++
		p.capture()
	}
	return moved, nil
}

func (m *Mux) ScrollViewportToGlobalRow(id PaneID, globalRow int) (bool, error) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return false, ErrPaneNotFound
	}
	moved := p.terminal.ScrollViewportToGlobalRow(globalRow)
	if moved {
		p.viewportGen++
		p.capture()
	}
	return moved, nil
}

// SetScrollbackCapacity applies a live history-capacity change to every active pane.
// Mux remains the owner of pane terminals; frontends never reach into pane state.
func (m *Mux) SetScrollbackCapacity(capacity int) {
	capacityCopy := capacity
	m.options.ScrollbackCapacity = &capacityCopy
	for _, id := range m.model.PaneIDs() {
		p, ok := m.sessions.lookup(id)
		if !ok {
			continue
		}
		oldOffset := p.terminal.DisplayOffset()
		p.terminal.SetScrollbackCapacity(capacity)
		p.contentGen++
		if p.terminal.DisplayOffset() != oldOffset {
			p.viewportGen++
		}
		p.capture()
	}
}

// SetHideCursorWhenScrolled updates snapshot policy for active and future panes.
func (m *Mux) SetHideCursorWhenScrolled(hide bool) {
	hideCopy := hide
	m.options.HideCursorWhenScrolled = &hideCopy
	for _, id := range m.model.PaneIDs() {
		p, ok := m.sessions.lookup(id)
		if !ok {
			continue
		}
		p.captureOptions.HideCursorWhenScrolled = hide
		p.capture()
	}
}
