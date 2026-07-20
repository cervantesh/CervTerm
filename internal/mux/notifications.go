package mux

func appendPaneNotificationEvents(events []Event, p *pane) []Event {
	requests, latest, truncated := p.terminal.NotificationRequestsSince(p.notificationSeq, p.notificationScratch[:0])
	p.notificationScratch = requests
	p.notificationSeq = latest
	if truncated {
		events = append(events, Event{Kind: PaneNotificationOverflow, Pane: p.id, Revision: latest})
	}
	for _, request := range requests {
		events = append(events, Event{Kind: PaneNotificationRequested, Pane: p.id, Notification: request, Revision: request.Sequence, Fresh: true})
	}
	return events
}
