package termimage

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	process *ProcessBudget
	pane    paneBudget
	now     func() time.Time

	epoch           atomic.Uint64
	closed          atomic.Bool
	owner           atomic.Pointer[StoreOwner]
	ownerGeneration atomic.Uint64
	resetting       atomic.Bool
	ownerFault      func(string) error // package-private deterministic fault injection

	pendingMu             sync.Mutex
	pending               map[TransferID]*CandidateTransfer
	state                 *storeState
	prepared              *PreparedStoreState
	preparedClose         atomic.Pointer[PreparedStoreClose]
	placementMu           sync.Mutex
	candidateMu           sync.Mutex
	candidates            map[*DecodedCandidate]struct{}
	placements            map[*PlacementReservation]struct{}
	identityMu            sync.Mutex
	nextInternalImage     ImageID
	nextInternalPlacement PlacementID
}

type resource struct {
	ref           ResourceRef
	width, height uint32
	stride        uint32
	rgba          []byte
	lease         *reservation
	retention     ResourceRetention
}

type storeState struct {
	resources      map[ImageID]*resource
	nextGeneration ResourceGeneration
}

func NewStore(process *ProcessBudget, limits Limits) *Store {
	effective, err := ValidateLimits(limits)
	if err != nil || process == nil {
		return nil
	}
	store := &Store{
		process:               process,
		pane:                  paneBudget{limits: effective},
		now:                   time.Now,
		pending:               make(map[TransferID]*CandidateTransfer),
		candidates:            make(map[*DecodedCandidate]struct{}),
		state:                 &storeState{resources: make(map[ImageID]*resource)},
		placements:            make(map[*PlacementReservation]struct{}),
		nextInternalImage:     MinInternalImageID - 1,
		nextInternalPlacement: MinInternalPlacementID - 1,
	}
	store.epoch.Store(1)
	store.ownerGeneration.Store(1)
	return store
}

type storeOwnerNoCopy struct{}

func (*storeOwnerNoCopy) Lock()   {}
func (*storeOwnerNoCopy) Unlock() {}

type StoreOwner struct {
	_            storeOwnerNoCopy
	store        *Store
	generation   uint64
	threadID     uint64
	threadLocked atomic.Bool
	released     atomic.Bool
	activeScope  atomic.Uint64
	nextScope    atomic.Uint64
}

// storeMutationScope is valid for one exact StoreOwner dispatch. Every sink

// revalidates owner identity, generation, nonce, activity, and native thread

// before its first write.
type storeMutationScope struct {
	owner      *StoreOwner
	store      *Store
	generation uint64
	nonce      uint64
	threadID   uint64
}

// PreparedStoreClose binds close preparation to one owner generation. Commit and
// Abort acquire a fresh one-dispatch owner scope before validation or mutation.
type PreparedStoreClose struct {
	owner      *StoreOwner
	store      *Store
	generation uint64
	active     bool
	finished   atomic.Bool
	result     error
}

func (s *Store) ClaimOwner() *StoreOwner {
	if s == nil || s.closed.Load() || s.resetting.Load() || s.prepared != nil || s.preparedClose.Load() != nil {
		return nil
	}
	runtime.LockOSThread()
	threadID := currentStoreOwnerThreadID()
	if threadID == 0 {
		runtime.UnlockOSThread()
		return nil
	}
	owner := &StoreOwner{store: s, generation: s.ownerGeneration.Load(), threadID: threadID}
	owner.threadLocked.Store(true)
	if !s.owner.CompareAndSwap(nil, owner) {
		runtime.UnlockOSThread()
		return nil
	}
	return owner
}

func (o *StoreOwner) enter() (storeMutationScope, error) {
	return o.enterWithPreparedClose(nil)
}

func (o *StoreOwner) enterPreparedClose(prepared *PreparedStoreClose) (storeMutationScope, error) {
	if prepared == nil || o == nil || o.store == nil || o.store.preparedClose.Load() != prepared {
		return storeMutationScope{}, ErrStaleMutationScope
	}
	return o.enterWithPreparedClose(prepared)
}

func (o *StoreOwner) enterWithPreparedClose(prepared *PreparedStoreClose) (storeMutationScope, error) {
	if o == nil || o.store == nil {
		return storeMutationScope{}, ErrWrongOwner
	}
	if o.released.Load() || o.store.closed.Load() {
		return storeMutationScope{}, ErrClosed
	}
	if o.store.owner.Load() != o {
		return storeMutationScope{}, ErrWrongOwner
	}
	if o.store.ownerGeneration.Load() != o.generation {
		return storeMutationScope{}, ErrStaleOwner
	}
	threadID := currentStoreOwnerThreadID()
	if threadID == 0 || threadID != o.threadID {
		return storeMutationScope{}, ErrWrongOwnerThread
	}
	if current := o.store.preparedClose.Load(); current != nil && current != prepared {
		return storeMutationScope{}, ErrOwnerBusy
	}
	if prepared != nil && o.store.preparedClose.Load() != prepared {
		return storeMutationScope{}, ErrStaleMutationScope
	}
	if o.activeScope.Load() != 0 {
		return storeMutationScope{}, ErrOwnerBusy
	}
	nonce := o.nextScope.Add(1)
	if nonce == 0 {
		return storeMutationScope{}, ErrStaleMutationScope
	}
	if !o.activeScope.CompareAndSwap(0, nonce) {
		return storeMutationScope{}, ErrOwnerBusy
	}
	scope := storeMutationScope{owner: o, store: o.store, generation: o.generation, nonce: nonce, threadID: threadID}
	if err := scope.valid(o.store); err != nil {
		o.activeScope.CompareAndSwap(nonce, 0)
		return storeMutationScope{}, err
	}
	return scope, nil
}

func (o *StoreOwner) leave(scope storeMutationScope) {
	if o == nil || scope.owner != o || scope.store != o.store || scope.threadID != o.threadID || scope.nonce == 0 {
		return
	}
	o.activeScope.CompareAndSwap(scope.nonce, 0)
}

func (s storeMutationScope) valid(store *Store) error {
	if s.owner == nil || s.store == nil || store == nil || s.store != store || s.owner.store != store || store.owner.Load() != s.owner {
		return ErrWrongOwner
	}
	if s.generation == 0 || s.owner.generation != s.generation || store.ownerGeneration.Load() != s.generation {
		return ErrStaleOwner
	}
	if s.threadID == 0 || s.threadID != s.owner.threadID || currentStoreOwnerThreadID() != s.threadID {
		return ErrWrongOwnerThread
	}
	if s.nonce == 0 || s.owner.activeScope.Load() != s.nonce {
		return ErrStaleMutationScope
	}
	return nil
}

// PrepareClose verifies that close can commit without retaining a dispatch scope.
// Commit or Abort must reacquire the exact owner and generation.
func (o *StoreOwner) PrepareClose(target *Store) (*PreparedStoreClose, error) {
	if o == nil || o.store == nil || target == nil || target != o.store {
		return nil, ErrWrongOwner
	}
	scope, err := o.enter()
	if err != nil {
		return nil, err
	}
	defer o.leave(scope)
	if o.store.ownerFault != nil {
		if err := o.store.ownerFault("close"); err != nil {
			return nil, err
		}
	}
	prepared := &PreparedStoreClose{owner: o, store: o.store, generation: o.generation, active: true}
	if !o.store.preparedClose.CompareAndSwap(nil, prepared) {
		return nil, ErrOwnerBusy
	}
	return prepared, nil
}

func (o *StoreOwner) PrepareCandidate(candidate *DecodedCandidate) (*PreparedStoreState, ResourceRef, error) {
	scope, err := o.enter()
	if err != nil {
		return nil, ResourceRef{}, err
	}
	defer o.leave(scope)
	return o.store.prepareCandidate(scope, candidate)
}

func (o *StoreOwner) PrepareResourceRemoval(refs []ResourceRef) (*PreparedStoreState, error) {
	scope, err := o.enter()
	if err != nil {
		return nil, err
	}
	defer o.leave(scope)
	return o.store.prepareResourceRemoval(scope, refs)
}

func (o *StoreOwner) PrepareReset() (*PreparedStoreState, error) {
	scope, err := o.enter()
	if err != nil {
		return nil, err
	}
	defer o.leave(scope)
	return o.store.prepareReset(scope)
}

func (o *StoreOwner) PublishPrepared(prepared *PreparedStoreState) error {
	scope, err := o.enter()
	if err != nil {
		return err
	}
	defer o.leave(scope)
	if o.store.ownerFault != nil {
		if err := o.store.ownerFault("publish"); err != nil {
			return err
		}
	}
	return o.store.publishPrepared(scope, prepared)
}

func (p *PreparedStoreClose) Commit() error {
	if p == nil || p.owner == nil || p.store == nil {
		return ErrWrongOwner
	}
	if p.finished.Load() {
		return p.result
	}
	o := p.owner
	scope, err := o.enterPreparedClose(p)
	if err != nil {
		return err
	}
	if p.generation != o.generation || !p.active {
		o.leave(scope)
		return ErrStaleMutationScope
	}
	p.result = p.store.closeOwned(scope)
	if p.result != nil {
		o.leave(scope)
		return p.result
	}
	if !p.store.preparedClose.CompareAndSwap(p, nil) {
		o.leave(scope)
		return ErrStaleMutationScope
	}
	o.released.Store(true)
	p.active = false
	p.finished.Store(true)
	o.leave(scope)
	if o.threadLocked.CompareAndSwap(true, false) {
		runtime.UnlockOSThread()
	}
	return nil
}

func (p *PreparedStoreClose) Abort() error {
	if p == nil || p.owner == nil || p.store == nil {
		return ErrWrongOwner
	}
	if p.finished.Load() {
		return p.result
	}
	scope, err := p.owner.enterPreparedClose(p)
	if err != nil {
		return err
	}
	defer p.owner.leave(scope)
	if p.generation != p.owner.generation || !p.active {
		return ErrStaleMutationScope
	}
	if !p.store.preparedClose.CompareAndSwap(p, nil) {
		return ErrStaleMutationScope
	}
	p.active = false
	p.finished.Store(true)
	return nil
}

// Close abandons an uncommitted close preflight under a fresh owner scope.
func (p *PreparedStoreClose) Close() error { return p.Abort() }

func (o *StoreOwner) Close() error {
	if o == nil || o.store == nil {
		return ErrWrongOwner
	}
	if o.released.Load() {
		return nil
	}
	prepared, err := o.PrepareClose(o.store)
	if err != nil {
		return err
	}
	return prepared.Commit()
}

func (s *Store) Closed() bool { return s == nil || s.closed.Load() }

func (s *Store) BeginTransfer(header Header) (*CandidateTransfer, error) {
	if s == nil {
		return nil, ErrClosed
	}
	if header.Transfer == 0 || header.Image == 0 {
		return nil, ErrInvalidID
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.closed.Load() || s.resetting.Load() {
		return nil, ErrClosed
	}
	if s.pending[header.Transfer] != nil {
		return nil, ErrDuplicateTransfer
	}
	lease, err := reserve(s.process, &s.pane, Usage{PendingTransfers: 1})
	if err != nil {
		return nil, err
	}
	transfer := &CandidateTransfer{
		store: s, header: header, epoch: StoreEpoch(s.epoch.Load()),
		deadline: s.now().Add(HardTransferLifetime), base: lease, open: true,
	}
	s.pending[header.Transfer] = transfer
	return transfer, nil
}

func (s *Store) removeTransfer(transfer *CandidateTransfer) {
	s.pendingMu.Lock()
	if s.pending[transfer.header.Transfer] == transfer {
		delete(s.pending, transfer.header.Transfer)
	}
	s.pendingMu.Unlock()
}

func (s *Store) takePending() []*CandidateTransfer {
	s.pendingMu.Lock()
	result := make([]*CandidateTransfer, 0, len(s.pending))
	for _, transfer := range s.pending {
		result = append(result, transfer)
	}
	s.pending = make(map[TransferID]*CandidateTransfer)
	s.pendingMu.Unlock()
	return result
}

func (s *Store) takeCandidates() []*DecodedCandidate {
	s.candidateMu.Lock()
	result := make([]*DecodedCandidate, 0, len(s.candidates))
	for candidate := range s.candidates {
		result = append(result, candidate)
	}
	s.candidates = make(map[*DecodedCandidate]struct{})
	s.candidateMu.Unlock()
	return result
}

func (s *Store) unregisterCandidate(candidate *DecodedCandidate) {
	s.candidateMu.Lock()
	delete(s.candidates, candidate)
	s.candidateMu.Unlock()
}

func (s *Store) Acquire(ref ResourceRef) (DetachedResource, bool) {
	if s == nil || ref.Image == 0 || ref.Generation == 0 {
		return DetachedResource{}, false
	}
	resources := s.state.resources
	if len(resources) == 0 {
		return DetachedResource{}, false
	}
	stored := resources[ref.Image]
	if stored == nil || stored.ref != ref {
		return DetachedResource{}, false
	}
	pixels := append([]byte(nil), stored.rgba...)
	return DetachedResource{
		Ref: ref, Width: stored.width, Height: stored.height,
		Stride: stored.stride, RGBA: pixels,
	}, true
}

func (s *Store) ResourceDimensions(ref ResourceRef) (uint32, uint32, bool) {
	if s == nil || ref.Image == 0 || ref.Generation == 0 {
		return 0, 0, false
	}
	stored := s.state.resources[ref.Image]
	if stored == nil || stored.ref != ref {
		return 0, 0, false
	}
	return stored.width, stored.height, true
}

// Reset is retained only for detached standalone worker stores. Claimed stores

// must transition through StoreOwner.PrepareReset/PublishPrepared/Commit.
func (s *Store) Reset() {
	if s == nil || s.closed.Load() || s.owner.Load() != nil || s.prepared != nil {
		return
	}
	_ = s.resetState(storeMutationScope{})
}

// Close is retained only for detached standalone worker stores. Claimed stores

// close through StoreOwner so prepared ownership cannot be bypassed.
func (s *Store) Close() {
	if s == nil || s.owner.Load() != nil || s.prepared != nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	_ = s.resetState(storeMutationScope{})
}

func (s *Store) closeOwned(scope storeMutationScope) error {
	if err := scope.valid(s); err != nil {
		return err
	}
	owner := scope.owner
	if !s.closed.CompareAndSwap(false, true) {
		if s.closed.Load() {
			return nil
		}
		return ErrClosed
	}
	if err := s.resetState(scope); err != nil {
		return err
	}
	s.ownerGeneration.Add(1)
	if !s.owner.CompareAndSwap(owner, nil) {
		return ErrWrongOwner
	}
	return nil
}

func (s *Store) resetState(scope storeMutationScope) error {
	if owner := s.owner.Load(); owner != nil {
		if err := scope.valid(s); err != nil {
			return err
		}
	}
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	s.closePlacementReservations()
	if err := s.abortPrepared(scope); err != nil {
		return err
	}
	for _, candidate := range s.takeCandidates() {
		candidate.Close()
	}
	for _, transfer := range s.takePending() {
		transfer.Close()
	}
	for _, stored := range s.state.resources {
		stored.lease.Close()
	}
	s.state = &storeState{resources: make(map[ImageID]*resource), nextGeneration: s.state.nextGeneration}
	currentEpoch := s.epoch.Load()
	if currentEpoch == math.MaxUint64 {
		s.closed.Store(true)
		s.resetting.Store(false)
		return nil
	}
	s.epoch.Store(currentEpoch + 1)
	s.resetting.Store(false)
	return nil
}

func (s *Store) Epoch() StoreEpoch {
	if s == nil {
		return 0
	}
	return StoreEpoch(s.epoch.Load())
}

func (s *Store) Usage() Usage {
	if s == nil {
		return Usage{}
	}
	return s.pane.usage()
}

func (s *Store) prepareNextRef(image ImageID) (ResourceRef, error) {
	if image == 0 {
		return ResourceRef{}, ErrInvalidID
	}
	if s.state.nextGeneration == ResourceGeneration(math.MaxUint64) {
		return ResourceRef{}, ErrGenerationExhausted
	}
	return ResourceRef{Image: image, Generation: s.state.nextGeneration + 1}, nil
}

func (s *Store) consumePreparedRef(ref ResourceRef) bool {
	if ref.Image == 0 || ref.Generation == 0 || ref.Generation != s.state.nextGeneration+1 {
		return false
	}
	s.state.nextGeneration = ref.Generation
	return true
}

type CandidateTransfer struct {
	store    *Store
	header   Header
	epoch    StoreEpoch
	deadline time.Time
	base     *reservation
	open     bool

	mu      sync.Mutex
	closing atomic.Bool
	chunks  [][]byte
	leases  []*reservation
	encoded uint64
}

func (t *CandidateTransfer) Touch() error {
	if t == nil {
		return ErrTransferClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || !t.open {
		return ErrTransferClosed
	}
	now := t.store.now()
	if !now.Before(t.deadline) {
		t.closeLocked()
		return ErrTransferExpired
	}
	t.deadline = now.Add(HardTransferLifetime)
	return nil
}

func (t *CandidateTransfer) Deadline() (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || !t.open {
		return time.Time{}, false
	}
	return t.deadline, true
}

// Expire closes an open transfer when its owner-observed deadline is due.
func (t *CandidateTransfer) Expire(now time.Time) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || !t.open || now.Before(t.deadline) {
		return false
	}
	t.closeLocked()
	return true
}

func (t *CandidateTransfer) Seal() error {
	if t == nil {
		return ErrTransferClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || !t.open {
		return ErrTransferClosed
	}
	if !t.store.now().Before(t.deadline) {
		t.closeLocked()
		return ErrTransferExpired
	}
	t.open = false
	return nil
}

func (t *CandidateTransfer) Append(chunk []byte) error {
	if t == nil || len(chunk) == 0 || uint64(len(chunk)) > HardControlChunkBytes {
		return ErrInvalidChunk
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || !t.open {
		return ErrTransferClosed
	}
	if !t.store.now().Before(t.deadline) {
		t.closeLocked()
		return ErrTransferExpired
	}
	if uint64(len(t.chunks)) >= HardChunksPerTransfer {
		return ErrTooManyChunks
	}
	lease, err := reserve(t.store.process, &t.store.pane, Usage{EncodedBytes: uint64(len(chunk))})
	if err != nil {
		return err
	}
	copyOfChunk := append([]byte(nil), chunk...)
	t.chunks = append(t.chunks, copyOfChunk)
	t.leases = append(t.leases, lease)
	t.encoded += uint64(len(copyOfChunk))
	t.deadline = t.store.now().Add(HardTransferLifetime)
	return nil
}

func (t *CandidateTransfer) EncodedCopy() ([]byte, error) {
	if t == nil {
		return nil, ErrTransferClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() {
		return nil, ErrTransferClosed
	}
	if t.open && !t.store.now().Before(t.deadline) {
		t.closeLocked()
		return nil, ErrTransferExpired
	}
	result := make([]byte, 0, int(t.encoded))
	for _, chunk := range t.chunks {
		result = append(result, chunk...)
	}
	return result, nil
}

func (t *CandidateTransfer) SealedEncodedCopy(store *Store) ([]byte, Header, StoreEpoch, error) {
	if t == nil || store == nil {
		return nil, Header{}, 0, ErrTransferClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || t.open || t.store != store || store.closed.Load() || store.resetting.Load() || t.epoch != StoreEpoch(store.epoch.Load()) {
		return nil, Header{}, 0, ErrTransferClosed
	}
	result := make([]byte, 0, int(t.encoded))
	for _, chunk := range t.chunks {
		result = append(result, chunk...)
	}
	return result, t.header, t.epoch, nil
}

func (t *CandidateTransfer) Header() Header {
	if t == nil {
		return Header{}
	}
	return t.header
}
func (t *CandidateTransfer) Epoch() StoreEpoch {
	if t == nil {
		return 0
	}
	return t.epoch
}
func (t *CandidateTransfer) Closed() bool { return t == nil || t.closing.Load() }

func (t *CandidateTransfer) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.closeLocked()
	t.mu.Unlock()
}

func (t *CandidateTransfer) closeLocked() {
	if !t.closing.CompareAndSwap(false, true) {
		return
	}
	for i := len(t.leases) - 1; i >= 0; i-- {
		t.leases[i].Close()
	}
	t.base.Close()
	t.chunks, t.leases, t.encoded = nil, nil, 0
	t.store.removeTransfer(t)
}
