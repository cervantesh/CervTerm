package mux

import "sync/atomic"

// noCopy makes accidental Owner value copies visible to go vet. Owner methods
// additionally verify pointer identity, so a copied value is not a capability.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type ownerKey struct{}

type ownerState struct {
	key        *ownerKey
	owner      *Owner
	generation atomic.Uint64
	active     atomic.Bool
	closed     atomic.Bool
	closeErr   error
}

// Owner is the unforgeable process-local mux mutation capability. It serializes
// mutation by explicit capability possession and enter/leave, not by claiming a
// goroutine or OS-thread identity. It never exposes the concrete Mux.
type Owner struct {
	_          noCopy
	mux        *Mux
	state      *ownerState
	key        *ownerKey
	generation uint64
}

// ownerStamp binds prepared work to one exact owner generation.
type ownerStamp struct {
	state      *ownerState
	key        *ownerKey
	generation uint64
}

// NewOwner creates the process-owned mux capability. The legacy New constructor
// remains authoritative until Slice 3.1 wiring migrates production callers.
func NewOwner(factory SessionFactory, options Options) *Owner {
	m := New(factory, options)
	key := &ownerKey{}
	state := &ownerState{key: key}
	state.generation.Store(1)
	owner := &Owner{mux: m, state: state, key: key, generation: 1}
	state.owner = owner
	m.owner = state
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
	return o == nil || o.state == nil || o.state.closed.Load()
}

func (o *Owner) enter(target *Mux) (ownerStamp, error) {
	if o == nil || o.state == nil || o.key == nil || target == nil ||
		o.state.owner != o || o.mux != target || target.owner != o.state || o.state.key != o.key {
		return ownerStamp{}, ErrWrongOwner
	}
	if o.state.closed.Load() {
		return ownerStamp{}, ErrOwnerClosed
	}
	if current := o.state.generation.Load(); current != o.generation {
		return ownerStamp{}, ErrStaleOwner
	}
	if !o.state.active.CompareAndSwap(false, true) {
		return ownerStamp{}, ErrOwnerBusy
	}
	if o.state.closed.Load() {
		o.state.active.Store(false)
		return ownerStamp{}, ErrOwnerClosed
	}
	if current := o.state.generation.Load(); current != o.generation {
		o.state.active.Store(false)
		return ownerStamp{}, ErrStaleOwner
	}
	return ownerStamp{state: o.state, key: o.key, generation: o.generation}, nil
}

func (o *Owner) leave(stamp ownerStamp) {
	if o == nil || stamp.state == nil || stamp.state != o.state || stamp.key != o.key || stamp.generation != o.generation {
		return
	}
	stamp.state.active.Store(false)
}

func (s ownerStamp) valid(target *Mux) error {
	if s.state == nil || s.key == nil || target == nil || target.owner != s.state || s.state.key != s.key {
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

func (o *Owner) publishClosed(err error) {
	o.state.closeErr = err
	o.state.generation.Add(1)
	o.state.closed.Store(true)
}
