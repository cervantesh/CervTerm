package mux

func (m *Mux) advancePane(p *pane, data []byte) []Event {
	oldTitle, oldCWD, oldBell := p.title, p.cwd, p.bellCount
	public := p.advanceTerminal(data)
	events := p.kittyEvents
	p.kittyEvents = nil
	events = append(events, m.processKittyOutcomes(p)...)
	m.processSixelOutcomes(p)
	m.processITermOutcomes(p)
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
