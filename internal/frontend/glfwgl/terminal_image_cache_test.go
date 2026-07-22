//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/termimage"
)

type fakeCacheTexture struct {
	id     int
	key    gpu.ImageTextureKey
	closed int
	log    *[]string
}

func cacheTestTime(offset time.Duration) time.Time { return time.Unix(0, 0).Add(offset) }

func (t *fakeCacheTexture) Close() error {
	if t.closed != 0 {
		return nil
	}
	t.closed++
	if t.log != nil {
		*t.log = append(*t.log, fmt.Sprintf("texture-close:%d", t.id))
	}
	return nil
}

type fakeCacheRenderer struct {
	attempts   []gpu.ImageTextureKey
	failures   map[gpu.ImageTextureKey]int
	textures   []*fakeCacheTexture
	nextID     int
	textureLog *[]string
}

func (r *fakeCacheRenderer) PrepareTerminalImage(key gpu.ImageTextureKey, _ termimage.DetachedResource) (gpu.ImageTexture, error) {
	r.attempts = append(r.attempts, key)
	if r.failures[key] > 0 {
		r.failures[key]--
		return nil, errors.New("injected upload failure")
	}
	r.nextID++
	texture := &fakeCacheTexture{id: r.nextID, key: key, log: r.textureLog}
	r.textures = append(r.textures, texture)
	return texture, nil
}

func (*fakeCacheRenderer) DrawTerminalImage(gpu.ImageTexture, gpu.ImageRect, gpu.ImageRect, float32) error {
	return nil
}

type fakeCacheAcquire struct {
	resources map[gpu.ImageTextureKey]termimage.DetachedResource
	calls     []gpu.ImageTextureKey
}

func (a *fakeCacheAcquire) acquire(key gpu.ImageTextureKey) (termimage.DetachedResource, bool) {
	a.calls = append(a.calls, key)
	resource, ok := a.resources[key]
	if !ok {
		return termimage.DetachedResource{}, false
	}
	resource.RGBA = append([]byte(nil), resource.RGBA...)
	return resource, true
}

func cacheKey(pane uint64, image uint32, generation uint64) gpu.ImageTextureKey {
	return gpu.ImageTextureKey{
		PaneObject: pane,
		Resource: termimage.ResourceRef{
			Image:      termimage.ImageID(image),
			Generation: termimage.ResourceGeneration(generation),
		},
	}
}

func cacheResource(key gpu.ImageTextureKey, bytes uint64) termimage.DetachedResource {
	if bytes == 0 || bytes%4 != 0 {
		panic("test resource byte count must be a positive RGBA pixel multiple")
	}
	return termimage.DetachedResource{
		Ref:    key.Resource,
		Width:  uint32(bytes / 4),
		Height: 1,
		Stride: uint32(bytes),
		RGBA:   make([]byte, bytes),
	}
}

func newFakeTerminalImageCache(t testing.TB, limits terminalImageCacheLimits, resources map[gpu.ImageTextureKey]termimage.DetachedResource) (*terminalImageCache, *fakeCacheRenderer, *fakeCacheAcquire) {
	t.Helper()
	renderer := &fakeCacheRenderer{failures: make(map[gpu.ImageTextureKey]int)}
	acquire := &fakeCacheAcquire{resources: resources}
	cache, err := newTerminalImageCache(renderer, acquire.acquire, limits)
	if err != nil {
		t.Fatal(err)
	}
	return cache, renderer, acquire
}

func TestTerminalImageCacheLimitsAreHardAndLowerOnly(t *testing.T) {
	defaults := defaultTerminalImageCacheLimits()
	if defaults.Entries != 512 || defaults.Bytes != 256*1024*1024 {
		t.Fatalf("hard defaults=%#v", defaults)
	}
	if got, err := validateTerminalImageCacheLimits(terminalImageCacheLimits{Entries: 1, Bytes: 4}); err != nil || got.Entries != 1 || got.Bytes != 4 {
		t.Fatalf("lower limits=%#v err=%v", got, err)
	}
	invalid := []terminalImageCacheLimits{
		{},
		{Entries: 1, Bytes: 0},
		{Entries: termimage.HardGPUEntriesPerContext + 1, Bytes: 4},
		{Entries: 1, Bytes: termimage.HardGPUBytesPerContext + 1},
	}
	for _, limits := range invalid {
		if _, err := validateTerminalImageCacheLimits(limits); err == nil {
			t.Fatalf("invalid limits accepted: %#v", limits)
		}
	}
	if cache, err := newTerminalImageCache(nil, nil, terminalImageCacheLimits{Entries: 1, Bytes: 4}); err == nil || cache != nil {
		t.Fatal("nil cache dependencies accepted")
	}
}

func TestTerminalImageCacheSelectsLongestOrderedPrefixWithinBothCaps(t *testing.T) {
	t.Run("entry cap", func(t *testing.T) {
		keys := []gpu.ImageTextureKey{cacheKey(1, 1, 1), cacheKey(1, 2, 2), cacheKey(1, 3, 3)}
		resources := make(map[gpu.ImageTextureKey]termimage.DetachedResource)
		for _, key := range keys {
			resources[key] = cacheResource(key, 4)
		}
		cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 32}, resources)
		result := cache.beginFrame(time.Time{}, keys)
		if result.Prefix != 2 || result.Ready != 2 || result.Omitted != 1 || result.OmittedEntryCap != 1 || result.OmittedByteCap != 0 {
			t.Fatalf("result=%#v", result)
		}
		if len(acquire.calls) != 2 || len(renderer.attempts) != 2 {
			t.Fatalf("acquires/uploads=%d/%d", len(acquire.calls), len(renderer.attempts))
		}
		stats := cache.snapshotStats()
		if stats.Entries != 2 || stats.Bytes != 8 || stats.Pins != 2 || stats.OmittedEntryCap != 1 {
			t.Fatalf("stats=%#v", stats)
		}
	})

	t.Run("byte cap", func(t *testing.T) {
		keys := []gpu.ImageTextureKey{cacheKey(1, 1, 1), cacheKey(1, 2, 2), cacheKey(1, 3, 3)}
		resources := map[gpu.ImageTextureKey]termimage.DetachedResource{
			keys[0]: cacheResource(keys[0], 4),
			keys[1]: cacheResource(keys[1], 8),
			keys[2]: cacheResource(keys[2], 4),
		}
		cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 3, Bytes: 8}, resources)
		result := cache.beginFrame(time.Time{}, keys)
		if result.Prefix != 1 || result.Ready != 1 || result.Omitted != 2 || result.OmittedByteCap != 2 || result.OmittedEntryCap != 0 {
			t.Fatalf("result=%#v", result)
		}
		if !reflect.DeepEqual(acquire.calls, keys[:2]) || !reflect.DeepEqual(renderer.attempts, keys[:1]) {
			t.Fatalf("acquires=%v uploads=%v", acquire.calls, renderer.attempts)
		}
	})
}

func TestTerminalImageCacheReleasesPriorPinsWithoutUnderflowAndEvictsOnlyUnpinned(t *testing.T) {
	keys := []gpu.ImageTextureKey{cacheKey(1, 1, 1), cacheKey(1, 2, 2), cacheKey(1, 3, 3), cacheKey(1, 4, 4)}
	resources := make(map[gpu.ImageTextureKey]termimage.DetachedResource)
	for _, key := range keys {
		resources[key] = cacheResource(key, 4)
	}
	cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 8}, resources)
	if got := cache.beginFrame(time.Time{}, keys[:2]); got.Ready != 2 {
		t.Fatal(got)
	}
	if got := cache.beginFrame(cacheTestTime(time.Millisecond), keys[2:]); got.Ready != 2 {
		t.Fatal(got)
	}
	if renderer.textures[0].closed != 1 || renderer.textures[1].closed != 1 || renderer.textures[2].closed != 0 || renderer.textures[3].closed != 0 {
		t.Fatalf("texture closes=%d,%d,%d,%d", renderer.textures[0].closed, renderer.textures[1].closed, renderer.textures[2].closed, renderer.textures[3].closed)
	}
	stats := cache.snapshotStats()
	if stats.Entries != 2 || stats.Bytes != 8 || stats.Pins != 2 || stats.Evictions != 2 {
		t.Fatalf("stats=%#v", stats)
	}
	cache.beginFrame(cacheTestTime(2*time.Millisecond), nil)
	cache.beginFrame(cacheTestTime(3*time.Millisecond), nil)
	if stats = cache.snapshotStats(); stats.Pins != 0 || stats.Entries != 2 || stats.Bytes != 8 {
		t.Fatalf("empty-frame underflow/residency=%#v", stats)
	}
}

func TestTerminalImageCacheLRUIsDeterministicAndHitsNeverAcquire(t *testing.T) {
	a, b, cKey := cacheKey(2, 1, 1), cacheKey(2, 2, 2), cacheKey(2, 3, 3)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{
		a: cacheResource(a, 4), b: cacheResource(b, 4), cKey: cacheResource(cKey, 4),
	}
	cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 8}, resources)
	cache.beginFrame(time.Time{}, []gpu.ImageTextureKey{a, b})
	cache.beginFrame(cacheTestTime(time.Millisecond), []gpu.ImageTextureKey{a})
	cache.beginFrame(cacheTestTime(2*time.Millisecond), []gpu.ImageTextureKey{cKey})
	if renderer.textures[0].closed != 0 || renderer.textures[1].closed != 1 {
		t.Fatalf("LRU closed A/B=%d/%d", renderer.textures[0].closed, renderer.textures[1].closed)
	}
	if len(acquire.calls) != 3 || acquire.calls[0] != a || acquire.calls[1] != b || acquire.calls[2] != cKey {
		t.Fatalf("hit acquired detached pixels: %v", acquire.calls)
	}
	if texture, ok := cache.texture(a); !ok || texture != renderer.textures[0] {
		t.Fatal("recent hit was evicted")
	}

	cache.beginFrame(cacheTestTime(3*time.Millisecond), nil)
	cache.entries[a].lastUsed = 7
	cache.entries[cKey].lastUsed = 7
	d := cacheKey(2, 4, 4)
	acquire.resources[d] = cacheResource(d, 4)
	cache.beginFrame(cacheTestTime(4*time.Millisecond), []gpu.ImageTextureKey{d})
	if _, ok := cache.texture(a); ok {
		t.Fatal("equal-age deterministic key order did not evict the lower key")
	}
}

func TestTerminalImageCacheRetriesSameGenerationOnFixedScheduleAndThenStops(t *testing.T) {
	now := time.Unix(100, 0)
	key := cacheKey(3, 9, 11)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{key: cacheResource(key, 4)}
	cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 4, Bytes: 16}, resources)
	renderer.failures[key] = 4

	cache.beginFrame(now, []gpu.ImageTextureKey{key})
	assertRetryDeadline(t, cache, now.Add(100*time.Millisecond))
	cache.beginFrame(now.Add(99*time.Millisecond), []gpu.ImageTextureKey{key})
	if len(renderer.attempts) != 1 || len(acquire.calls) != 1 {
		t.Fatal("retry ran before deadline")
	}
	cache.beginFrame(now.Add(100*time.Millisecond), []gpu.ImageTextureKey{key})
	assertRetryDeadline(t, cache, now.Add(600*time.Millisecond))
	cache.beginFrame(now.Add(599*time.Millisecond), []gpu.ImageTextureKey{key})
	if len(renderer.attempts) != 2 {
		t.Fatal("second retry ran before deadline")
	}
	cache.beginFrame(now.Add(600*time.Millisecond), []gpu.ImageTextureKey{key})
	assertRetryDeadline(t, cache, now.Add(2600*time.Millisecond))
	cache.beginFrame(now.Add(2600*time.Millisecond), []gpu.ImageTextureKey{key})
	if _, ok := cache.nextRetryDeadline(); ok {
		t.Fatal("exhausted generation retained a retry deadline")
	}
	cache.beginFrame(now.Add(5*time.Second), []gpu.ImageTextureKey{key})
	if len(renderer.attempts) != 4 || len(acquire.calls) != 4 {
		t.Fatalf("exhausted generation spun: uploads/acquires=%d/%d", len(renderer.attempts), len(acquire.calls))
	}
	stats := cache.snapshotStats()
	if stats.RetryAttempts != 3 || stats.UploadFailures != 4 || stats.RetryExhausted != 1 {
		t.Fatalf("retry stats=%#v", stats)
	}

	newGeneration := cacheKey(3, 9, 12)
	acquire.resources[newGeneration] = cacheResource(newGeneration, 4)
	renderer.failures[newGeneration] = 1
	cache.beginFrame(now.Add(6*time.Second), []gpu.ImageTextureKey{newGeneration})
	if len(renderer.attempts) != 5 || renderer.attempts[4] != newGeneration {
		t.Fatal("new generation did not reset retry eligibility")
	}
	assertRetryDeadline(t, cache, now.Add(6100*time.Millisecond))

	otherCache, otherRenderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, resources)
	otherRenderer.failures[key] = 1
	otherCache.beginFrame(now.Add(7*time.Second), []gpu.ImageTextureKey{key})
	if len(otherRenderer.attempts) != 1 {
		t.Fatal("new context inherited exhausted retry state")
	}
}

func assertRetryDeadline(t *testing.T, cache *terminalImageCache, want time.Time) {
	t.Helper()
	got, ok := cache.nextRetryDeadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("retry deadline=%v,%v want=%v", got, ok, want)
	}
}

func TestTerminalImageCacheRetrySaturationDoesNotReattemptUnscheduledUploads(t *testing.T) {
	now := time.Unix(200, 0)
	first, second := cacheKey(5, 1, 1), cacheKey(5, 2, 1)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{first: cacheResource(first, 4), second: cacheResource(second, 4)}
	cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 8}, resources)
	renderer.failures[first] = 1
	renderer.failures[second] = 3
	cache.beginFrame(now, []gpu.ImageTextureKey{first})
	cache.beginFrame(now, []gpu.ImageTextureKey{second})
	cache.beginFrame(now.Add(time.Second), []gpu.ImageTextureKey{second})
	if len(renderer.attempts) != 1 || renderer.attempts[0] != first {
		t.Fatalf("saturated retry table attempted uploads: %v", renderer.attempts)
	}
}

func TestTerminalImageCacheOlderGenerationCannotResetRetryBudget(t *testing.T) {
	now := time.Unix(300, 0)
	newer, older := cacheKey(6, 9, 2), cacheKey(6, 9, 1)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{newer: cacheResource(newer, 4), older: cacheResource(older, 4)}
	cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 8}, resources)
	renderer.failures[newer] = 1
	renderer.failures[older] = 1
	cache.beginFrame(now, []gpu.ImageTextureKey{newer})
	cache.beginFrame(now.Add(time.Second), []gpu.ImageTextureKey{older})
	if len(renderer.attempts) != 1 || renderer.attempts[0] != newer {
		t.Fatalf("older generation reset retry state: %v", renderer.attempts)
	}
}

func TestTerminalImageCacheSuccessfulRetryClearsWakeAndBecomesHit(t *testing.T) {
	now := time.Unix(200, 0)
	key := cacheKey(4, 1, 1)
	cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, map[gpu.ImageTextureKey]termimage.DetachedResource{key: cacheResource(key, 4)})
	renderer.failures[key] = 1
	cache.beginFrame(now, []gpu.ImageTextureKey{key})
	result := cache.beginFrame(now.Add(100*time.Millisecond), []gpu.ImageTextureKey{key})
	if result.Ready != 1 || result.Omitted != 0 || cache.retryDue(now.Add(time.Second)) {
		t.Fatalf("successful retry result=%#v", result)
	}
	if _, ok := cache.nextRetryDeadline(); ok {
		t.Fatal("successful retry retained idle wake")
	}
	cache.beginFrame(now.Add(time.Second), []gpu.ImageTextureKey{key})
	if len(acquire.calls) != 2 || len(renderer.attempts) != 2 || cache.snapshotStats().Hits != 1 {
		t.Fatalf("successful texture was not a hit: acquire/upload/stats=%d/%d/%#v", len(acquire.calls), len(renderer.attempts), cache.snapshotStats())
	}
}

func TestTerminalImageCacheDeadlineIsEarliestPendingVisibleRetryOnly(t *testing.T) {
	now := time.Unix(300, 0)
	first, second := cacheKey(5, 1, 1), cacheKey(5, 2, 2)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{first: cacheResource(first, 4), second: cacheResource(second, 4)}
	cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 8}, resources)
	renderer.failures[first], renderer.failures[second] = 4, 4
	cache.beginFrame(now, []gpu.ImageTextureKey{first})
	cache.beginFrame(now.Add(100*time.Millisecond), []gpu.ImageTextureKey{first, second})
	assertRetryDeadline(t, cache, now.Add(200*time.Millisecond))
	cache.beginFrame(now.Add(101*time.Millisecond), nil)
	if _, ok := cache.nextRetryDeadline(); ok || cache.retryDue(now.Add(time.Hour)) {
		t.Fatal("invisible retries created idle wake")
	}
}

func TestTerminalImageCacheStaleGenerationIsOmittedWithoutUploadOrRetry(t *testing.T) {
	stale, current := cacheKey(6, 1, 1), cacheKey(6, 2, 2)
	cache, renderer, acquire := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 2, Bytes: 8}, map[gpu.ImageTextureKey]termimage.DetachedResource{current: cacheResource(current, 4)})
	result := cache.beginFrame(time.Time{}, []gpu.ImageTextureKey{stale, current})
	if result.Prefix != 2 || result.Ready != 1 || result.OmittedUnavailable != 1 || len(renderer.attempts) != 1 || renderer.attempts[0] != current {
		t.Fatalf("stale result=%#v uploads=%v", result, renderer.attempts)
	}
	if !reflect.DeepEqual(acquire.calls, []gpu.ImageTextureKey{stale, current}) {
		t.Fatalf("acquires=%v", acquire.calls)
	}
	if _, ok := cache.nextRetryDeadline(); ok {
		t.Fatal("stale acquisition scheduled upload retry")
	}
}

func TestTerminalImageCacheIsProjectionLocalAndModelIndependent(t *testing.T) {
	key := cacheKey(77, 3, 9)
	resource := cacheResource(key, 4)
	resources := map[gpu.ImageTextureKey]termimage.DetachedResource{key: resource}
	first, firstRenderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, resources)
	second, secondRenderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, resources)
	first.beginFrame(time.Time{}, []gpu.ImageTextureKey{key})
	second.beginFrame(time.Time{}, []gpu.ImageTextureKey{key})
	firstTexture, firstOK := first.texture(key)
	secondTexture, secondOK := second.texture(key)
	if !firstOK || !secondOK || firstTexture == secondTexture || len(firstRenderer.attempts) != 1 || len(secondRenderer.attempts) != 1 {
		t.Fatal("projection contexts shared texture handles")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if firstRenderer.textures[0].closed != 1 || secondRenderer.textures[0].closed != 0 {
		t.Fatal("closing source context affected destination context")
	}
	if texture, ok := second.texture(key); !ok || texture != secondRenderer.textures[0] {
		t.Fatal("destination texture was not context-local")
	}
}

func TestTerminalImageCacheClosesWithCurrentContextBeforeRendererAcrossProjectionLifecycles(t *testing.T) {
	for _, lifecycle := range []string{"initial", "runtime", "restore"} {
		t.Run(lifecycle, func(t *testing.T) {
			var log []string
			key := cacheKey(8, 1, 1)
			cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 1, Bytes: 4}, map[gpu.ImageTextureKey]termimage.DetachedResource{key: cacheResource(key, 4)})
			renderer.textureLog = &log
			cache.beginFrame(time.Time{}, []gpu.ImageTextureKey{key})
			app := &App{terminalImageCache: cache}
			host := &fakeNativeWindow{id: lifecycle, log: &log}
			bundle := &nativeProjectionBundle{host: host, app: app, handle: func([]termmux.Event) bool { return true }}
			bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
				log = append(log, "renderer-destroy")
				return nil
			}))
			appendTerminalImageCacheResource(bundle, app)

			var err error
			switch lifecycle {
			case "initial":
				controller := newWindowController(processServices{}, fakeNativePump{log: &log})
				if attachErr := controller.attachApp(initialWindowID, host, app, bundle.handle); attachErr != nil {
					t.Fatal(attachErr)
				}
				controller.windows[initialWindowID].bundle = bundle
				if startErr := controller.startLoop(); startErr != nil {
					t.Fatal(startErr)
				}
				err = controller.closeProjection(initialWindowID)
			case "runtime":
				err = closeProjectionBundleWithCurrent(bundle)
			case "restore":
				err = closeRestoreProjectionBundle(bundle)
			}
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"current:" + lifecycle, "texture-close:1", "renderer-destroy", "destroy:" + lifecycle}
			if !reflect.DeepEqual(log, want) {
				t.Fatalf("teardown log=%v want=%v", log, want)
			}
			if app.terminalImageCache != nil {
				t.Fatal("projection retained closed cache")
			}
		})
	}
}

func TestTerminalImageCacheCloseIsIdempotentAndDeterministic(t *testing.T) {
	var log []string
	keys := []gpu.ImageTextureKey{cacheKey(9, 3, 3), cacheKey(9, 1, 1), cacheKey(9, 2, 2)}
	resources := make(map[gpu.ImageTextureKey]termimage.DetachedResource)
	for _, key := range keys {
		resources[key] = cacheResource(key, 4)
	}
	cache, renderer, _ := newFakeTerminalImageCache(t, terminalImageCacheLimits{Entries: 3, Bytes: 12}, resources)
	renderer.textureLog = &log
	cache.beginFrame(time.Time{}, keys)
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"texture-close:2", "texture-close:3", "texture-close:1"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("close order=%v want=%v", log, want)
	}
	if stats := cache.snapshotStats(); stats.Entries != 0 || stats.Bytes != 0 || stats.Pins != 0 {
		t.Fatalf("closed stats=%#v", stats)
	}
}

func BenchmarkTerminalImageCacheFrameHits(b *testing.B) {
	const count = 128
	keys := make([]gpu.ImageTextureKey, count)
	resources := make(map[gpu.ImageTextureKey]termimage.DetachedResource, count)
	for index := range keys {
		keys[index] = cacheKey(10, uint32(index+1), uint64(index+1))
		resources[keys[index]] = cacheResource(keys[index], 4)
	}
	cache, _, _ := newFakeTerminalImageCache(b, terminalImageCacheLimits{Entries: count, Bytes: count * 4}, resources)
	if result := cache.beginFrame(time.Time{}, keys); result.Ready != count {
		b.Fatal(result)
	}
	b.Cleanup(func() { _ = cache.Close() })
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		cache.beginFrame(time.Time{}, keys)
	}
}
