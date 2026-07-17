package fontglyph

import (
	"bytes"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gomono"
)

func useTestFontCache(t *testing.T, maxFaces int, maxBytes int64) *fontCacheManager {
	t.Helper()
	manager := newFontCacheManager(maxFaces, maxBytes)
	restore := resetFontCacheForTest(manager)
	t.Cleanup(restore)
	return manager
}

func releaseLoadedFace(face loadedFace) {
	face.cacheHandle.release()
	if closer, ok := face.face.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func TestFontParseCacheReusesParseAcrossSizes(t *testing.T) {
	manager := useTestFontCache(t, 4, int64(len(gomono.TTF))*2)
	var calls atomic.Int32
	load := func() ([]byte, error) {
		calls.Add(1)
		return gomono.TTF, nil
	}
	const key = "test:cache-reuse-fixture"
	f1, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 12, DPI: 96}, load)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	defer releaseLoadedFace(f1)
	f2, _, err := loadCachedFaceIndex(key, 0, Spec{Family: "Go Mono", Size: 24, DPI: 96}, load)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	defer releaseLoadedFace(f2)
	if calls.Load() != 1 {
		t.Fatalf("loads = %d, want 1", calls.Load())
	}
	if f1.sfnt == nil || f1.sfnt != f2.sfnt {
		t.Fatal("faces do not share parsed sfnt")
	}
	if f1.face == f2.face {
		t.Fatal("point sizes unexpectedly share font.Face")
	}
	if got := manager.stats().Pinned; got != 2 {
		t.Fatalf("pins = %d, want 2", got)
	}
}

func TestFontParseCacheConcurrentMissSingleLoad(t *testing.T) {
	manager := newFontCacheManager(4, 1024)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	var calls atomic.Int32
	start := make(chan struct{})
	load := func() ([]byte, error) {
		calls.Add(1)
		<-start
		return make([]byte, 16), nil
	}
	const goroutines = 12
	var wg sync.WaitGroup
	handles := make(chan *parsedFontHandle, goroutines)
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, handle, err := manager.acquire("test:concurrent", 0, 0, load)
			handles <- handle
			errs <- err
		}()
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(start)
	wg.Wait()
	close(handles)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
	}
	for handle := range handles {
		handle.release()
	}
	if calls.Load() != 1 {
		t.Fatalf("loads = %d, want 1", calls.Load())
	}
}

func TestFontParseCacheFailureWakesWaitersAndRetries(t *testing.T) {
	manager := newFontCacheManager(2, 100)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	loadErr := errors.New("load failed")
	var calls atomic.Int32
	load := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			return nil, loadErr
		}
		return []byte("ok"), nil
	}
	if _, _, err := manager.acquire("test:retry", 0, 0, load); !errors.Is(err, loadErr) {
		t.Fatalf("first error = %v", err)
	}
	_, handle, err := manager.acquire("test:retry", 0, 0, load)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	handle.release()
	if calls.Load() != 2 {
		t.Fatalf("loads = %d, want 2", calls.Load())
	}
}

func TestFontParseCachePinLRUAndBackendCloseIdempotent(t *testing.T) {
	manager := newFontCacheManager(2, 100)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	load := func() ([]byte, error) { return make([]byte, 10), nil }
	_, first, _ := manager.acquire("test:a", 0, 10, load)
	_, second, _ := manager.acquire("test:b", 0, 10, load)
	first.release()
	backend := &OpenTypeBackend{faces: []loadedFace{{cacheHandle: second}}}
	backend.Close()
	backend.Close()
	if got := manager.stats().Pinned; got != 0 {
		t.Fatalf("pins after close = %d, want 0", got)
	}
	_, third, err := manager.acquire("test:c", 0, 10, load)
	if err != nil {
		t.Fatalf("third acquire: %v", err)
	}
	third.release()
	manager.mu.Lock()
	_, hasA := manager.entries[fontCacheKey("test:a", 0)]
	_, hasB := manager.entries[fontCacheKey("test:b", 0)]
	manager.mu.Unlock()
	if hasA || !hasB {
		t.Fatalf("LRU entries: hasA=%v hasB=%v, want false,true", hasA, hasB)
	}
}

func TestFontParseCachePinnedCapacityRefusal(t *testing.T) {
	manager := newFontCacheManager(1, 10)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	_, pinned, err := manager.acquire("test:pinned", 0, 0, func() ([]byte, error) { return make([]byte, 10), nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.acquire("test:other", 0, 0, func() ([]byte, error) { return []byte{1}, nil }); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("face-cap error = %v", err)
	}
	if got := manager.stats(); got.Entries != 1 || got.Bytes != 10 || got.Pinned != 1 {
		t.Fatalf("active entry changed: %+v", got)
	}
	pinned.release()
}

func TestFontParseCacheOversizedAndOverflow(t *testing.T) {
	manager := newFontCacheManager(2, 8)
	var parses atomic.Int32
	manager.parse = func([]byte, int) (*parsedFontData, error) {
		parses.Add(1)
		return &parsedFontData{}, nil
	}
	if _, _, err := manager.acquire("test:oversized", 0, 0, func() ([]byte, error) { return make([]byte, 9), nil }); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("oversized error = %v", err)
	}
	if parses.Load() != 0 {
		t.Fatal("oversized data was parsed")
	}
	if got := manager.stats(); got.Entries != 0 || got.Bytes != 0 {
		t.Fatalf("oversized reservation leaked: %+v", got)
	}
	overflow := newFontCacheManager(2, math.MaxInt64)
	overflow.bytes = math.MaxInt64
	if _, _, err := overflow.acquire("test:overflow", 0, 1, func() ([]byte, error) { return []byte{1}, nil }); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestFontParseOccursOutsideCacheLock(t *testing.T) {
	manager := newFontCacheManager(1, 100)
	manager.parse = func([]byte, int) (*parsedFontData, error) {
		_ = manager.stats() // Deadlocks if parsing happens while manager.mu is held.
		return &parsedFontData{}, nil
	}
	done := make(chan error, 1)
	go func() {
		_, handle, err := manager.acquire("test:outside-lock", 0, 0, func() ([]byte, error) { return []byte{1}, nil })
		if handle != nil {
			handle.release()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parse appears to run under cache mutex")
	}
}

func TestFontParseCacheWaiterKeepsPublishedEntryPinned(t *testing.T) {
	manager := newFontCacheManager(1, 8)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	loadStarted := make(chan struct{})
	finishLoad := make(chan struct{})
	waiterWoke := make(chan struct{})
	releaseWaiter := make(chan struct{})
	manager.afterWait = func() {
		close(waiterWoke)
		<-releaseWaiter
	}

	loaderResult := make(chan *parsedFontHandle, 1)
	go func() {
		_, handle, _ := manager.acquire("test:shared", 0, 1, func() ([]byte, error) {
			close(loadStarted)
			<-finishLoad
			return []byte{1}, nil
		})
		loaderResult <- handle
	}()
	<-loadStarted

	var waiterLoads atomic.Int32
	waiterResult := make(chan *parsedFontHandle, 1)
	go func() {
		_, handle, _ := manager.acquire("test:shared", 0, 1, func() ([]byte, error) {
			waiterLoads.Add(1)
			return nil, nil
		})
		waiterResult <- handle
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		manager.mu.Lock()
		entry := manager.entries[fontCacheKey("test:shared", 0)]
		joined := entry != nil && entry.waiters == 1
		manager.mu.Unlock()
		if joined {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("waiter did not join loading entry")
		}
		time.Sleep(time.Millisecond)
	}
	if waiterLoads.Load() != 0 {
		t.Fatal("waiter invoked loader")
	}
	close(finishLoad)
	loader := <-loaderResult
	loader.release()
	<-waiterWoke
	if _, _, err := manager.acquire("test:competitor", 0, 1, func() ([]byte, error) { return []byte{2}, nil }); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("competitor error while waiter pin reserved = %v", err)
	}
	close(releaseWaiter)
	waiter := <-waiterResult
	waiter.release()
	_, competitor, err := manager.acquire("test:competitor", 0, 1, func() ([]byte, error) { return []byte{2}, nil })
	if err != nil {
		t.Fatalf("competitor after waiter release: %v", err)
	}
	competitor.release()
}

func TestFontParseCacheUnknownSizeAdmissionIsPessimistic(t *testing.T) {
	manager := newFontCacheManager(2, 8)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	started := make(chan struct{})
	finish := make(chan struct{})
	firstResult := make(chan *parsedFontHandle, 1)
	go func() {
		_, handle, _ := manager.acquire("test:unknown-a", 0, 0, func() ([]byte, error) {
			close(started)
			<-finish
			return []byte{1}, nil
		})
		firstResult <- handle
	}()
	<-started
	var secondLoads atomic.Int32
	if _, _, err := manager.acquire("test:unknown-b", 0, 0, func() ([]byte, error) {
		secondLoads.Add(1)
		return []byte{2}, nil
	}); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("second unknown-size error = %v", err)
	}
	if secondLoads.Load() != 0 {
		t.Fatal("rejected unknown-size load callback ran")
	}
	close(finish)
	(<-firstResult).release()
}

func TestFontParseCacheSharesSourceBlobAcrossIndices(t *testing.T) {
	manager := newFontCacheManager(2, 64)
	blob := make([]byte, 7)
	var loads, parses atomic.Int32
	manager.parse = func([]byte, int) (*parsedFontData, error) {
		parses.Add(1)
		return &parsedFontData{}, nil
	}
	load := func() ([]byte, error) { loads.Add(1); return blob, nil }
	_, zero, err := manager.acquire("test:collection", 0, int64(len(blob)), load)
	if err != nil {
		t.Fatal(err)
	}
	_, one, err := manager.acquire("test:collection", 1, int64(len(blob)), load)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.stats(); loads.Load() != 1 || parses.Load() != 2 || got.Bytes != int64(len(blob)) || got.Entries != 2 || got.Pinned != 2 {
		t.Fatalf("loads=%d parses=%d stats=%+v", loads.Load(), parses.Load(), got)
	}
	zero.release()
	if got := manager.stats(); got.Pinned != 1 || got.Bytes != int64(len(blob)) {
		t.Fatalf("independent release stats=%+v", got)
	}
	one.release()

	other := make([]byte, 5)
	_, replacement, err := manager.acquire("test:replacement", 0, int64(len(other)), func() ([]byte, error) { return other, nil })
	if err != nil {
		t.Fatal(err)
	}
	replacement.release()
	if got := manager.stats(); got.Entries != 2 || got.Bytes != int64(len(blob)+len(other)) {
		t.Fatalf("one-face eviction stats=%+v", got)
	}
	final := make([]byte, 6)
	_, last, err := manager.acquire("test:final", 0, int64(len(final)), func() ([]byte, error) { return final, nil })
	if err != nil {
		t.Fatal(err)
	}
	last.release()
	if got := manager.stats(); got.Entries != 2 || got.Bytes != int64(len(other)+len(final)) {
		t.Fatalf("final source eviction stats=%+v", got)
	}
}

func TestFontParseCacheConcurrentDifferentIndicesShareLoad(t *testing.T) {
	manager := newFontCacheManager(2, 16)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	started := make(chan struct{})
	finish := make(chan struct{})
	var loads atomic.Int32
	load := func() ([]byte, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-finish
		return make([]byte, 8), nil
	}
	type result struct {
		handle *parsedFontHandle
		err    error
	}
	results := make(chan result, 2)
	go func() { _, handle, err := manager.acquire("test:ttc", 0, 8, load); results <- result{handle, err} }()
	<-started
	go func() { _, handle, err := manager.acquire("test:ttc", 1, 8, load); results <- result{handle, err} }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if manager.stats().Entries == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("different-index waiter did not join source load")
		}
		time.Sleep(time.Millisecond)
	}
	close(finish)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.handle.release()
	}
	if got := manager.stats(); loads.Load() != 1 || got.Entries != 2 || got.Bytes != 8 {
		t.Fatalf("loads=%d stats=%+v", loads.Load(), got)
	}
}

func TestFontParseCacheSharedLoadFailureWakesIndicesAndRetries(t *testing.T) {
	manager := newFontCacheManager(2, 16)
	manager.parse = func([]byte, int) (*parsedFontData, error) { return &parsedFontData{}, nil }
	loadErr := errors.New("shared load failed")
	started := make(chan struct{})
	finish := make(chan struct{})
	var loads atomic.Int32
	load := func() ([]byte, error) {
		if loads.Add(1) == 1 {
			close(started)
			<-finish
			return nil, loadErr
		}
		return []byte{1, 2, 3}, nil
	}
	errs := make(chan error, 2)
	go func() { _, _, err := manager.acquire("test:failed-ttc", 0, 8, load); errs <- err }()
	<-started
	go func() { _, _, err := manager.acquire("test:failed-ttc", 1, 8, load); errs <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if manager.stats().Entries == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("different-index failure waiter did not join")
		}
		time.Sleep(time.Millisecond)
	}
	close(finish)
	for range 2 {
		if err := <-errs; !errors.Is(err, loadErr) {
			t.Fatalf("shared error=%v", err)
		}
	}
	if got := manager.stats(); got.Entries != 0 || got.Bytes != 0 {
		t.Fatalf("failed source leaked: %+v", got)
	}
	_, handle, err := manager.acquire("test:failed-ttc", 1, 3, load)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	handle.release()
	if loads.Load() != 2 {
		t.Fatalf("loads=%d, want 2", loads.Load())
	}
}

func TestReadFontFileBoundedRejectsGrowthAndOversize(t *testing.T) {
	if _, err := readFontFileBounded(bytes.NewReader(make([]byte, 7)), 5, 8); !errors.Is(err, errFontFileGrew) {
		t.Fatalf("growth error = %v", err)
	}
	if _, err := readFontFileBounded(bytes.NewReader(make([]byte, 9)), 8, 8); !errors.Is(err, errFontCacheCapacity) {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestOpenTypeBackendCloseReleasesPinAndRejectsRaster(t *testing.T) {
	manager := useTestFontCache(t, 2, int64(len(gomono.TTF))*2)
	face, metrics, err := loadCachedFaceIndexKnownSize("test:backend-close", 0, int64(len(gomono.TTF)), Spec{Family: "Go Mono", Size: 12, DPI: 96}, func() ([]byte, error) { return gomono.TTF, nil })
	if err != nil {
		t.Fatal(err)
	}
	backend := &OpenTypeBackend{faces: []loadedFace{face}, cellW: 8, cellH: 16, baseline: metrics.Ascent.Ceil()}
	backend.Close()
	backend.Close()
	if got := manager.stats().Pinned; got != 0 {
		t.Fatalf("pins after backend close = %d", got)
	}
	if _, ok := backend.Rasterize('A', 1); ok {
		t.Fatal("closed backend rasterized a glyph")
	}
}
