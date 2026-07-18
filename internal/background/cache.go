package background

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

const MaxCacheEntries = 8

var (
	ErrCacheClosed    = errors.New("background cache is closed")
	ErrCacheDuplicate = errors.New("background cache key already exists")
	ErrCachePinned    = errors.New("background cache has pinned entries")
	ErrLeaseReleased  = errors.New("background cache lease is already released")
)

// CacheVariant distinguishes placements whose transformed/composed results differ.
type CacheVariant struct {
	Fit, Horizontal, Vertical string
	Width, Height             int
	DPIBits                   uint64
}

type cacheKey struct {
	path    string
	digest  [sha256.Size]byte
	variant CacheVariant
}

type cacheEntry struct {
	key      cacheKey
	source   *Source
	pins     uint32
	lastUsed uint64
}

// Lease pins one decoded source until Release is called.
type Lease struct {
	cache    *Cache
	entry    *cacheEntry
	released bool
}

func (l *Lease) Source() *Source {
	if l == nil || l.released || l.entry == nil {
		return nil
	}
	return l.entry.source
}

// Cache is a synchronous, bounded decoded-source cache. It has no goroutines.
type Cache struct {
	entries map[cacheKey]*cacheEntry
	clock   uint64
	bytes   uint64
	closed  bool
}

func NewCache() *Cache {
	return &Cache{entries: make(map[cacheKey]*cacheEntry, MaxCacheEntries)}
}

// Acquire canonicalizes path, looks up path+digest, and pins a hit. A miss is
// reported as (nil, false, nil).
func (c *Cache) Acquire(path string, digest [sha256.Size]byte) (*Lease, bool, error) {
	return c.AcquireVariant(path, digest, CacheVariant{})
}

func (c *Cache) AcquireVariant(path string, digest [sha256.Size]byte, variant CacheVariant) (*Lease, bool, error) {
	if c == nil || c.closed {
		return nil, false, ErrCacheClosed
	}
	canonical, err := canonicalizePath(path)
	if err != nil {
		return nil, false, fmt.Errorf("cache path: canonicalization failed")
	}
	entry, ok := c.entries[cacheKey{path: canonical, digest: digest, variant: variant}]
	if !ok {
		return nil, false, nil
	}
	c.touch(entry)
	entry.pins++
	return &Lease{cache: c, entry: entry}, true, nil
}

// Insert transfers source ownership to the cache and returns an initial pin.
// On error, ownership remains with the caller.
func (c *Cache) Insert(path string, source *Source) (*Lease, error) {
	return c.InsertVariant(path, source, CacheVariant{})
}

func (c *Cache) InsertVariant(path string, source *Source, variant CacheVariant) (*Lease, error) {
	if c == nil || c.closed {
		return nil, ErrCacheClosed
	}
	if source == nil || source.closed || source.rgba == nil {
		return nil, ErrSourceClosed
	}
	if source.cpuBytes > MaxAggregateCPUBytes {
		return nil, fmt.Errorf("background cache residency: exceeds limit")
	}
	if source.owner != nil {
		return nil, fmt.Errorf("cache insert: source already owned")
	}
	canonical, err := canonicalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("cache path: canonicalization failed")
	}
	key := cacheKey{path: canonical, digest: source.digest, variant: variant}
	if _, exists := c.entries[key]; exists {
		return nil, ErrCacheDuplicate
	}
	for len(c.entries) == MaxCacheEntries || c.bytes > MaxAggregateCPUBytes-source.cpuBytes {
		victim := c.evictionCandidate()
		if victim == nil {
			return nil, ErrCachePinned
		}
		delete(c.entries, victim.key)
		c.bytes -= victim.source.cpuBytes
		_ = victim.source.closeOwned()
	}
	entry := &cacheEntry{key: key, source: source, pins: 1}
	source.owner = c
	c.touch(entry)
	c.entries[key] = entry
	c.bytes += source.cpuBytes
	return &Lease{cache: c, entry: entry}, nil
}

// Release consumes exactly one lease. Releasing the same lease twice is an
// error and never decrements a pin below zero.
func (c *Cache) Release(lease *Lease) error {
	if c == nil || c.closed {
		return ErrCacheClosed
	}
	if lease == nil || lease.cache != c || lease.entry == nil {
		return fmt.Errorf("cache release: foreign lease")
	}
	if lease.released {
		return ErrLeaseReleased
	}
	entry, exists := c.entries[lease.entry.key]
	if !exists || entry != lease.entry || entry.pins == 0 {
		return fmt.Errorf("cache release: invalid pin state")
	}
	entry.pins--
	lease.released = true
	return nil
}

// Close closes every unpinned source exactly once. If any entry is pinned, no
// state is changed and ErrCachePinned is returned. Close is idempotent.
func (c *Cache) Close() error {
	if c == nil || c.closed {
		return nil
	}
	for _, entry := range c.entries {
		if entry.pins != 0 {
			return ErrCachePinned
		}
	}
	for key, entry := range c.entries {
		delete(c.entries, key)
		c.bytes -= entry.source.cpuBytes
		_ = entry.source.closeOwned()
	}
	c.bytes = 0
	c.closed = true
	return nil
}

func (c *Cache) touch(entry *cacheEntry) {
	c.clock++
	entry.lastUsed = c.clock
}

// ResidentBytes reports retained decoded-source bytes.
func (c *Cache) ResidentBytes() uint64 {
	if c == nil {
		return 0
	}
	return c.bytes
}

// TrimFor evicts unpinned LRU entries until resident plus additional fits the CPU ceiling.
func (c *Cache) TrimFor(additional uint64) error {
	if c == nil || c.closed {
		return ErrCacheClosed
	}
	if additional > MaxAggregateCPUBytes {
		return fmt.Errorf("background cache residency: exceeds limit")
	}
	for c.bytes > MaxAggregateCPUBytes-additional {
		victim := c.evictionCandidate()
		if victim == nil {
			return ErrCachePinned
		}
		delete(c.entries, victim.key)
		c.bytes -= victim.source.cpuBytes
		_ = victim.source.closeOwned()
	}
	return nil
}

func (c *Cache) evictionCandidate() *cacheEntry {
	var victim *cacheEntry
	for _, entry := range c.entries {
		if entry.pins != 0 {
			continue
		}
		if victim == nil || entry.lastUsed < victim.lastUsed || (entry.lastUsed == victim.lastUsed && lessCacheKey(entry.key, victim.key)) {
			victim = entry
		}
	}
	return victim
}

func lessCacheKey(left, right cacheKey) bool {
	if left.path != right.path {
		return left.path < right.path
	}
	for index := range left.digest {
		if left.digest[index] != right.digest[index] {
			return left.digest[index] < right.digest[index]
		}
	}
	if left.variant.Fit != right.variant.Fit {
		return left.variant.Fit < right.variant.Fit
	}
	if left.variant.Horizontal != right.variant.Horizontal {
		return left.variant.Horizontal < right.variant.Horizontal
	}
	if left.variant.Vertical != right.variant.Vertical {
		return left.variant.Vertical < right.variant.Vertical
	}
	if left.variant.Width != right.variant.Width {
		return left.variant.Width < right.variant.Width
	}
	if left.variant.Height != right.variant.Height {
		return left.variant.Height < right.variant.Height
	}
	return left.variant.DPIBits < right.variant.DPIBits
}
