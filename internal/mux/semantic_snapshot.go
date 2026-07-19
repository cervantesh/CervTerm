package mux

import "cervterm/internal/core"

type SemanticSnapshot struct {
	PaneID, FocusedPaneID              PaneID
	Ranges                             []core.SemanticRange
	HistoryTruncated                   bool
	ViewportTopGlobalRow               int
	TotalRows                          int
	ContentGen, ReflowGen, ViewportGen uint64
}

func (m *Mux) SemanticSnapshot(id PaneID) (SemanticSnapshot, bool) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return SemanticSnapshot{}, false
	}
	ranges, truncated := p.terminal.SemanticHistory()
	return SemanticSnapshot{
		PaneID: id, FocusedPaneID: m.model.FocusedPane(), Ranges: ranges, HistoryTruncated: truncated,
		ViewportTopGlobalRow: p.terminal.ViewportTopGlobalRow(), TotalRows: p.terminal.ScrollbackLines() + p.terminal.Rows(),
		ContentGen: p.contentGen, ReflowGen: p.reflowGen, ViewportGen: p.viewportGen,
	}, true
}

func (m *Mux) SemanticSnapshotCurrent(snapshot SemanticSnapshot) bool {
	p, ok := m.sessions.lookup(snapshot.PaneID)
	return ok && m.model.paneExists(snapshot.PaneID) && m.model.FocusedPane() == snapshot.FocusedPaneID &&
		snapshot.FocusedPaneID == snapshot.PaneID && p.contentGen == snapshot.ContentGen &&
		p.reflowGen == snapshot.ReflowGen && p.viewportGen == snapshot.ViewportGen
}
