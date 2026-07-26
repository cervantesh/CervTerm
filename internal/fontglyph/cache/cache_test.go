package cache

import (
	"bytes"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cervterm/internal/fontglyph/internal/face"
)

type testParsed struct{}

func newTestManager(maxFaces int, maxBytes int64) *Manager[testParsed] {
	return New(maxFaces, maxBytes, func([]byte, int) (*face.Owner[testParsed], error) {
		return face.NewOwner(testParsed{}, nil), nil
	})
}

func TestFontParseCacheConcurrentMissSingleLoad(t *testing.T) {
	manager := newTestManager(4, 1024)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
	var calls atomic.Int32
	start := make(chan struct{})
	load := func() ([]byte, error) {
		calls.Add(1)
		<-start
		return make([]byte, 16), nil
	}
	const goroutines = 12
	var wg sync.WaitGroup
	handles := make(chan *Lease[testParsed], goroutines)
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, handle, err := manager.Acquire("test:concurrent", 0, 0, load)
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
		handle.Close()
	}
	if calls.Load() != 1 {
		t.Fatalf("loads = %d, want 1", calls.Load())
	}
}

func TestFontParseCacheFailureWakesWaitersAndRetries(t *testing.T) {
	manager := newTestManager(2, 100)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
	loadErr := errors.New("load failed")
	var calls atomic.Int32
	load := func() ([]byte, error) {
		if calls.Add(1) == 1 {
			return nil, loadErr
		}
		return []byte("ok"), nil
	}
	if _, _, err := manager.Acquire("test:retry", 0, 0, load); !errors.Is(err, loadErr) {
		t.Fatalf("first error = %v", err)
	}
	_, handle, err := manager.Acquire("test:retry", 0, 0, load)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	handle.Close()
	if calls.Load() != 2 {
		t.Fatalf("loads = %d, want 2", calls.Load())
	}
}

func TestFontParseCacheLRUEvictionClosesFaceOwner(t *testing.T) {
	var closes atomic.Int32
	manager := New(2, 16, func(data []byte, _ int) (*face.Owner[testParsed], error) {
		return face.NewOwner(testParsed{}, func(testParsed) { closes.Add(1) }), nil
	})
	load := func(value byte) func() ([]byte, error) {
		return func() ([]byte, error) { return []byte{value}, nil }
	}
	_, first, err := manager.Acquire("test:a", 0, 1, load('a'))
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := manager.Acquire("test:b", 0, 1, load('b'))
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	first.Close()
	second.Close()
	_, third, err := manager.Acquire("test:c", 0, 1, load('c'))
	if err != nil {
		t.Fatal(err)
	}
	third.Close()
	if closes.Load() != 1 {
		t.Fatalf("owner closes after one eviction = %d, want 1", closes.Load())
	}
	manager.mu.Lock()
	_, hasA := manager.entries[Key("test:a", 0)]
	_, hasB := manager.entries[Key("test:b", 0)]
	manager.mu.Unlock()
	if hasA || !hasB {
		t.Fatalf("LRU entries hasA=%v hasB=%v, want false,true", hasA, hasB)
	}
}

func TestFontParseCacheEvictionCloserCanReenterManager(t *testing.T) {
	var manager *Manager[testParsed]
	var callbackCalls atomic.Int32
	var callbackStats Stats
	var callbackAcquireErr error
	manager = New(1, 1, func(data []byte, _ int) (*face.Owner[testParsed], error) {
		var closeOwner func(testParsed)
		if len(data) == 1 && data[0] == 'a' {
			closeOwner = func(testParsed) {
				callbackCalls.Add(1)
				callbackStats = manager.Stats()
				_, lease, err := manager.Acquire("test:b", 0, 1, func() ([]byte, error) { return []byte{'b'}, nil })
				if lease != nil {
					lease.Close()
				}
				callbackAcquireErr = err
			}
		}
		return face.NewOwner(testParsed{}, closeOwner), nil
	})
	_, first, err := manager.Acquire("test:a", 0, 1, func() ([]byte, error) { return []byte{'a'}, nil })
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	type result struct {
		lease *Lease[testParsed]
		err   error
	}
	done := make(chan result, 1)
	go func() {
		_, lease, acquireErr := manager.Acquire("test:b", 0, 1, func() ([]byte, error) { return []byte{'b'}, nil })
		done <- result{lease: lease, err: acquireErr}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("outer acquire: %v", got.err)
		}
		defer got.lease.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant eviction closer deadlocked cache manager")
	}
	if callbackCalls.Load() != 1 {
		t.Fatalf("closer calls = %d, want 1", callbackCalls.Load())
	}
	if callbackAcquireErr != nil {
		t.Fatalf("reentrant same-key acquire error = %v", callbackAcquireErr)
	}
	if callbackStats.Entries != 1 || callbackStats.Ready != 1 || callbackStats.Pinned != 1 || callbackStats.Bytes != 1 {
		t.Fatalf("callback observed non-atomic replacement accounting: %+v", callbackStats)
	}
}

func TestFontParseCacheSlowEvictionCloserDoesNotHoldManagerLock(t *testing.T) {
	started := make(chan struct{})
	releaseCloser := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCloser) }) }
	defer release()

	manager := New(1, 1, func(data []byte, _ int) (*face.Owner[testParsed], error) {
		var closeOwner func(testParsed)
		if len(data) == 1 && data[0] == 'a' {
			closeOwner = func(testParsed) {
				close(started)
				<-releaseCloser
			}
		}
		return face.NewOwner(testParsed{}, closeOwner), nil
	})
	_, first, err := manager.Acquire("test:slow-a", 0, 1, func() ([]byte, error) { return []byte{'a'}, nil })
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	type result struct {
		lease *Lease[testParsed]
		err   error
	}
	acquired := make(chan result, 1)
	go func() {
		_, lease, acquireErr := manager.Acquire("test:slow-b", 0, 1, func() ([]byte, error) { return []byte{'b'}, nil })
		acquired <- result{lease: lease, err: acquireErr}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow eviction closer did not start")
	}

	statsDone := make(chan Stats, 1)
	go func() { statsDone <- manager.Stats() }()
	select {
	case stats := <-statsDone:
		if stats.Entries != 1 || stats.Ready != 1 || stats.Pinned != 1 || stats.Bytes != 1 {
			t.Fatalf("stats during slow close = %+v", stats)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow eviction closer held cache manager lock")
	}
	probeDone := make(chan error, 1)
	go func() {
		_, _, probeErr := manager.Acquire("test:slow-probe", 0, 1, func() ([]byte, error) { return []byte{'p'}, nil })
		probeDone <- probeErr
	}()
	select {
	case probeErr := <-probeDone:
		if !errors.Is(probeErr, ErrCapacity) {
			t.Fatalf("probe error = %v, want capacity", probeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow eviction closer blocked concurrent Acquire")
	}

	release()
	select {
	case got := <-acquired:
		if got.err != nil {
			t.Fatalf("replacement acquire: %v", got.err)
		}
		got.lease.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("replacement acquire did not finish after closer release")
	}
}

func TestFontParseCacheMultipleEvictionsPreserveExactBudgets(t *testing.T) {
	var closes atomic.Int32
	manager := New(4, 6, func([]byte, int) (*face.Owner[testParsed], error) {
		return face.NewOwner(testParsed{}, func(testParsed) { closes.Add(1) }), nil
	})
	for _, source := range []string{"test:multi-a", "test:multi-b", "test:multi-c"} {
		_, lease, err := manager.Acquire(source, 0, 2, func() ([]byte, error) { return []byte{1, 2}, nil })
		if err != nil {
			t.Fatalf("acquire %s: %v", source, err)
		}
		lease.Close()
	}
	_, replacement, err := manager.Acquire("test:multi-replacement", 0, 5, func() ([]byte, error) { return []byte{1, 2, 3, 4, 5}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if closes.Load() != 3 {
		t.Fatalf("owner closes = %d, want 3", closes.Load())
	}
	if got := manager.Stats(); got.Entries != 1 || got.Ready != 1 || got.Pinned != 1 || got.Bytes != 5 {
		t.Fatalf("post-eviction stats = %+v", got)
	}
	manager.mu.Lock()
	sources := len(manager.sources)
	manager.mu.Unlock()
	if sources != 1 {
		t.Fatalf("retained sources = %d, want 1", sources)
	}
}

func TestFontParseCacheEqualLastUsedEvictsLexicalKey(t *testing.T) {
	manager := newTestManager(2, 3)
	_, zLease, err := manager.Acquire("test:z", 0, 1, func() ([]byte, error) { return []byte{'z'}, nil })
	if err != nil {
		t.Fatal(err)
	}
	_, aLease, err := manager.Acquire("test:a", 0, 1, func() ([]byte, error) { return []byte{'a'}, nil })
	if err != nil {
		t.Fatal(err)
	}
	zLease.Close()
	aLease.Close()
	manager.mu.Lock()
	manager.entries[Key("test:z", 0)].lastUsed = 7
	manager.entries[Key("test:a", 0)].lastUsed = 7
	manager.mu.Unlock()

	_, replacement, err := manager.Acquire("test:m", 0, 1, func() ([]byte, error) { return []byte{'m'}, nil })
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	manager.mu.Lock()
	_, hasA := manager.entries[Key("test:a", 0)]
	_, hasM := manager.entries[Key("test:m", 0)]
	_, hasZ := manager.entries[Key("test:z", 0)]
	manager.mu.Unlock()
	if hasA || !hasM || !hasZ {
		t.Fatalf("equal-age lexical eviction hasA=%t hasM=%t hasZ=%t", hasA, hasM, hasZ)
	}
}

func TestFontParseCachePinnedCapacityRefusal(t *testing.T) {
	manager := newTestManager(1, 10)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
	_, pinned, err := manager.Acquire("test:pinned", 0, 0, func() ([]byte, error) { return make([]byte, 10), nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Acquire("test:other", 0, 0, func() ([]byte, error) { return []byte{1}, nil }); !errors.Is(err, ErrCapacity) {
		t.Fatalf("face-cap error = %v", err)
	}
	if got := manager.Stats(); got.Entries != 1 || got.Bytes != 10 || got.Pinned != 1 {
		t.Fatalf("active entry changed: %+v", got)
	}
	pinned.Close()
}

func TestFontParseCacheOversizedAndOverflow(t *testing.T) {
	manager := newTestManager(2, 8)
	var parses atomic.Int32
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) {
		parses.Add(1)
		return face.NewOwner(testParsed{}, nil), nil
	}
	if _, _, err := manager.Acquire("test:oversized", 0, 0, func() ([]byte, error) { return make([]byte, 9), nil }); !errors.Is(err, ErrCapacity) {
		t.Fatalf("oversized error = %v", err)
	}
	if parses.Load() != 0 {
		t.Fatal("oversized data was parsed")
	}
	if got := manager.Stats(); got.Entries != 0 || got.Bytes != 0 {
		t.Fatalf("oversized reservation leaked: %+v", got)
	}
	overflow := newTestManager(2, math.MaxInt64)
	overflow.bytes = math.MaxInt64
	if _, _, err := overflow.Acquire("test:overflow", 0, 1, func() ([]byte, error) { return []byte{1}, nil }); !errors.Is(err, ErrCapacity) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestFontParseOccursOutsideCacheLock(t *testing.T) {
	manager := newTestManager(1, 100)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) {
		_ = manager.Stats() // Deadlocks if parsing happens while manager.mu is held.
		return face.NewOwner(testParsed{}, nil), nil
	}
	done := make(chan error, 1)
	go func() {
		_, handle, err := manager.Acquire("test:outside-lock", 0, 0, func() ([]byte, error) { return []byte{1}, nil })
		if handle != nil {
			handle.Close()
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
	manager := newTestManager(1, 8)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
	loadStarted := make(chan struct{})
	finishLoad := make(chan struct{})
	waiterWoke := make(chan struct{})
	releaseWaiter := make(chan struct{})
	manager.afterWait = func() {
		close(waiterWoke)
		<-releaseWaiter
	}

	loaderResult := make(chan *Lease[testParsed], 1)
	go func() {
		_, handle, _ := manager.Acquire("test:shared", 0, 1, func() ([]byte, error) {
			close(loadStarted)
			<-finishLoad
			return []byte{1}, nil
		})
		loaderResult <- handle
	}()
	<-loadStarted

	var waiterLoads atomic.Int32
	waiterResult := make(chan *Lease[testParsed], 1)
	go func() {
		_, handle, _ := manager.Acquire("test:shared", 0, 1, func() ([]byte, error) {
			waiterLoads.Add(1)
			return nil, nil
		})
		waiterResult <- handle
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		manager.mu.Lock()
		entry := manager.entries[Key("test:shared", 0)]
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
	loader.Close()
	<-waiterWoke
	if _, _, err := manager.Acquire("test:competitor", 0, 1, func() ([]byte, error) { return []byte{2}, nil }); !errors.Is(err, ErrCapacity) {
		t.Fatalf("competitor error while waiter pin reserved = %v", err)
	}
	close(releaseWaiter)
	waiter := <-waiterResult
	waiter.Close()
	_, competitor, err := manager.Acquire("test:competitor", 0, 1, func() ([]byte, error) { return []byte{2}, nil })
	if err != nil {
		t.Fatalf("competitor after waiter release: %v", err)
	}
	competitor.Close()
}

func TestFontParseCacheUnknownSizeAdmissionIsPessimistic(t *testing.T) {
	manager := newTestManager(2, 8)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
	started := make(chan struct{})
	finish := make(chan struct{})
	firstResult := make(chan *Lease[testParsed], 1)
	go func() {
		_, handle, _ := manager.Acquire("test:unknown-a", 0, 0, func() ([]byte, error) {
			close(started)
			<-finish
			return []byte{1}, nil
		})
		firstResult <- handle
	}()
	<-started
	var secondLoads atomic.Int32
	if _, _, err := manager.Acquire("test:unknown-b", 0, 0, func() ([]byte, error) {
		secondLoads.Add(1)
		return []byte{2}, nil
	}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("second unknown-size error = %v", err)
	}
	if secondLoads.Load() != 0 {
		t.Fatal("rejected unknown-size load callback ran")
	}
	close(finish)
	(<-firstResult).Close()
}

func TestFontParseCacheSharesSourceBlobAcrossIndices(t *testing.T) {
	manager := newTestManager(2, 64)
	blob := make([]byte, 7)
	var loads, parses atomic.Int32
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) {
		parses.Add(1)
		return face.NewOwner(testParsed{}, nil), nil
	}
	load := func() ([]byte, error) { loads.Add(1); return blob, nil }
	_, zero, err := manager.Acquire("test:collection", 0, int64(len(blob)), load)
	if err != nil {
		t.Fatal(err)
	}
	_, one, err := manager.Acquire("test:collection", 1, int64(len(blob)), load)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Stats(); loads.Load() != 1 || parses.Load() != 2 || got.Bytes != int64(len(blob)) || got.Entries != 2 || got.Pinned != 2 {
		t.Fatalf("loads=%d parses=%d stats=%+v", loads.Load(), parses.Load(), got)
	}
	zero.Close()
	if got := manager.Stats(); got.Pinned != 1 || got.Bytes != int64(len(blob)) {
		t.Fatalf("independent release stats=%+v", got)
	}
	one.Close()

	other := make([]byte, 5)
	_, replacement, err := manager.Acquire("test:replacement", 0, int64(len(other)), func() ([]byte, error) { return other, nil })
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	if got := manager.Stats(); got.Entries != 2 || got.Bytes != int64(len(blob)+len(other)) {
		t.Fatalf("one-face eviction stats=%+v", got)
	}
	final := make([]byte, 6)
	_, last, err := manager.Acquire("test:final", 0, int64(len(final)), func() ([]byte, error) { return final, nil })
	if err != nil {
		t.Fatal(err)
	}
	last.Close()
	if got := manager.Stats(); got.Entries != 2 || got.Bytes != int64(len(other)+len(final)) {
		t.Fatalf("final source eviction stats=%+v", got)
	}
}

func TestFontParseCacheConcurrentDifferentIndicesShareLoad(t *testing.T) {
	manager := newTestManager(2, 16)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
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
		handle *Lease[testParsed]
		err    error
	}
	results := make(chan result, 2)
	go func() { _, handle, err := manager.Acquire("test:ttc", 0, 8, load); results <- result{handle, err} }()
	<-started
	go func() { _, handle, err := manager.Acquire("test:ttc", 1, 8, load); results <- result{handle, err} }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if manager.Stats().Entries == 2 {
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
		result.handle.Close()
	}
	if got := manager.Stats(); loads.Load() != 1 || got.Entries != 2 || got.Bytes != 8 {
		t.Fatalf("loads=%d stats=%+v", loads.Load(), got)
	}
}

func TestFontParseCacheSharedLoadFailureWakesIndicesAndRetries(t *testing.T) {
	manager := newTestManager(2, 16)
	manager.parse = func([]byte, int) (*face.Owner[testParsed], error) { return face.NewOwner(testParsed{}, nil), nil }
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
	go func() { _, _, err := manager.Acquire("test:failed-ttc", 0, 8, load); errs <- err }()
	<-started
	go func() { _, _, err := manager.Acquire("test:failed-ttc", 1, 8, load); errs <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if manager.Stats().Entries == 2 {
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
	if got := manager.Stats(); got.Entries != 0 || got.Bytes != 0 {
		t.Fatalf("failed source leaked: %+v", got)
	}
	_, handle, err := manager.Acquire("test:failed-ttc", 1, 3, load)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	handle.Close()
	if loads.Load() != 2 {
		t.Fatalf("loads=%d, want 2", loads.Load())
	}
}

func TestReadFontFileBoundedRejectsGrowthAndOversize(t *testing.T) {
	if _, err := ReadFileBounded(bytes.NewReader(make([]byte, 7)), 5, 8); !errors.Is(err, ErrFileGrew) {
		t.Fatalf("growth error = %v", err)
	}
	if _, err := ReadFileBounded(bytes.NewReader(make([]byte, 9)), 8, 8); !errors.Is(err, ErrCapacity) {
		t.Fatalf("oversize error = %v", err)
	}
}
