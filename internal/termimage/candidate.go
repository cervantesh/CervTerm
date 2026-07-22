package termimage

import "sync"

// NewDecodedCandidate reserves pane/process residency before allocating pixels.
func (s *Store) NewDecodedCandidate(image ImageID, width, height uint32) (*DecodedCandidate, error) {
	if s == nil || s.closed.Load() || s.resetting.Load() {
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
	candidate := &DecodedCandidate{
		store: s, epoch: StoreEpoch(epoch), image: image,
		width: width, height: height, stride: stride, rgba: pixels, lease: lease,
	}
	s.candidateMu.Lock()
	if s.closed.Load() || s.resetting.Load() || s.epoch.Load() != epoch {
		s.candidateMu.Unlock()
		lease.Close()
		return nil, ErrClosed
	}
	s.candidates[candidate] = struct{}{}
	s.candidateMu.Unlock()
	return candidate, nil
}

type DecodedCandidate struct {
	store                 *Store
	epoch                 StoreEpoch
	image                 ImageID
	width, height, stride uint32
	mu                    sync.Mutex
	rgba                  []byte
	lease                 *reservation
	closed                bool
	claimed               bool
	sealed                bool
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

// WriteRGBAAt copies decoded bytes into candidate-owned storage.
func (c *DecodedCandidate) WriteRGBAAt(offset int, data []byte) error {
	if c == nil {
		return ErrCandidateInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.claimed || c.sealed || offset < 0 || offset > len(c.rgba) || len(data) > len(c.rgba)-offset {
		return ErrCandidateInvalid
	}
	copy(c.rgba[offset:], data)
	return nil
}

func (c *DecodedCandidate) SealWrites() error {
	if c == nil {
		return ErrCandidateInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.claimed || c.sealed {
		return ErrCandidateInvalid
	}
	c.sealed = true
	return nil
}

func (c *DecodedCandidate) WritesSealed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sealed && !c.closed && !c.claimed
}

type DecodeScratchLease struct {
	lease *reservation
	once  sync.Once
}

func (s *Store) ReserveDecodeScratch(bytes uint64) (*DecodeScratchLease, error) {
	if s == nil || bytes == 0 || s.closed.Load() || s.resetting.Load() {
		return nil, ErrClosed
	}
	epoch := s.epoch.Load()
	lease, err := reserve(s.process, &s.pane, Usage{DecodedBytes: bytes})
	if err != nil {
		return nil, err
	}
	if s.closed.Load() || s.resetting.Load() || s.epoch.Load() != epoch {
		lease.Close()
		return nil, ErrClosed
	}
	return &DecodeScratchLease{lease: lease}, nil
}

func (l *DecodeScratchLease) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.lease != nil {
			l.lease.Close()
			l.lease = nil
		}
	})
}

// RGBA returns a detached diagnostic copy, never mutable candidate storage.
func (c *DecodedCandidate) RGBA() []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.claimed {
		return nil
	}
	return append([]byte(nil), c.rgba...)
}

func (c *DecodedCandidate) ValidFor(store *Store) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && !c.claimed && store == c.store && !store.closed.Load() && !store.resetting.Load() && c.epoch == StoreEpoch(store.epoch.Load())
}

func (c *DecodedCandidate) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed || c.claimed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	lease := c.lease
	c.rgba, c.lease = nil, nil
	c.mu.Unlock()
	c.store.unregisterCandidate(c)
	lease.Close()
}

func (c *DecodedCandidate) claimOwnership() ([]byte, *reservation, bool) {
	c.mu.Lock()
	if c.closed || c.claimed {
		c.mu.Unlock()
		return nil, nil, false
	}
	c.claimed, c.closed = true, true
	pixels, lease := c.rgba, c.lease
	c.rgba, c.lease = nil, nil
	c.mu.Unlock()
	c.store.unregisterCandidate(c)
	return pixels, lease, true
}
