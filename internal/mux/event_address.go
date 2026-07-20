package mux

// ResolveEventAddresses fills missing window/workspace coordinates while the
// referenced topology is still owned. Explicit producer addresses always win.
func (m *Mux) ResolveEventAddresses(events []Event) []Event {
	for i := range events {
		event := &events[i]
		if event.Window == 0 {
			if event.Pane != 0 {
				event.Window, _ = m.WindowForPane(event.Pane)
			}
			if event.Window == 0 && event.Tab != 0 {
				event.Window, _ = m.WindowForTab(event.Tab)
			}
		}
		if event.Workspace == 0 && event.Window != 0 {
			event.Workspace, _ = m.WorkspaceForWindow(event.Window)
		}
		if event.SourceWorkspace == 0 && event.SourceWindow != 0 {
			event.SourceWorkspace, _ = m.WorkspaceForWindow(event.SourceWindow)
		}
	}
	return events
}
