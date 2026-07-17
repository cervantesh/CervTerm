package mux

import "cervterm/internal/core"

// SetPaletteBase updates the configured palette beneath every pane-local OSC override.
func (m *Mux) SetPaletteBase(base core.PaletteBase) {
	m.paletteBase = base
	for _, pane := range m.panes {
		pane.terminal.SetPaletteBase(base)
	}
}
