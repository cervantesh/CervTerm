package mux

func (m *Mux) shutdown(scope mutationScope) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	prepared, err := m.sessions.prepareShutdownScoped(scope)
	if err != nil {
		return err
	}
	if m.imageScheduler != nil {
		m.imageScheduler.closeScoped(scope, m)
		m.imageScheduler = nil
	}
	clear(m.kittyPending)
	clear(m.sixelPending)
	clear(m.itermPending)
	return prepared.commit()
}

func (m *Mux) shutdownCommitted() bool {
	if m == nil {
		return false
	}
	m.sessions.mu.Lock()
	defer m.sessions.mu.Unlock()
	return m.sessions.shutdown
}
