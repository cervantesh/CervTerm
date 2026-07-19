package mux

import "cervterm/internal/core"

type SemanticSnapshot struct {
	PaneID, FocusedPaneID              PaneID
	Ranges                             []core.SemanticRange
	HistoryTruncated                   bool
	ViewportTopGlobalRow               int
	TotalRows                          int
	CursorGlobalRow                    int
	Rows                               int
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
		CursorGlobalRow: p.terminal.ScrollbackLines() + p.terminal.CursorRow(), Rows: p.terminal.Rows(),
		ContentGen: p.contentGen, ReflowGen: p.reflowGen, ViewportGen: p.viewportGen,
	}, true
}

func (m *Mux) SemanticSnapshotCurrent(snapshot SemanticSnapshot) bool {
	p, ok := m.sessions.lookup(snapshot.PaneID)
	return ok && m.model.paneExists(snapshot.PaneID) && m.model.FocusedPane() == snapshot.FocusedPaneID &&
		p.contentGen == snapshot.ContentGen &&
		p.reflowGen == snapshot.ReflowGen && p.viewportGen == snapshot.ViewportGen
}

func (m *Mux) SemanticRangeText(snapshot SemanticSnapshot, target core.SemanticRange) (string, error) {
	if !m.SemanticSnapshotCurrent(snapshot) {
		return "", ErrSemanticSnapshotStale
	}
	found := false
	for _, candidate := range snapshot.Ranges {
		if candidate == target {
			found = true
			break
		}
	}
	if !found {
		return "", ErrSemanticRangeUnavailable
	}
	p, ok := m.sessions.lookup(snapshot.PaneID)
	if !ok || !m.model.paneExists(snapshot.PaneID) {
		return "", ErrPaneNotFound
	}
	currentRanges, _ := p.terminal.SemanticHistory()
	currentFound := false
	for _, candidate := range currentRanges {
		if candidate == target {
			currentFound = true
			break
		}
	}
	if !currentFound {
		return "", ErrSemanticRangeUnavailable
	}
	text, ok := p.terminal.SemanticRangeText(target)
	if !ok {
		return "", ErrSemanticRangeUnavailable
	}
	return text, nil
}
