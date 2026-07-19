package mux

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"cervterm/internal/pty"
)

// localSessionRegistry is the closed process-local spawn and session ownership seam.
// It deliberately has no window geometry or remote-domain extension points.
type localSessionRegistry struct {
	mu sync.Mutex

	factory  SessionFactory
	panes    map[PaneID]*pane
	reserved map[PaneID]struct{}
	closed   map[PaneID]struct{}
	started  map[PaneID]struct{}

	shuttingDown bool
	shutdown     bool
	shutdownDone chan struct{}
	shutdownErr  error

	incoming chan ingressRecord
	ctx      context.Context
	cancel   context.CancelFunc
	readers  sync.WaitGroup
	wake     func()
}

type detachResult struct {
	pane  *pane
	owned bool
}

func newLocalSessionRegistry(factory SessionFactory, capacity int, wake func()) *localSessionRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &localSessionRegistry{
		factory: factory, panes: make(map[PaneID]*pane), reserved: make(map[PaneID]struct{}),
		closed: make(map[PaneID]struct{}), started: make(map[PaneID]struct{}),
		shutdownDone: make(chan struct{}), incoming: make(chan ingressRecord, capacity),
		ctx: ctx, cancel: cancel, wake: wake,
	}
}

func (r *localSessionRegistry) reserve(id PaneID) error {
	if id == 0 {
		return invariantError("cannot reserve zero pane")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shuttingDown {
		return ErrShuttingDown
	}
	if _, exists := r.panes[id]; exists {
		return invariantError("pane %d is already registered", id)
	}
	if _, exists := r.reserved[id]; exists {
		return invariantError("pane %d is already reserved", id)
	}
	if _, closed := r.closed[id]; closed {
		return invariantError("closed pane %d cannot be reserved again", id)
	}
	r.reserved[id] = struct{}{}
	return nil
}

func (r *localSessionRegistry) release(id PaneID) {
	r.mu.Lock()
	delete(r.reserved, id)
	r.mu.Unlock()
}

func (r *localSessionRegistry) spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		return nil, ErrShuttingDown
	}
	factory := r.factory
	r.mu.Unlock()

	session, err := factory.Spawn(rows, cols, options)

	r.mu.Lock()
	shuttingDown := r.shuttingDown
	r.mu.Unlock()
	if shuttingDown {
		if session != nil {
			_ = session.Close()
		}
		return nil, ErrShuttingDown
	}
	return session, err
}

func (r *localSessionRegistry) register(p *pane) error {
	if p == nil || p.id == 0 {
		return invariantError("cannot register nil or zero pane")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shuttingDown {
		return ErrShuttingDown
	}
	if _, reserved := r.reserved[p.id]; !reserved {
		return invariantError("pane %d is not reserved", p.id)
	}
	if _, exists := r.panes[p.id]; exists {
		return invariantError("pane %d is already registered", p.id)
	}
	if _, closed := r.closed[p.id]; closed {
		return invariantError("closed pane %d cannot be registered again", p.id)
	}
	delete(r.reserved, p.id)
	r.panes[p.id] = p
	return nil
}

// start begins a reader while holding the registry lock across WaitGroup.Add.
// Once a pane is registered this operation is mechanically infallible unless
// shutdown has begun.
func (r *localSessionRegistry) start(id PaneID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shuttingDown {
		return ErrShuttingDown
	}
	p := r.panes[id]
	if p == nil {
		return invariantError("pane %d is not registry-owned", id)
	}
	if p.session == nil {
		return ErrPaneNotRunning
	}
	if _, started := r.started[id]; started {
		return invariantError("pane %d reader is already started", id)
	}
	r.started[id] = struct{}{}
	p.startReader(r.ctx, r.incoming, r.wake, &r.readers)
	return nil
}

func (r *localSessionRegistry) lookup(id PaneID) (*pane, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.panes[id]
	return p, ok
}

func (r *localSessionRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.panes)
}

func (r *localSessionRegistry) forEach(fn func(PaneID, *pane)) {
	r.mu.Lock()
	entries := make([]struct {
		id PaneID
		p  *pane
	}, 0, len(r.panes))
	for id, p := range r.panes {
		entries = append(entries, struct {
			id PaneID
			p  *pane
		}{id, p})
	}
	r.mu.Unlock()
	for _, entry := range entries {
		fn(entry.id, entry.p)
	}
}

func (r *localSessionRegistry) wasClosed(id PaneID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.closed[id]
	return ok
}

func (r *localSessionRegistry) factoryForTest() SessionFactory {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.factory
}

func (r *localSessionRegistry) detach(id PaneID) detachResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, owned := r.panes[id]
	if !owned {
		return detachResult{}
	}
	delete(r.panes, id)
	delete(r.started, id)
	r.closed[id] = struct{}{}
	return detachResult{pane: p, owned: true}
}

// abort detaches an unpublished pane without recording its proposed identity
// as closed. Stale reader ingress is rejected by owner identity in Mux.Drain.
func (r *localSessionRegistry) abort(id PaneID, expected *pane) detachResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, owned := r.panes[id]
	if !owned || p != expected {
		return detachResult{}
	}
	delete(r.panes, id)
	delete(r.started, id)
	delete(r.reserved, id)
	return detachResult{pane: p, owned: true}
}

func (r *localSessionRegistry) shutdownRegistry() error {
	r.mu.Lock()
	if r.shuttingDown {
		done := r.shutdownDone
		r.mu.Unlock()
		<-done
		r.mu.Lock()
		err := r.shutdownErr
		r.mu.Unlock()
		return err
	}
	r.shuttingDown = true
	r.cancel()
	panes := make([]*pane, 0, len(r.panes))
	for id, p := range r.panes {
		panes = append(panes, p)
		delete(r.panes, id)
		delete(r.started, id)
		r.closed[id] = struct{}{}
	}
	r.mu.Unlock()

	var closeErrors []error
	for _, p := range panes {
		if err := p.close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("pane %d close: %w", p.id, err))
		}
	}
	r.readers.Wait()
	err := errors.Join(closeErrors...)

	r.mu.Lock()
	r.shutdownErr = err
	r.shutdown = true
	close(r.shutdownDone)
	r.mu.Unlock()
	return err
}
