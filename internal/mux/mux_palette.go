package mux

import "cervterm/internal/core"

// setPaletteBase updates the configured palette beneath every pane-local OSC override.
func (m *Mux) setPaletteBase(scope mutationScope, base core.PaletteBase) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	m.paletteBase = &base
	m.sessions.forEach(func(id PaneID, pane *pane) {
		if m.restorePanePending(id) {
			return
		}
		pane.terminal.SetPaletteBase(base)
	})
	return nil
}
