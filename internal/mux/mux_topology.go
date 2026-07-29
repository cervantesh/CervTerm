package mux

// ResizeCurrentPane grows the focused pane toward direction by delta terminal
// cells. Geometry and desired PTY sizes are updated atomically; session Resize
// remains deferred to ApplyResize.
func (m *Mux) resizeCurrentPane(scope mutationScope, direction Direction, delta int) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	focused := m.model.FocusedPane()
	if focused == 0 {
		return nil, ErrEmptyModel
	}
	if err := m.model.ResizePaneDirection(focused, direction, delta, m.bounds, m.resolveMetrics); err != nil {
		return nil, err
	}
	return m.applyCurrentLayout(scope)
}

// SwapCurrentPane exchanges the focused pane with its directional neighbor.
// Focus remains at the original visual slot and therefore transfers to the
// neighbor pane identity.
func (m *Mux) swapCurrentPane(scope mutationScope, direction Direction) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	focused := m.model.FocusedPane()
	if focused == 0 {
		return nil, ErrEmptyModel
	}
	newFocus, err := m.model.SwapPaneDirection(focused, direction, m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	events, err := m.applyCurrentLayout(scope)
	if err != nil {
		return nil, err
	}
	return append([]Event{{Kind: PaneFocused, Pane: newFocus}}, events...), nil
}

// MoveCurrentPane reorders the focused pane with its directional neighbor while
// focus follows the moved pane identity.
func (m *Mux) moveCurrentPane(scope mutationScope, direction Direction) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	focused := m.model.FocusedPane()
	if focused == 0 {
		return nil, ErrEmptyModel
	}
	if err := m.model.MovePaneDirection(focused, direction, m.bounds, m.resolveMetrics); err != nil {
		return nil, err
	}
	return m.applyCurrentLayout(scope)
}

func (m *Mux) applyCurrentLayout(scope mutationScope) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	layout, err := m.model.LayoutWithMetrics(m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	return m.applyLayout(scope, layout)
}
