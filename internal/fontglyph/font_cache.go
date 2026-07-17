package fontglyph

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"cervterm/internal/fontdesc"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// parsedFontData is the size-independent result of parsing a font file.
type parsedFontData struct {
	sfnt   *sfnt.Font
	tables ColorTables
	sbix   *sbixExtractor
	cbdt   *cbdtExtractor
	colr   *colrParser
	svg    *svgExtractor
}
type fontCacheState uint8

const (
	fontCacheMissing fontCacheState = iota
	fontCacheLoading
	fontCacheReady
)

var (
	errFontCacheCapacity = errors.New("font parse cache capacity exceeded")
	errFontFileGrew      = errors.New("font file grew while loading")
)

type fontSourceBlob struct {
	state    fontCacheState
	wait     chan struct{}
	data     []byte
	err      error
	reserved int64
	refs     int
}
type fontCacheEntry struct {
	state    fontCacheState
	wait     chan struct{}
	waiters  int
	parsed   *parsedFontData
	err      error
	source   *fontSourceBlob
	pins     int
	lastUsed uint64
}
type fontCacheManager struct {
	mu        sync.Mutex
	entries   map[string]*fontCacheEntry
	sources   map[string]*fontSourceBlob
	maxFaces  int
	maxBytes  int64
	bytes     int64
	clock     uint64
	parse     func([]byte, int) (*parsedFontData, error)
	afterWait func() // test-only scheduling seam; nil in production
}
type fontCacheStats struct {
	Entries int
	Loading int
	Ready   int
	Pinned  int
	Bytes   int64
}
type parsedFontHandle struct {
	once    sync.Once
	manager *fontCacheManager
	entry   *fontCacheEntry
}

func (h *parsedFontHandle) release() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		m := h.manager
		m.mu.Lock()
		if h.entry.pins > 0 {
			h.entry.pins--
		}
		m.mu.Unlock()
	})
}
func newFontCacheManager(maxFaces int, maxBytes int64) *fontCacheManager {
	return &fontCacheManager{
		entries:  make(map[string]*fontCacheEntry),
		sources:  make(map[string]*fontSourceBlob),
		maxFaces: maxFaces, maxBytes: maxBytes, parse: parseFontData,
	}
}

var (
	fontCacheManagerMu sync.Mutex
	fontCache          = newFontCacheManager(fontdesc.MaxParsedFaces, fontdesc.MaxParsedBytes)
)

func currentFontCache() *fontCacheManager {
	fontCacheManagerMu.Lock()
	defer fontCacheManagerMu.Unlock()
	return fontCache
}

// resetFontCacheForTest installs an isolated manager.
func resetFontCacheForTest(manager *fontCacheManager) func() {
	fontCacheManagerMu.Lock()
	previous := fontCache
	fontCache = manager
	fontCacheManagerMu.Unlock()
	return func() {
		fontCacheManagerMu.Lock()
		fontCache = previous
		fontCacheManagerMu.Unlock()
	}
}
func (m *fontCacheManager) stats() fontCacheStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := fontCacheStats{Entries: len(m.entries), Bytes: m.bytes}
	for _, entry := range m.entries {
		switch entry.state {
		case fontCacheLoading:
			stats.Loading++
		case fontCacheReady:
			stats.Ready++
		}
		stats.Pinned += entry.pins
	}
	return stats
}
func canonicalFontCacheSource(source string) string {
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
func fontCacheKey(source string, index int) string {
	return canonicalFontCacheSource(source) + "#" + strconv.Itoa(index)
}
func checkedAddInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// acquire singleflights source loading and exact source#index parsing.
func (m *fontCacheManager) acquire(source string, index int, knownSize int64, load func() ([]byte, error)) (*parsedFontData, *parsedFontHandle, error) {
	if knownSize < 0 {
		return nil, nil, errFontCacheCapacity
	}
	reservation := knownSize
	if reservation == 0 {
		reservation = m.maxBytes
	}
	canonical := canonicalFontCacheSource(source)
	key := canonical + "#" + strconv.Itoa(index)
	m.mu.Lock()
	if entry := m.entries[key]; entry != nil {
		if entry.state == fontCacheReady {
			entry.pins++
			m.touchLocked(entry)
			parsed := entry.parsed
			m.mu.Unlock()
			return parsed, &parsedFontHandle{manager: m, entry: entry}, nil
		}
		entry.waiters++
		wait := entry.wait
		m.mu.Unlock()
		<-wait
		if m.afterWait != nil {
			m.afterWait()
		}
		m.mu.Lock()
		err, parsed := entry.err, entry.parsed
		m.mu.Unlock()
		if err != nil {
			return nil, nil, err
		}
		return parsed, &parsedFontHandle{manager: m, entry: entry}, nil
	}
	blob := m.sources[canonical]
	sourceLoader := blob == nil
	if sourceLoader {
		if !m.makeRoomLocked(1, reservation) {
			m.mu.Unlock()
			return nil, nil, errFontCacheCapacity
		}
		blob = &fontSourceBlob{state: fontCacheLoading, wait: make(chan struct{}), reserved: reservation, refs: 1}
		m.sources[canonical] = blob
		m.bytes += reservation
	} else {
		blob.refs++ // Pending face keeps ready data alive while admission evicts.
		if !m.makeRoomLocked(1, 0) {
			blob.refs--
			m.mu.Unlock()
			return nil, nil, errFontCacheCapacity
		}
	}
	entry := &fontCacheEntry{state: fontCacheLoading, wait: make(chan struct{}), source: blob}
	m.entries[key] = entry
	waitForSource := !sourceLoader && blob.state == fontCacheLoading
	m.mu.Unlock()
	if sourceLoader {
		data, err := load()
		if err == nil && int64(len(data)) > m.maxBytes {
			err = fmt.Errorf("%w: font is %d bytes (limit %d)", errFontCacheCapacity, len(data), m.maxBytes)
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
	parsed, err := m.parse(blob.data, index) // Deliberately outside m.mu.
	if err != nil {
		m.failFace(key, entry, err)
		return nil, nil, err
	}
	m.mu.Lock()
	entry.parsed = parsed
	entry.state = fontCacheReady
	entry.pins = 1 + entry.waiters
	entry.waiters = 0
	m.touchLocked(entry)
	close(entry.wait)
	m.mu.Unlock()
	return parsed, &parsedFontHandle{manager: m, entry: entry}, nil
}
func (m *fontCacheManager) publishSource(blob *fontSourceBlob, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	actual := int64(len(data))
	delta := actual - blob.reserved
	if delta > 0 && !m.makeRoomLocked(0, delta) {
		return errFontCacheCapacity
	}
	m.bytes += delta
	blob.reserved = 0
	blob.data = data
	blob.state = fontCacheReady
	close(blob.wait)
	return nil
}
func (m *fontCacheManager) failSource(source string, blob *fontSourceBlob, err error) {
	m.mu.Lock()
	if m.sources[source] == blob {
		delete(m.sources, source)
		m.bytes -= blob.reserved
		blob.reserved = 0
		for key, entry := range m.entries {
			if entry.source == blob {
				delete(m.entries, key)
				entry.err = err
				entry.state = fontCacheMissing
				close(entry.wait)
			}
		}
		blob.refs = 0
		blob.err = err
		close(blob.wait)
	}
	m.mu.Unlock()
}
func (m *fontCacheManager) failFace(key string, entry *fontCacheEntry, err error) {
	m.mu.Lock()
	if m.entries[key] == entry {
		delete(m.entries, key)
		m.releaseSourceLocked(entry.source)
	}
	entry.err = err
	entry.state = fontCacheMissing
	close(entry.wait)
	m.mu.Unlock()
}
func (m *fontCacheManager) touchLocked(entry *fontCacheEntry) {
	m.clock++
	entry.lastUsed = m.clock
}
func (m *fontCacheManager) sourceCharge(blob *fontSourceBlob) int64 {
	if blob.reserved != 0 {
		return blob.reserved
	}
	return int64(len(blob.data))
}
func (m *fontCacheManager) releaseSourceLocked(blob *fontSourceBlob) {
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
func (m *fontCacheManager) removeFaceLocked(key string, entry *fontCacheEntry) {
	delete(m.entries, key)
	m.releaseSourceLocked(entry.source)
}
func (m *fontCacheManager) makeRoomLocked(extraFaces int, extraBytes int64) bool {
	if extraFaces < 0 || extraBytes < 0 || extraFaces > m.maxFaces || extraBytes > m.maxBytes {
		return false
	}
	for {
		bytesAfter, ok := checkedAddInt64(m.bytes, extraBytes)
		if ok && len(m.entries)+extraFaces <= m.maxFaces && bytesAfter <= m.maxBytes {
			return true
		}
		var oldestKey string
		var oldest *fontCacheEntry
		for key, entry := range m.entries {
			if entry.state != fontCacheReady || entry.pins != 0 {
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

// parseFontData does the expensive, size-independent work.
func parseFontData(data []byte, index int) (*parsedFontData, error) {
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	parsed, err := collection.Font(index)
	if err != nil {
		return nil, err
	}
	pf := &parsedFontData{sfnt: parsed}
	if index != 0 {
		return pf, nil
	}
	tables, err := DetectColorTables(data)
	if err != nil {
		return pf, nil
	}
	pf.tables = tables
	if tables.HasSbix && parsed != nil {
		if table, ok, tableErr := getSFNTTable(data, "sbix"); tableErr == nil && ok {
			pf.sbix, _ = newSbixExtractor(table, parsed.NumGlyphs())
		}
	}
	if tables.HasCBDT && tables.HasCBLC {
		cbdt, hasCBDT, cbdtErr := getSFNTTable(data, "CBDT")
		cblc, hasCBLC, cblcErr := getSFNTTable(data, "CBLC")
		if cbdtErr == nil && cblcErr == nil && hasCBDT && hasCBLC {
			pf.cbdt, _ = newCBDTExtractor(cbdt, cblc)
		}
	}
	if tables.HasRenderableLayerColor() {
		colr, hasCOLR, colrErr := getSFNTTable(data, "COLR")
		cpal, hasCPAL, cpalErr := getSFNTTable(data, "CPAL")
		if colrErr == nil && cpalErr == nil && hasCOLR && hasCPAL {
			pf.colr, _ = newCOLRParser(colr, cpal)
		}
	}
	if tables.HasSVG {
		if table, ok, tableErr := getSFNTTable(data, "SVG "); tableErr == nil && ok {
			pf.svg, _ = newSVGExtractor(table)
		}
	}
	return pf, nil
}
