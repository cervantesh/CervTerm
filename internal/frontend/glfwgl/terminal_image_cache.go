//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"cervterm/internal/frontend/gpu"
	"cervterm/internal/termimage"
)

const terminalImageMaxRetries = 3

var terminalImageRetryDelays = [...]time.Duration{
	100 * time.Millisecond,
	500 * time.Millisecond,
	2 * time.Second,
}

type terminalImageCacheLimits struct {
	Entries uint64
	Bytes   uint64
}

func defaultTerminalImageCacheLimits() terminalImageCacheLimits {
	return terminalImageCacheLimits{
		Entries: termimage.HardGPUEntriesPerContext,
		Bytes:   termimage.HardGPUBytesPerContext,
	}
}

func validateTerminalImageCacheLimits(limits terminalImageCacheLimits) (terminalImageCacheLimits, error) {
	if limits.Entries == 0 || limits.Entries > termimage.HardGPUEntriesPerContext ||
		limits.Bytes == 0 || limits.Bytes > termimage.HardGPUBytesPerContext {
		return terminalImageCacheLimits{}, fmt.Errorf("terminal image cache limits exceed hard caps")
	}
	return limits, nil
}

type terminalImageAcquire func(gpu.ImageTextureKey) (termimage.DetachedResource, bool)

type terminalImageCacheEntry struct {
	texture  gpu.ImageTexture
	bytes    uint64
	lastUsed uint64
	pinned   bool
}

type terminalImageRetryIdentity struct {
	paneObject uint64
	image      termimage.ImageID
}

type terminalImageRetryState struct {
	generation termimage.ResourceGeneration
	bytes      uint64
	retries    uint8
	deadline   time.Time
	pending    bool
	active     bool
	exhausted  bool
}

type terminalImageCacheStats struct {
	Entries, Bytes, Pins uint64
	Hits, Misses         uint64
	Acquisitions         uint64
	UploadAttempts       uint64
	UploadFailures       uint64
	RetryAttempts        uint64
	RetryExhausted       uint64
	Evictions            uint64
	CloseFailures        uint64
	OmittedEntryCap      uint64
	OmittedByteCap       uint64
	OmittedUnavailable   uint64
	OmittedUploadFailure uint64
}

type terminalImageFrameResult struct {
	Prefix               uint64
	Ready                uint64
	Omitted              uint64
	OmittedEntryCap      uint64
	OmittedByteCap       uint64
	OmittedUnavailable   uint64
	OmittedUploadFailure uint64
	Err                  error
}

type terminalImageCache struct {
	renderer gpu.TerminalImageRenderer
	acquire  terminalImageAcquire
	limits   terminalImageCacheLimits
	entries  map[gpu.ImageTextureKey]*terminalImageCacheEntry
	retries  map[terminalImageRetryIdentity]*terminalImageRetryState
	pins     []gpu.ImageTextureKey
	bytes    uint64
	clock    uint64
	stats    terminalImageCacheStats
	closed   bool
}

func newTerminalImageCache(renderer gpu.TerminalImageRenderer, acquire terminalImageAcquire, limits terminalImageCacheLimits) (*terminalImageCache, error) {
	validated, err := validateTerminalImageCacheLimits(limits)
	if err != nil {
		return nil, err
	}
	if renderer == nil || acquire == nil {
		return nil, fmt.Errorf("terminal image cache requires renderer and detached acquisition")
	}
	return &terminalImageCache{
		renderer: renderer,
		acquire:  acquire,
		limits:   validated,
		entries:  make(map[gpu.ImageTextureKey]*terminalImageCacheEntry),
		retries:  make(map[terminalImageRetryIdentity]*terminalImageRetryState),
		pins:     make([]gpu.ImageTextureKey, 0, validated.Entries),
	}, nil
}

func (c *terminalImageCache) beginFrame(now time.Time, visible []gpu.ImageTextureKey) terminalImageFrameResult {
	var result terminalImageFrameResult
	if c == nil || c.closed {
		return result
	}
	c.releasePins()
	for _, retry := range c.retries {
		retry.active = false
	}
	var selectedBytes uint64
	for index, key := range visible {
		if result.Prefix == c.limits.Entries {
			omitted := uint64(len(visible) - index)
			result.Omitted += omitted
			result.OmittedEntryCap += omitted
			c.stats.OmittedEntryCap += omitted
			break
		}

		entry := c.entries[key]
		var resource termimage.DetachedResource
		var resourceReady bool
		var bytes uint64
		var retry *terminalImageRetryState
		var retryBlocked bool
		if entry != nil {
			bytes = entry.bytes
		} else {
			c.stats.Misses++
			retry, retryBlocked = c.retryForVisibleGeneration(key)
			if retry != nil && retry.bytes != 0 {
				bytes = retry.bytes
			} else {
				resource, resourceReady = c.acquireResource(key)
				if resourceReady {
					bytes, resourceReady = terminalImageResourceBytes(key, resource)
				}
			}
		}

		if entry == nil && bytes == 0 {
			result.Prefix++
			result.Omitted++
			result.OmittedUnavailable++
			c.stats.OmittedUnavailable++
			continue
		}
		if bytes > c.limits.Bytes-selectedBytes {
			omitted := uint64(len(visible) - index)
			result.Omitted += omitted
			result.OmittedByteCap += omitted
			c.stats.OmittedByteCap += omitted
			break
		}
		selectedBytes += bytes
		result.Prefix++

		if entry != nil {
			c.stats.Hits++
			c.pin(key, entry)
			result.Ready++
			continue
		}
		if retryBlocked {
			result.Omitted++
			result.OmittedUploadFailure++
			c.stats.OmittedUploadFailure++
			continue
		}
		if retry != nil {
			retry.active = true
			if retry.exhausted {
				result.Omitted++
				result.OmittedUploadFailure++
				c.stats.OmittedUploadFailure++
				continue
			}
			if retry.pending && now.Before(retry.deadline) {
				result.Omitted++
				result.OmittedUploadFailure++
				c.stats.OmittedUploadFailure++
				continue
			}
			if retry.pending {
				retry.pending = false
				retry.deadline = time.Time{}
				retry.retries++
				c.stats.RetryAttempts++
			}
		}
		if !resourceReady {
			resource, resourceReady = c.acquireResource(key)
			if resourceReady {
				var currentBytes uint64
				currentBytes, resourceReady = terminalImageResourceBytes(key, resource)
				resourceReady = resourceReady && currentBytes == bytes
			}
		}
		if !resourceReady {
			if retry != nil {
				retry.pending, retry.exhausted = false, true
				retry.deadline = time.Time{}
			}
			result.Omitted++
			result.OmittedUnavailable++
			c.stats.OmittedUnavailable++
			continue
		}
		if err := c.makeRoom(bytes); err != nil {
			result.Err = errors.Join(result.Err, err)
			result.Omitted++
			result.OmittedUnavailable++
			c.stats.OmittedUnavailable++
			continue
		}

		c.stats.UploadAttempts++
		texture, err := c.renderer.PrepareTerminalImage(key, resource)
		if err != nil || texture == nil {
			if texture != nil {
				if closeErr := texture.Close(); closeErr != nil {
					c.stats.CloseFailures++
					result.Err = errors.Join(result.Err, closeErr)
				}
			}
			c.stats.UploadFailures++
			c.recordUploadFailure(key, bytes, now, retry)
			result.Omitted++
			result.OmittedUploadFailure++
			c.stats.OmittedUploadFailure++
			continue
		}
		if retry != nil {
			retry.bytes = bytes
			retry.pending = false
			retry.deadline = time.Time{}
			retry.exhausted = false
		}
		entry = &terminalImageCacheEntry{texture: texture, bytes: bytes}
		c.entries[key] = entry
		c.bytes += bytes
		c.pin(key, entry)
		result.Ready++
	}
	return result
}

func (c *terminalImageCache) acquireResource(key gpu.ImageTextureKey) (termimage.DetachedResource, bool) {
	c.stats.Acquisitions++
	return c.acquire(key)
}

func terminalImageResourceBytes(key gpu.ImageTextureKey, resource termimage.DetachedResource) (uint64, bool) {
	if key.PaneObject == 0 || key.Resource.Image == 0 || key.Resource.Generation == 0 || resource.Ref != key.Resource {
		return 0, false
	}
	stride, bytes, err := termimage.CheckedRGBABytes(resource.Width, resource.Height)
	if err != nil || resource.Stride != stride || uint64(len(resource.RGBA)) != bytes {
		return 0, false
	}
	return bytes, true
}

func (c *terminalImageCache) retryForVisibleGeneration(key gpu.ImageTextureKey) (*terminalImageRetryState, bool) {
	identity := terminalImageRetryIdentity{paneObject: key.PaneObject, image: key.Resource.Image}
	retry := c.retries[identity]
	if retry != nil {
		if key.Resource.Generation < retry.generation {
			return retry, true
		}
		if key.Resource.Generation > retry.generation {
			*retry = terminalImageRetryState{generation: key.Resource.Generation}
		}
		return retry, false
	}
	if uint64(len(c.retries)) >= c.limits.Entries {
		return nil, true
	}
	return nil, false
}

func (c *terminalImageCache) recordUploadFailure(key gpu.ImageTextureKey, bytes uint64, now time.Time, retry *terminalImageRetryState) {
	if retry == nil {
		identity := terminalImageRetryIdentity{paneObject: key.PaneObject, image: key.Resource.Image}
		retry = &terminalImageRetryState{generation: key.Resource.Generation}
		c.retries[identity] = retry
	}
	retry.bytes = bytes
	retry.active = true
	if retry.retries >= terminalImageMaxRetries {
		retry.pending = false
		retry.deadline = time.Time{}
		retry.exhausted = true
		c.stats.RetryExhausted++
		return
	}
	retry.pending = true
	retry.deadline = now.Add(terminalImageRetryDelays[retry.retries])
}

func (c *terminalImageCache) makeRoom(bytes uint64) error {
	for uint64(len(c.entries))+1 > c.limits.Entries || c.bytes+bytes > c.limits.Bytes {
		key, entry, ok := c.lruUnpinned()
		if !ok {
			return fmt.Errorf("terminal image cache has no unpinned eviction candidate")
		}
		if err := entry.texture.Close(); err != nil {
			c.stats.CloseFailures++
			return err
		}
		delete(c.entries, key)
		c.bytes -= entry.bytes
		c.stats.Evictions++
	}
	return nil
}

func (c *terminalImageCache) lruUnpinned() (gpu.ImageTextureKey, *terminalImageCacheEntry, bool) {
	var selectedKey gpu.ImageTextureKey
	var selected *terminalImageCacheEntry
	for key, entry := range c.entries {
		if entry.pinned {
			continue
		}
		if selected == nil || entry.lastUsed < selected.lastUsed ||
			(entry.lastUsed == selected.lastUsed && imageTextureKeyLess(key, selectedKey)) {
			selectedKey, selected = key, entry
		}
	}
	return selectedKey, selected, selected != nil
}

func imageTextureKeyLess(left, right gpu.ImageTextureKey) bool {
	if left.PaneObject != right.PaneObject {
		return left.PaneObject < right.PaneObject
	}
	if left.Resource.Image != right.Resource.Image {
		return left.Resource.Image < right.Resource.Image
	}
	return left.Resource.Generation < right.Resource.Generation
}

func (c *terminalImageCache) pin(key gpu.ImageTextureKey, entry *terminalImageCacheEntry) {
	c.clock++
	entry.lastUsed = c.clock
	if entry.pinned {
		return
	}
	entry.pinned = true
	c.pins = append(c.pins, key)
}

func (c *terminalImageCache) releasePins() {
	for _, key := range c.pins {
		if entry := c.entries[key]; entry != nil {
			entry.pinned = false
		}
	}
	c.pins = c.pins[:0]
}

func (c *terminalImageCache) texture(key gpu.ImageTextureKey) (gpu.ImageTexture, bool) {
	if c == nil || c.closed {
		return nil, false
	}
	entry := c.entries[key]
	return func() (gpu.ImageTexture, bool) {
		if entry == nil || entry.texture == nil {
			return nil, false
		}
		return entry.texture, true
	}()
}

func (c *terminalImageCache) nextRetryDeadline() (time.Time, bool) {
	if c == nil || c.closed {
		return time.Time{}, false
	}
	var earliest time.Time
	for _, retry := range c.retries {
		if !retry.active || !retry.pending || retry.deadline.IsZero() {
			continue
		}
		if earliest.IsZero() || retry.deadline.Before(earliest) {
			earliest = retry.deadline
		}
	}
	return earliest, !earliest.IsZero()
}

func (c *terminalImageCache) retryDue(now time.Time) bool {
	deadline, ok := c.nextRetryDeadline()
	return ok && !now.Before(deadline)
}

func (c *terminalImageCache) snapshotStats() terminalImageCacheStats {
	if c == nil {
		return terminalImageCacheStats{}
	}
	stats := c.stats
	stats.Entries = uint64(len(c.entries))
	stats.Bytes = c.bytes
	stats.Pins = uint64(len(c.pins))
	return stats
}

func (c *terminalImageCache) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	c.releasePins()
	keys := make([]gpu.ImageTextureKey, 0, len(c.entries))
	for key := range c.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return imageTextureKeyLess(keys[i], keys[j]) })
	var joined error
	for _, key := range keys {
		if err := c.entries[key].texture.Close(); err != nil {
			c.stats.CloseFailures++
			joined = errors.Join(joined, err)
		}
		delete(c.entries, key)
	}
	c.bytes = 0
	clear(c.retries)
	return joined
}

func (a *App) closeTerminalImageCache() error {
	if a == nil || a.terminalImageCache == nil {
		return nil
	}
	cache := a.terminalImageCache
	a.terminalImageCache = nil
	return cache.Close()
}

func appendTerminalImageCacheResource(bundle *nativeProjectionBundle, app *App) {
	if bundle == nil || app == nil {
		return
	}
	bundle.resources = append(bundle.resources, projectionResourceFunc(app.closeTerminalImageCache))
}
