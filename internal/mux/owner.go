package mux

import (
	"runtime"
	"sync/atomic"
)

// noCopy makes accidental Owner value copies visible to go vet. Owner methods
// additionally verify pointer identity, so a copied value is not a capability.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type ownerState struct {
	owner       *Owner
	generation  atomic.Uint64
	activeScope atomic.Uint64
	nextScope   atomic.Uint64
	closed      atomic.Bool
	closeErr    error
}

// Owner is the process-local mux owner. NewOwner binds it to the caller's exact
// native thread; each mutation additionally receives a one-dispatch scope.
// Pointer possession alone is never mutation authority, and Owner never exposes
// the concrete Mux.
type Owner struct {
	_            noCopy
	mux          *Mux
	state        ownerState
	generation   uint64
	threadID     uint64
	threadLocked atomic.Bool
}

// ownerStamp binds prepared work to one exact owner generation.
type ownerStamp struct {
	state      *ownerState
	generation uint64
}

// mutationScope is valid only for one active dispatch on the exact native owner
// thread. Copies are harmless: leave invalidates every copy by clearing nonce.
type mutationScope struct {
	stamp    ownerStamp
	nonce    uint64
	threadID uint64
	origin   WindowIdentity
}

// NewOwner creates the sole production constructor for the process-owned mux.
// The caller stays pinned until successful Shutdown so a different goroutine
// cannot inherit the same native thread while the owner remains live.
func NewOwner(factory SessionFactory, options Options) *Owner {
	runtime.LockOSThread()
	threadID := currentOwnerThreadID()
	if threadID == 0 {
		runtime.UnlockOSThread()
		panic("mux: native owner thread identity unavailable")
	}
	mux := newMux(factory, options)
	owner := &Owner{
		mux: mux, generation: 1, threadID: threadID,
	}
	owner.threadLocked.Store(true)
	owner.state.owner = owner
	owner.state.generation.Store(1)
	mux.owner = &owner.state
	return owner
}

// Generation identifies the immutable capability generation.
func (o *Owner) Generation() uint64 {
	if o == nil {
		return 0
	}
	return o.generation
}

// Closed reports whether the process owner has completed close publication.
func (o *Owner) Closed() bool {
	return o == nil || o.state.closed.Load()
}

func (o *Owner) enter(target *Mux) (mutationScope, error) {
	return o.enterAttested(target, currentOwnerThreadID())
}

// enterAttested completes scope creation from a native-thread identity already
// sampled at the dispatch boundary. It never trusts the supplied identity
// without matching it to the immutable owner thread.
func (o *Owner) enterAttested(target *Mux, threadID uint64) (mutationScope, error) {
	if o == nil || target == nil || o.state.owner != o || o.mux != target ||
		target.owner != &o.state {
		return mutationScope{}, ErrWrongOwner
	}
	if o.state.closed.Load() {
		return mutationScope{}, ErrOwnerClosed
	}
	if threadID == 0 || threadID != o.threadID {
		return mutationScope{}, ErrWrongOwnerThread
	}
	if current := o.state.generation.Load(); current != o.generation {
		return mutationScope{}, ErrStaleOwner
	}
	if o.state.activeScope.Load() != 0 {
		return mutationScope{}, ErrOwnerBusy
	}
	nonce := o.state.nextScope.Add(1)
	if nonce == 0 {
		return mutationScope{}, ErrStaleMutationScope
	}
	if !o.state.activeScope.CompareAndSwap(0, nonce) {
		return mutationScope{}, ErrOwnerBusy
	}
	if o.state.closed.Load() {
		o.state.activeScope.CompareAndSwap(nonce, 0)
		return mutationScope{}, ErrOwnerClosed
	}
	if current := o.state.generation.Load(); current != o.generation {
		o.state.activeScope.CompareAndSwap(nonce, 0)
		return mutationScope{}, ErrStaleOwner
	}
	return mutationScope{
		stamp: ownerStamp{state: &o.state, generation: o.generation},
		nonce: nonce, threadID: threadID,
	}, nil
}

func (o *Owner) leave(scope mutationScope) {
	if o == nil || scope.stamp.state == nil || scope.stamp.state != &o.state ||
		scope.threadID != o.threadID {
		return
	}
	o.state.activeScope.CompareAndSwap(scope.nonce, 0)
}

func (s mutationScope) valid(target *Mux) error {
	if err := s.stamp.valid(target); err != nil {
		return err
	}
	if s.threadID == 0 || currentOwnerThreadID() != s.threadID {
		return ErrWrongOwnerThread
	}
	if s.nonce == 0 || s.stamp.state.activeScope.Load() != s.nonce {
		return ErrStaleMutationScope
	}
	return s.validOrigin(target)
}

func (s mutationScope) validOrigin(target *Mux) error {
	if s.origin == (WindowIdentity{}) {
		return nil
	}
	if s.origin.ID == 0 || s.origin.Incarnation == 0 || target == nil {
		return ErrWrongOrigin
	}
	window := target.model.windowByID(s.origin.ID)
	if window == nil || window.incarnation != s.origin.Incarnation {
		return ErrWrongOrigin
	}
	return nil
}

func (s mutationScope) validActiveOrigin(target *Mux) error {
	if err := s.valid(target); err != nil {
		return err
	}
	if s.origin != (WindowIdentity{}) && target.model.activeWindow != s.origin.ID {
		return ErrWrongOrigin
	}
	return nil
}

func (s mutationScope) validPaneOrigin(target *Mux, pane PaneID) error {
	if err := s.valid(target); err != nil {
		return err
	}
	if s.origin == (WindowIdentity{}) {
		return nil
	}
	window, ok := target.WindowForPane(pane)
	if !ok || window != s.origin.ID {
		return ErrWrongOrigin
	}
	return nil
}

func (s mutationScope) validTabOrigin(target *Mux, tab TabID) error {
	if err := s.valid(target); err != nil {
		return err
	}
	if s.origin == (WindowIdentity{}) {
		return nil
	}
	window, ok := target.WindowForTab(tab)
	if !ok || window != s.origin.ID {
		return ErrWrongOrigin
	}
	return nil
}

// validDispatch preserves the nested-helper name while performing a fresh exact

// native-thread, owner, nonce, activity, and origin attestation at every sink.
func (s mutationScope) validDispatch(target *Mux) error { return s.valid(target) }

func (s ownerStamp) live() error {
	if s.state == nil && s.generation == 0 {
		return nil
	}
	if s.state == nil || s.state.owner == nil || &s.state.owner.state != s.state {
		return ErrWrongOwner
	}
	if current := s.state.generation.Load(); current != s.generation {
		return ErrStaleOwner
	}
	if s.state.closed.Load() {
		return ErrOwnerClosed
	}
	return nil
}

func (s ownerStamp) valid(target *Mux) error {
	if s.state == nil || s.state.owner == nil || target == nil || target.owner != s.state ||
		&s.state.owner.state != s.state {
		return ErrWrongOwner
	}
	if current := s.state.generation.Load(); current != s.generation {
		return ErrStaleOwner
	}
	if s.state.closed.Load() {
		return ErrOwnerClosed
	}
	return nil
}

// validPrepared permits only the zero stamp used by package tests' unowned Mux.
// Production construction always installs a non-zero owner stamp.
func (s ownerStamp) validPrepared(target *Mux) error {
	if target != nil && target.owner == nil && s.state == nil && s.generation == 0 {
		return nil
	}
	return s.valid(target)
}

func (m *Mux) currentOwnerStamp() ownerStamp {
	if m == nil || m.owner == nil {
		return ownerStamp{}
	}
	return ownerStamp{state: m.owner, generation: m.owner.generation.Load()}
}

func (o *Owner) publishClosed(err error) {
	o.state.closeErr = err
	o.state.generation.Add(1)
	o.state.closed.Store(true)
}
