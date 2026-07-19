package mux

import "cervterm/internal/core"

const (
	MaxQuickSelectSnapshotRows  = 256
	MaxQuickSelectSnapshotCells = 32 * 1024
)

// QuickSelectSnapshot is a bounded, detached view of terminal cells. It is
// internal to CervTerm (the mux package itself is internal) and has no action or
// configuration surface.
type QuickSelectSnapshot struct {
	PaneID          PaneID
	FocusedPaneID   PaneID
	GlobalRowOrigin int
	Cols            int
	Rows            int
	Cells           []core.Cell
	Wrapped         []bool
	ContentGen      uint64
	ReflowGen       uint64
	ViewportGen     uint64
}

// QuickSelectSnapshot returns at most the requested visible rows and cells.
// Non-positive limits select the package caps. It copies only the bounded
// viewport suffix, never terminal history.
func (m *Mux) QuickSelectSnapshot(id PaneID, rowLimit, cellLimit int) (QuickSelectSnapshot, bool) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return QuickSelectSnapshot{}, false
	}
	rowLimit = boundedLimit(rowLimit, MaxQuickSelectSnapshotRows)
	cellLimit = boundedLimit(cellLimit, MaxQuickSelectSnapshotCells)
	cols := p.terminal.Cols()
	rows := min(p.terminal.Rows(), rowLimit)
	if cols <= 0 || cellLimit < cols {
		rows = 0
	} else {
		rows = min(rows, cellLimit/cols)
	}
	first := p.terminal.Rows() - rows
	cells := make([]core.Cell, rows*cols)
	p.terminal.CopyViewRows(cells, first, rows)
	cells = cloneDetachedCells(cells)
	wrapped := make([]bool, rows)
	for row := range wrapped {
		wrapped[row], _ = p.terminal.LineWrapped(first + row)
	}
	return QuickSelectSnapshot{
		PaneID: p.id, FocusedPaneID: m.model.FocusedPane(),
		GlobalRowOrigin: p.terminal.ViewportTopGlobalRow() + first,
		Cols:            cols, Rows: rows, Cells: cells, Wrapped: wrapped,
		ContentGen: p.contentGen, ReflowGen: p.reflowGen, ViewportGen: p.viewportGen,
	}, true
}

// QuickSelectSnapshotCurrent reports whether a retained result is still safe to
// act on. Focus, pane identity, geometry and every relevant generation must match.
func (m *Mux) QuickSelectSnapshotCurrent(s QuickSelectSnapshot) bool {
	p, ok := m.sessions.lookup(s.PaneID)
	return ok && m.model.paneExists(s.PaneID) &&
		m.model.FocusedPane() == s.FocusedPaneID && s.FocusedPaneID == s.PaneID &&
		p.terminal.Cols() == s.Cols && p.terminal.Rows() >= s.Rows &&
		p.contentGen == s.ContentGen && p.reflowGen == s.ReflowGen &&
		p.viewportGen == s.ViewportGen
}

func boundedLimit(value, capValue int) int {
	if value <= 0 || value > capValue {
		return capValue
	}
	return value
}

func cloneDetachedCells(src []core.Cell) []core.Cell {
	out := make([]core.Cell, len(src))
	for i, cell := range src {
		marks := cell.CloneCombining()
		out[i] = core.NewCellWithCombining(cell.Rune, cell.Attr, marks...)
		out[i].WideContinuation = cell.WideContinuation
	}
	return out
}

func (p *pane) advanceTerminal(data []byte) {
	oldOffset := p.terminal.DisplayOffset()
	p.parser.Advance(p.terminal, data)
	p.contentGen++
	if p.terminal.DisplayOffset() != oldOffset {
		p.viewportGen++
	}
}
