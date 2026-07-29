package mux

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"cervterm/internal/pty"
)

// localSessionRegistry is the closed process-local spawn and session ownership seam.
// It deliberately has no window geometry or remote-domain extension points.
type localSessionRegistry struct {
	mu    sync.Mutex
	owner *Mux

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

type sessionIngressRecordAdapter struct {
	record     ingressRecord
	registered *pane
	found      bool
}

var _ sessionIngressOwnerPort = sessionIngressRecordAdapter{}

// adaptSessionIngressRecord runs only from the serialized Mux.Drain mutation.
// Session readers enqueue immutable records but never mutate the registry, so
// live registry revalidation is a lock-free owner-thread map lookup.
func (r *localSessionRegistry) adaptSessionIngressRecord(record ingressRecord) sessionIngressRecordAdapter {
	registered, found := r.panes[record.pane]
	return sessionIngressRecordAdapter{record: record, registered: registered, found: found}
}

// lookupOwned is valid only after an active dispatch scope has been checked.
// Owner serialization makes the registry map stable for the synchronous call.
func (r *localSessionRegistry) lookupOwned(id PaneID) (*pane, bool) {
	p, ok := r.panes[id]
	return p, ok
}

func (a sessionIngressRecordAdapter) acceptSessionIngress() bool {
	return a.found && a.registered == a.record.owner && a.registered.ownerStamp == a.record.stamp &&
		a.record.stamp.live() == nil && a.registered.state != PaneStateClosed && a.registered.state != PaneStateClosing
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

func (r *localSessionRegistry) validateMutation(scope mutationScope) error {
	if r == nil || r.owner == nil {
		return ErrWrongOwner
	}
	return scope.valid(r.owner)
}

func (r *localSessionRegistry) reserveScoped(scope mutationScope, id PaneID) error {
	if err := r.validateMutation(scope); err != nil {
		return err
	}
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

func (r *localSessionRegistry) releaseScoped(scope mutationScope, id PaneID) {
	if r == nil || r.validateMutation(scope) != nil {
		return
	}
	r.mu.Lock()
	delete(r.reserved, id)
	r.mu.Unlock()
}

func (r *localSessionRegistry) spawnScoped(scope mutationScope, rows, cols uint16, options pty.Options) (pty.Session, error) {
	if err := r.validateMutation(scope); err != nil {
		return nil, err
	}
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

func (r *localSessionRegistry) registerScoped(scope mutationScope, p *pane) error {
	if err := r.validateMutation(scope); err != nil {
		return err
	}
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

// prepareStarts validates a future reader launch without publishing started state
// or reserving WaitGroup work. Mux owner serialization keeps the captured panes
// stable until the returned closure runs after model publication; discarding the
// closure therefore provides bounded cancellation for unpublished rollback.
func (r *localSessionRegistry) prepareStartsScoped(scope mutationScope, ids []PaneID) (func(), error) {
	if err := r.validateMutation(scope); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shuttingDown {
		return nil, ErrShuttingDown
	}
	panes := make([]*pane, len(ids))
	seen := make(map[PaneID]struct{}, len(ids))
	for i, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return nil, invariantError("pane %d reader requested twice", id)
		}
		seen[id] = struct{}{}
		p := r.panes[id]
		if p == nil {
			return nil, invariantError("pane %d is not registry-owned", id)
		}
		if p.session == nil {
			return nil, ErrPaneNotRunning
		}
		if _, started := r.started[id]; started {
			return nil, invariantError("pane %d reader is already started", id)
		}
		panes[i] = p
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			for _, p := range panes {
				r.started[p.id] = struct{}{}
			}
			r.readers.Add(len(panes))
			r.mu.Unlock()
			for _, p := range panes {
				p.launchReader(r.ctx, r.incoming, r.wake, &r.readers)
			}
		})
	}, nil
}

func (r *localSessionRegistry) startScoped(scope mutationScope, id PaneID) error {
	launch, err := r.prepareStartsScoped(scope, []PaneID{id})
	if err != nil {
		return err
	}
	launch()
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

func (r *localSessionRegistry) activeCounts() (panes, reserved, started int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.panes), len(r.reserved), len(r.started)
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

func (r *localSessionRegistry) detachScoped(scope mutationScope, id PaneID) detachResult {
	if r == nil || r.validateMutation(scope) != nil {
		return detachResult{}
	}
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
func (r *localSessionRegistry) abortScoped(scope mutationScope, id PaneID, expected *pane) detachResult {
	if r == nil || r.validateMutation(scope) != nil {
		return detachResult{}
	}
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

type preparedRegistryShutdown struct {
	registry     *localSessionRegistry
	panes        []*pane
	closes       []*preparedPaneClose
	scope        mutationScope
	completed    bool
	completedErr error
}

func (r *localSessionRegistry) prepareShutdownScoped(scope mutationScope) (*preparedRegistryShutdown, error) {
	if err := r.validateMutation(scope); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.shuttingDown {
		done := r.shutdownDone
		r.mu.Unlock()
		<-done
		r.mu.Lock()
		err := r.shutdownErr
		r.mu.Unlock()
		return &preparedRegistryShutdown{registry: r, scope: scope, completed: true, completedErr: err}, nil
	}
	ids := make([]PaneID, 0, len(r.panes))
	for id := range r.panes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	panes := make([]*pane, 0, len(ids))
	for _, id := range ids {
		panes = append(panes, r.panes[id])
	}
	r.mu.Unlock()
	closes, err := preparePaneClosures(panes)
	if err != nil {
		return nil, err
	}
	return &preparedRegistryShutdown{registry: r, scope: scope, panes: panes, closes: closes}, nil
}

func (p *preparedRegistryShutdown) abort() error {
	if p != nil && p.registry != nil {
		if err := p.scope.valid(p.registry.owner); err != nil {
			return err
		}
	}
	if p == nil || p.completed {
		return nil
	}
	p.completed = true
	return abortPaneClosures(p.closes)
}

func (p *preparedRegistryShutdown) commit() error {
	if p == nil || p.registry == nil {
		return ErrInvariant
	}
	if err := p.scope.valid(p.registry.owner); err != nil {
		return err
	}
	if p.completed {
		return p.completedErr
	}
	r := p.registry
	r.mu.Lock()
	if r.shuttingDown {
		done := r.shutdownDone
		r.mu.Unlock()
		abortErr := p.abort()
		<-done
		r.mu.Lock()
		err := r.shutdownErr
		r.mu.Unlock()
		return errors.Join(err, abortErr)
	}
	if len(r.panes) != len(p.panes) {
		r.mu.Unlock()
		return errors.Join(invariantError("registry changed after shutdown preflight"), p.abort())
	}
	for _, owned := range p.panes {
		if current, ok := r.panes[owned.id]; !ok || current != owned {
			r.mu.Unlock()
			return errors.Join(invariantError("pane %d changed after shutdown preflight", owned.id), p.abort())
		}
	}
	r.shuttingDown = true
	r.cancel()
	for _, owned := range p.panes {
		delete(r.panes, owned.id)
		delete(r.started, owned.id)
		r.closed[owned.id] = struct{}{}
	}
	r.mu.Unlock()

	var closeErrors []error
	for index, closeState := range p.closes {
		if err := closeState.commit(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("pane %d close: %w", p.panes[index].id, err))
		}
	}
	r.readers.Wait()
	err := errors.Join(closeErrors...)

	r.mu.Lock()
	r.shutdownErr = err
	r.shutdown = true
	close(r.shutdownDone)
	r.mu.Unlock()
	p.completed = true
	p.completedErr = err
	return err
}

func (r *localSessionRegistry) shutdownRegistryScoped(scope mutationScope) error {
	prepared, err := r.prepareShutdownScoped(scope)
	if err != nil {
		return err
	}
	return prepared.commit()
}
