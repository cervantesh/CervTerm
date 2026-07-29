package mux

func (m *Mux) advancePaneScoped(scope mutationScope, p *pane, data []byte) []Event {
	if p == nil || scope.validPaneOrigin(m, p.id) != nil {
		return nil
	}
	p.activeScope = scope
	defer func() { p.activeScope = mutationScope{} }()
	oldTitle, oldCWD, oldBell := p.title, p.cwd, p.bellCount
	public := p.advanceTerminal(data)
	events := p.kittyEvents
	p.kittyEvents = nil
	if len(p.kittyOutcomes) != 0 {
		events = append(events, m.processKittyOutcomesScoped(scope, p)...)
	}
	if len(p.sixelOutcomes) != 0 {
		m.processSixelOutcomesScoped(scope, p)
	}
	if len(p.itermOutcomes) != 0 {
		m.processITermOutcomesScoped(scope, p)
	}
	events = append(events, p.flushReplies()...)
	p.capture()
	events = append(events,
		Event{Kind: PaneOutput, Pane: p.id, Data: append([]byte(nil), public...), BytesRead: len(data)},
		Event{Kind: PaneDirty, Pane: p.id},
	)
	if p.title != oldTitle {
		events = append(events, Event{Kind: PaneTitleChanged, Pane: p.id, Text: p.title})
	}
	if p.cwd != oldCWD {
		events = append(events, Event{Kind: PaneCWDChanged, Pane: p.id, Text: p.cwd})
	}
	for bell := oldBell; bell < p.bellCount; bell++ {
		events = append(events, Event{Kind: PaneBell, Pane: p.id})
	}
	events = appendPaneNotificationEvents(events, p)
	return m.ResolveEventAddresses(events)
}
