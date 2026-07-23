package termimage

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type Store struct {
	process *ProcessBudget
	pane    paneBudget
	now     func() time.Time

	epoch     atomic.Uint64
	closed    atomic.Bool
	owner     atomic.Pointer[StoreOwner]
	resetting atomic.Bool

	pendingMu             sync.Mutex
	pending               map[TransferID]*CandidateTransfer
	state                 *storeState
	prepared              *PreparedStoreState
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
	return store
}

type StoreOwner struct {
	store    *Store
	released atomic.Bool
}

func (s *Store) ClaimOwner() *StoreOwner {
	if s == nil || s.closed.Load() || s.resetting.Load() || s.prepared != nil {
		return nil
	}
	owner := &StoreOwner{store: s}
	if !s.owner.CompareAndSwap(nil, owner) {
		return nil
	}
	return owner
}

func (o *StoreOwner) valid() bool {
	return o != nil && !o.released.Load() && o.store != nil && o.store.owner.Load() == o && !o.store.closed.Load()
}

func (o *StoreOwner) PrepareCandidate(candidate *DecodedCandidate) (*PreparedStoreState, ResourceRef, error) {
	if !o.valid() {
		return nil, ResourceRef{}, ErrClosed
	}
	return o.store.prepareCandidate(candidate)
}

func (o *StoreOwner) PrepareResourceRemoval(refs []ResourceRef) (*PreparedStoreState, error) {
	if !o.valid() {
		return nil, ErrClosed
	}
	return o.store.prepareResourceRemoval(refs)
}

func (o *StoreOwner) PrepareReset() (*PreparedStoreState, error) {
	if !o.valid() {
		return nil, ErrClosed
	}
	return o.store.prepareReset()
}

func (o *StoreOwner) PublishPrepared(prepared *PreparedStoreState) {
	if !o.valid() {
		panic(ErrPreparedState)
	}
	o.store.publishPrepared(prepared)
}

func (o *StoreOwner) Close() {
	if o == nil || !o.released.CompareAndSwap(false, true) {
		return
	}
	o.store.closeOwned(o)
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

func (s *Store) Reset() {
	if s == nil || s.closed.Load() || s.owner.Load() != nil {
		return
	}
	s.resetState()
}

func (s *Store) Close() {
	if s == nil || s.owner.Load() != nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.resetState()
}

func (s *Store) closeOwned(owner *StoreOwner) {
	if s == nil || s.owner.Load() != owner || !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.resetState()
	s.owner.CompareAndSwap(owner, nil)
}

func (s *Store) resetState() {
	s.identityMu.Lock()
	defer s.identityMu.Unlock()
	s.closePlacementReservations()
	s.abortPrepared()
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
		return
	}
	s.epoch.Store(currentEpoch + 1)
	s.resetting.Store(false)
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
