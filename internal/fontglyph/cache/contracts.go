// Package cache owns the bounded, pin-aware parsed-face cache.
package cache

import (
	"errors"
	"sync"

	"cervterm/internal/fontglyph/internal/face"
)

var (
	// ErrCapacity reports that face or source-byte admission cannot proceed
	// without evicting a pinned or loading entry.
	ErrCapacity = errors.New("font parse cache capacity exceeded")
	// ErrFileGrew reports that a source exceeded its stat-sized reservation.
	ErrFileGrew = errors.New("font file grew while loading")
)

// Parser constructs one size-independent face owner for source bytes and a
// collection index. Expensive parsing runs outside the cache mutex.
type Parser[T any] func([]byte, int) (*face.Owner[T], error)

// Stats is a detached cache accounting snapshot.
type Stats struct {
	Entries int
	Loading int
	Ready   int
	Pinned  int
	Bytes   int64
}

type state uint8

const (
	missing state = iota
	loading
	ready
)

type sourceBlob struct {
	state    state
	wait     chan struct{}
	data     []byte
	err      error
	reserved int64
	refs     int
}

type entry[T any] struct {
	state    state
	wait     chan struct{}
	waiters  int
	owner    *face.Owner[T]
	err      error
	source   *sourceBlob
	pins     int
	lastUsed uint64
}

// Manager bounds parsed faces and retained source bytes. It has no global
// singleton; the fontglyph facade owns the production instance.
type Manager[T any] struct {
	mu        sync.Mutex
	entries   map[string]*entry[T]
	sources   map[string]*sourceBlob
	maxFaces  int
	maxBytes  int64
	bytes     int64
	clock     uint64
	parse     Parser[T]
	afterWait func() // test-only scheduling seam; nil in production
}

// Lease owns exactly one cache pin. Close is safe to call repeatedly.
type Lease[T any] struct {
	once    sync.Once
	manager *Manager[T]
	entry   *entry[T]
}

// Close releases the lease's one pin exactly once.
func (l *Lease[T]) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		manager := l.manager
		manager.mu.Lock()
		if l.entry.pins > 0 {
			l.entry.pins--
		}
		manager.mu.Unlock()
	})
}
