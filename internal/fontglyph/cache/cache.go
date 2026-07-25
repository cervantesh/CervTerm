package cache

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"cervterm/internal/fontglyph/internal/face"
)

func New[T any](maxFaces int, maxBytes int64, parse Parser[T]) *Manager[T] {
	return &Manager[T]{
		entries:  make(map[string]*entry[T]),
		sources:  make(map[string]*sourceBlob),
		maxFaces: maxFaces, maxBytes: maxBytes, parse: parse,
	}
}

// MaxBytes returns the immutable retained-source byte budget.
func (m *Manager[T]) MaxBytes() int64 {
	if m == nil {
		return 0
	}
	return m.maxBytes
}

func (m *Manager[T]) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := Stats{Entries: len(m.entries), Bytes: m.bytes}
	for _, entry := range m.entries {
		switch entry.state {
		case loading:
			stats.Loading++
		case ready:
			stats.Ready++
		}
		stats.Pinned += entry.pins
	}
	return stats
}
func CanonicalSource(source string) string {
	if strings.HasPrefix(source, "embedded:") || strings.HasPrefix(source, "test:") {
		return source
	}
	canonical, err := filepath.Abs(source)
	if err != nil {
		canonical = filepath.Clean(source)
	}
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = resolved
	}
	canonical = filepath.Clean(canonical)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	return canonical
}
func Key(source string, index int) string {
	return CanonicalSource(source) + "#" + strconv.Itoa(index)
}
func checkedAddInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// acquire singleflights source loading and exact source#index parsing.
func (m *Manager[T]) Acquire(source string, index int, knownSize int64, load func() ([]byte, error)) (*face.Owner[T], *Lease[T], error) {
	if knownSize < 0 {
		return nil, nil, ErrCapacity
	}
	reservation := knownSize
	if reservation == 0 {
		reservation = m.maxBytes
	}
	canonical := CanonicalSource(source)
	key := canonical + "#" + strconv.Itoa(index)
	m.mu.Lock()
	if entry := m.entries[key]; entry != nil {
		if entry.state == ready {
			entry.pins++
			m.touchLocked(entry)
			owner := entry.owner
			m.mu.Unlock()
			return owner, &Lease[T]{manager: m, entry: entry}, nil
		}
		entry.waiters++
		wait := entry.wait
		m.mu.Unlock()
		<-wait
		if m.afterWait != nil {
			m.afterWait()
		}
		m.mu.Lock()
		err, owner := entry.err, entry.owner
		m.mu.Unlock()
		if err != nil {
			return nil, nil, err
		}
		return owner, &Lease[T]{manager: m, entry: entry}, nil
	}
	blob := m.sources[canonical]
	sourceLoader := blob == nil
	if sourceLoader {
		if !m.makeRoomLocked(1, reservation) {
			m.mu.Unlock()
			return nil, nil, ErrCapacity
		}
		blob = &sourceBlob{state: loading, wait: make(chan struct{}), reserved: reservation, refs: 1}
		m.sources[canonical] = blob
		m.bytes += reservation
	} else {
		blob.refs++ // Pending face keeps ready data alive while admission evicts.
		if !m.makeRoomLocked(1, 0) {
			blob.refs--
			m.mu.Unlock()
			return nil, nil, ErrCapacity
		}
	}
	entry := &entry[T]{state: loading, wait: make(chan struct{}), source: blob}
	m.entries[key] = entry
	waitForSource := !sourceLoader && blob.state == loading
	m.mu.Unlock()
	if sourceLoader {
		data, err := load()
		if err == nil && int64(len(data)) > m.maxBytes {
			err = fmt.Errorf("%w: font is %d bytes (limit %d)", ErrCapacity, len(data), m.maxBytes)
		}
		if err != nil {
			m.failSource(canonical, blob, err)
			return nil, nil, err
		}
		if err = m.publishSource(blob, data); err != nil {
			m.failSource(canonical, blob, err)
			return nil, nil, err
		}
	} else if waitForSource {
		<-blob.wait
		if blob.err != nil {
			return nil, nil, blob.err
		}
	}
	owner, err := m.parse(blob.data, index) // Deliberately outside m.mu.
	if err != nil {
		m.failFace(key, entry, err)
		return nil, nil, err
	}
	m.mu.Lock()
	entry.owner = owner
	entry.state = ready
	entry.pins = 1 + entry.waiters
	entry.waiters = 0
	m.touchLocked(entry)
	close(entry.wait)
	m.mu.Unlock()
	return owner, &Lease[T]{manager: m, entry: entry}, nil
}
func (m *Manager[T]) publishSource(blob *sourceBlob, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	actual := int64(len(data))
	delta := actual - blob.reserved
	if delta > 0 && !m.makeRoomLocked(0, delta) {
		return ErrCapacity
	}
	m.bytes += delta
	blob.reserved = 0
	blob.data = data
	blob.state = ready
	close(blob.wait)
	return nil
}
func (m *Manager[T]) failSource(source string, blob *sourceBlob, err error) {
	m.mu.Lock()
	if m.sources[source] == blob {
		delete(m.sources, source)
		m.bytes -= blob.reserved
		blob.reserved = 0
		for key, entry := range m.entries {
			if entry.source == blob {
				delete(m.entries, key)
				entry.err = err
				entry.state = missing
				close(entry.wait)
			}
		}
		blob.refs = 0
		blob.err = err
		close(blob.wait)
	}
	m.mu.Unlock()
}
func (m *Manager[T]) failFace(key string, entry *entry[T], err error) {
	m.mu.Lock()
	if m.entries[key] == entry {
		delete(m.entries, key)
		m.releaseSourceLocked(entry.source)
	}
	entry.err = err
	entry.state = missing
	close(entry.wait)
	m.mu.Unlock()
}
func (m *Manager[T]) touchLocked(entry *entry[T]) {
	m.clock++
	entry.lastUsed = m.clock
}
func (m *Manager[T]) sourceCharge(blob *sourceBlob) int64 {
	if blob.reserved != 0 {
		return blob.reserved
	}
	return int64(len(blob.data))
}
func (m *Manager[T]) releaseSourceLocked(blob *sourceBlob) {
	blob.refs--
	if blob.refs != 0 {
		return
	}
	for source, candidate := range m.sources {
		if candidate == blob {
			delete(m.sources, source)
			break
		}
	}
	m.bytes -= m.sourceCharge(blob)
	blob.data = nil
}
func (m *Manager[T]) removeFaceLocked(key string, entry *entry[T]) {
	delete(m.entries, key)
	m.releaseSourceLocked(entry.source)
	entry.owner.Close()
}
func (m *Manager[T]) makeRoomLocked(extraFaces int, extraBytes int64) bool {
	if extraFaces < 0 || extraBytes < 0 || extraFaces > m.maxFaces || extraBytes > m.maxBytes {
		return false
	}
	for {
		bytesAfter, ok := checkedAddInt64(m.bytes, extraBytes)
		if ok && len(m.entries)+extraFaces <= m.maxFaces && bytesAfter <= m.maxBytes {
			return true
		}
		var oldestKey string
		var oldest *entry[T]
		for key, entry := range m.entries {
			if entry.state != ready || entry.pins != 0 {
				continue
			}
			if oldest == nil || entry.lastUsed < oldest.lastUsed || entry.lastUsed == oldest.lastUsed && key < oldestKey {
				oldestKey, oldest = key, entry
			}
		}
		if oldest == nil {
			return false
		}
		m.removeFaceLocked(oldestKey, oldest)
	}
}

// ReadFileBounded reads no more than the reserved stat size and hard budget.
func ReadFileBounded(reader io.Reader, reserved, maxBytes int64) ([]byte, error) {
	if reserved < 0 || maxBytes < 0 || reserved > maxBytes {
		return nil, ErrCapacity
	}
	limit := reserved
	if limit < math.MaxInt64 {
		limit++
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, err
	}
	actual := int64(len(data))
	if actual > maxBytes {
		return nil, fmt.Errorf("%w: font is larger than %d bytes", ErrCapacity, maxBytes)
	}
	if actual > reserved {
		return nil, fmt.Errorf("%w: stat=%d read=%d", ErrFileGrew, reserved, actual)
	}
	return data, nil
}
