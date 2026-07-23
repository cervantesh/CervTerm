package mux

func (m *Mux) Shutdown() error {
	if m.imageScheduler != nil {
		m.imageScheduler.close()
		m.imageScheduler = nil
	}
	clear(m.kittyPending)
	clear(m.sixelPending)
	clear(m.itermPending)
	return m.sessions.shutdownRegistry()
}
