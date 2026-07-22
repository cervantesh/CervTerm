package termimage

import (
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type timerStopper interface{ Stop() bool }

type Store struct {
	process *ProcessBudget
	pane    paneBudget
	now     func() time.Time
	after   func(time.Duration, func()) timerStopper

	epoch     atomic.Uint64
	closed    atomic.Bool
	owner     atomic.Pointer[StoreOwner]
	resetting atomic.Bool

	pendingMu   sync.Mutex
	pending     map[TransferID]*CandidateTransfer
	state       *storeState
	prepared    *PreparedStoreState
	placementMu sync.Mutex
	candidateMu sync.Mutex
	candidates  map[*DecodedCandidate]struct{}
	placements  map[*PlacementReservation]struct{}
}

type resource struct {
	ref           ResourceRef
	width, height uint32
	stride        uint32
	rgba          []byte
	lease         *reservation
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
		process: process,
		pane:    paneBudget{limits: effective},
		now:     time.Now,
		after: func(delay time.Duration, callback func()) timerStopper {
			return time.AfterFunc(delay, callback)
		},
		pending:    make(map[TransferID]*CandidateTransfer),
		candidates: make(map[*DecodedCandidate]struct{}),
		state:      &storeState{resources: make(map[ImageID]*resource)},
		placements: make(map[*PlacementReservation]struct{}),
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
		deadline: s.now().Add(HardTransferLifetime), base: lease,
	}
	s.pending[header.Transfer] = transfer
	transfer.timer = s.after(HardTransferLifetime, transfer.expire)
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
	timer    timerStopper

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
	if t.closing.Load() || t.timer == nil {
		return ErrTransferClosed
	}
	now := t.store.now()
	if !now.Before(t.deadline) {
		t.closeLocked()
		return ErrTransferExpired
	}
	t.deadline = now.Add(HardTransferLifetime)
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = t.store.after(HardTransferLifetime, t.expire)
	return nil
}

func (t *CandidateTransfer) Deadline() (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || t.timer == nil {
		return time.Time{}, false
	}
	return t.deadline, true
}

func (t *CandidateTransfer) Seal() error {
	if t == nil {
		return ErrTransferClosed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || t.timer == nil {
		return ErrTransferClosed
	}
	if !t.store.now().Before(t.deadline) {
		t.closeLocked()
		return ErrTransferExpired
	}
	t.timer.Stop()
	t.timer = nil
	return nil
}

func (t *CandidateTransfer) expire() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || t.timer == nil {
		return
	}
	delay := t.deadline.Sub(t.store.now())
	if delay > 0 {
		t.timer = t.store.after(delay, t.expire)
		return
	}
	t.closeLocked()
}

func (t *CandidateTransfer) Append(chunk []byte) error {
	if t == nil || len(chunk) == 0 || uint64(len(chunk)) > HardControlChunkBytes {
		return ErrInvalidChunk
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() || t.timer == nil {
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
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = t.store.after(HardTransferLifetime, t.expire)
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
	if t.timer != nil && !t.store.now().Before(t.deadline) {
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
	if t.closing.Load() || t.timer != nil || t.store != store || store.closed.Load() || store.resetting.Load() || t.epoch != StoreEpoch(store.epoch.Load()) {
		return nil, Header{}, 0, ErrTransferClosed
	}
	result := make([]byte, 0, int(t.encoded))
	for _, chunk := range t.chunks {
		result = append(result, chunk...)
	}
	return result, t.header, t.epoch, nil
}

type SealedEncodedPayload struct {
	header Header
	epoch  StoreEpoch
	chunks [][]byte
	leases []*reservation
	base   *reservation
	once   sync.Once
}

func (t *CandidateTransfer) TakeSealedPayload(store *Store) (*SealedEncodedPayload, error) {
	if t == nil || store == nil {
		return nil, ErrTransferClosed
	}
	t.mu.Lock()
	if t.closing.Load() || t.timer != nil || t.store != store || store.closed.Load() || store.resetting.Load() || t.epoch != StoreEpoch(store.epoch.Load()) || !t.closing.CompareAndSwap(false, true) {
		t.mu.Unlock()
		return nil, ErrTransferClosed
	}
	payload := &SealedEncodedPayload{header: t.header, epoch: t.epoch, chunks: t.chunks, leases: t.leases, base: t.base}
	t.chunks, t.leases, t.base, t.encoded = nil, nil, nil, 0
	t.mu.Unlock()
	store.removeTransfer(t)
	return payload, nil
}

func (p *SealedEncodedPayload) Header() Header {
	if p == nil {
		return Header{}
	}
	return p.header
}
func (p *SealedEncodedPayload) Epoch() StoreEpoch {
	if p == nil {
		return 0
	}
	return p.epoch
}
func (p *SealedEncodedPayload) EncodedLen() uint64 {
	var total uint64
	if p != nil {
		for _, chunk := range p.chunks {
			total += uint64(len(chunk))
		}
	}
	return total
}
func (p *SealedEncodedPayload) Reader() io.Reader {
	if p == nil {
		return &encodedChunkReader{}
	}
	return &encodedChunkReader{chunks: p.chunks}
}
func (p *SealedEncodedPayload) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		for i := len(p.leases) - 1; i >= 0; i-- {
			p.leases[i].Close()
		}
		if p.base != nil {
			p.base.Close()
		}
		p.chunks, p.leases, p.base = nil, nil, nil
	})
}

type encodedChunkReader struct {
	chunks        [][]byte
	chunk, offset int
}

func (r *encodedChunkReader) Read(dst []byte) (int, error) {
	for r.chunk < len(r.chunks) {
		if r.offset == len(r.chunks[r.chunk]) {
			r.chunk++
			r.offset = 0
			continue
		}
		n := copy(dst, r.chunks[r.chunk][r.offset:])
		r.offset += n
		return n, nil
	}
	return 0, io.EOF
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
	if t.timer != nil {
		t.timer.Stop()
	}
	for i := len(t.leases) - 1; i >= 0; i-- {
		t.leases[i].Close()
	}
	t.base.Close()
	t.chunks, t.leases, t.encoded = nil, nil, 0
	t.store.removeTransfer(t)
}
