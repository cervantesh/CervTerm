package mux

import "cervterm/internal/core"

// SetPaletteBase updates the configured palette beneath every pane-local OSC override.
func (m *Mux) SetPaletteBase(base core.PaletteBase) {
	m.paletteBase = base
	m.sessions.forEach(func(id PaneID, pane *pane) {
		if m.restorePanePending(id) {
			return
		}
		pane.terminal.SetPaletteBase(base)
	})
}
