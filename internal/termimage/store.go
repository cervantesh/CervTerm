package termimage

import (
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

	epoch          atomic.Uint64
	nextGeneration ResourceGeneration
	closed         atomic.Bool

	pendingMu sync.Mutex
	pending   map[TransferID]*CandidateTransfer
	resources map[ImageID]*resource
}

type resource struct {
	ref           ResourceRef
	width, height uint32
	stride        uint32
	rgba          []byte
	lease         *reservation
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
		pending:   make(map[TransferID]*CandidateTransfer),
		resources: make(map[ImageID]*resource),
	}
	store.epoch.Store(1)
	return store
}

func (s *Store) BeginTransfer(header Header) (*CandidateTransfer, error) {
	if s == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	if header.Transfer == 0 || header.Image == 0 {
		return nil, ErrInvalidID
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
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
	transfer.timer = s.after(HardTransferLifetime, transfer.Close)
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

func (s *Store) Acquire(ref ResourceRef) (DetachedResource, bool) {
	if s == nil || s.closed.Load() || ref.Image == 0 || ref.Generation == 0 {
		return DetachedResource{}, false
	}
	stored := s.resources[ref.Image]
	if stored == nil || stored.ref != ref {
		return DetachedResource{}, false
	}
	pixels := append([]byte(nil), stored.rgba...)
	return DetachedResource{
		Ref: ref, Width: stored.width, Height: stored.height,
		Stride: stored.stride, RGBA: pixels,
	}, true
}

// NewDecodedCandidate reserves pane/process residency before allocating pixels.
// RGBA is worker-owned until candidate close or a later owner-thread transaction.
func (s *Store) NewDecodedCandidate(image ImageID, width, height uint32) (*DecodedCandidate, error) {
	if s == nil || s.closed.Load() {
		return nil, ErrClosed
	}
	epoch := s.epoch.Load()
	if image == 0 {
		return nil, ErrInvalidID
	}
	stride, size, err := CheckedRGBABytes(width, height)
	if err != nil {
		return nil, err
	}
	lease, err := reserve(s.process, &s.pane, Usage{DecodedBytes: size, Images: 1})
	if err != nil {
		return nil, err
	}
	pixels := make([]byte, int(size))
	if s.closed.Load() || s.epoch.Load() != epoch {
		lease.Close()
		return nil, ErrClosed
	}
	return &DecodedCandidate{
		store: s, epoch: StoreEpoch(epoch), image: image,
		width: width, height: height, stride: stride,
		rgba: pixels, lease: lease,
	}, nil
}

func (s *Store) Reset() {
	if s == nil || s.closed.Load() {
		return
	}
	s.resetState()
}

func (s *Store) Close() {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.resetState()
}

func (s *Store) resetState() {
	for _, transfer := range s.takePending() {
		transfer.Close()
	}
	for _, stored := range s.resources {
		stored.lease.Close()
	}
	s.resources = make(map[ImageID]*resource)
	currentEpoch := s.epoch.Load()
	if currentEpoch == math.MaxUint64 {
		s.closed.Store(true)
		return
	}
	s.epoch.Store(currentEpoch + 1)
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
	if s.nextGeneration == ResourceGeneration(math.MaxUint64) {
		return ResourceRef{}, ErrGenerationExhausted
	}
	return ResourceRef{Image: image, Generation: s.nextGeneration + 1}, nil
}

func (s *Store) consumePreparedRef(ref ResourceRef) bool {
	if ref.Image == 0 || ref.Generation == 0 || ref.Generation != s.nextGeneration+1 {
		return false
	}
	s.nextGeneration = ref.Generation
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
	closed  atomic.Bool
	chunks  [][]byte
	leases  []*reservation
	encoded uint64
}

func (t *CandidateTransfer) Append(chunk []byte) error {
	if t == nil || len(chunk) == 0 || uint64(len(chunk)) > HardControlChunkBytes {
		return ErrInvalidChunk
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing.Load() {
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
	if !t.store.now().Before(t.deadline) {
		t.closeLocked()
		return nil, ErrTransferExpired
	}
	result := make([]byte, 0, int(t.encoded))
	for _, chunk := range t.chunks {
		result = append(result, chunk...)
	}
	return result, nil
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
	t.closed.Store(true)
	t.store.removeTransfer(t)
}

type DecodedCandidate struct {
	store                 *Store
	epoch                 StoreEpoch
	image                 ImageID
	width, height, stride uint32
	rgba                  []byte
	lease                 *reservation
	closed                atomic.Bool
}

func (c *DecodedCandidate) Image() ImageID {
	if c == nil {
		return 0
	}
	return c.image
}
func (c *DecodedCandidate) Epoch() StoreEpoch {
	if c == nil {
		return 0
	}
	return c.epoch
}
func (c *DecodedCandidate) Dimensions() (uint32, uint32, uint32) {
	if c == nil {
		return 0, 0, 0
	}
	return c.width, c.height, c.stride
}

// RGBA returns worker-owned mutable storage. It must not outlive the candidate.
func (c *DecodedCandidate) RGBA() []byte {
	if c == nil || c.closed.Load() {
		return nil
	}
	return c.rgba
}
func (c *DecodedCandidate) ValidFor(store *Store) bool {
	return c != nil && !c.closed.Load() && store == c.store && !store.closed.Load() && c.epoch == StoreEpoch(store.epoch.Load())
}
func (c *DecodedCandidate) Close() {
	if c == nil || !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.rgba = nil
	c.lease.Close()
}
