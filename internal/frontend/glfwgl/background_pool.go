//go:build glfw

package glfwgl

import (
	"fmt"
	"math"
	"sync"

	backgroundcore "cervterm/internal/background"
	"cervterm/internal/config"
)

type backgroundResourcePool struct {
	mu            sync.Mutex
	cache         *backgroundcore.Cache
	composedBytes uint64
	closed        bool
}

func newBackgroundResourcePool() *backgroundResourcePool {
	return &backgroundResourcePool{cache: backgroundcore.NewCache()}
}

func (a *App) ensureBackgroundResourcePool() *backgroundResourcePool {
	if a.configReloadAsync.resourcePool == nil {
		a.configReloadAsync.resourcePool = newBackgroundResourcePool()
	}
	return a.configReloadAsync.resourcePool
}

func (p *backgroundResourcePool) resolveLocked(imageIndex int, path string, variant backgroundcore.CacheVariant, budget *backgroundcore.Budget) (*backgroundcore.Lease, *backgroundcore.Source, error) {
	canonical, digest, err := backgroundcore.FileDigest(imageIndex, path)
	if err != nil {
		return nil, nil, err
	}
	lease, hit, err := p.cache.AcquireVariant(canonical, digest, variant)
	if err != nil {
		return nil, nil, err
	}
	if hit {
		if err := budget.PinSource(imageIndex, lease.Source()); err != nil {
			_ = p.cache.Release(lease)
			return nil, nil, err
		}
		return lease, lease.Source(), nil
	}
	// A miss may decode up to the remaining candidate budget. Evict every
	// unrelated unpinned source first so retained cache plus the new allocation
	// can never cross the aggregate CPU ceiling.
	remaining := backgroundcore.MaxAggregateCPUBytes - budget.CPUBytes()
	if err := p.cache.TrimFor(remaining); err != nil {
		return nil, nil, fmt.Errorf("background cache residency: %w", err)
	}
	source, err := backgroundcore.DecodeFile(imageIndex, canonical, budget)
	if err != nil {
		return nil, nil, err
	}
	lease, err = p.cache.InsertVariant(canonical, source, variant)
	if err != nil {
		_ = source.Close()
		return nil, nil, err
	}
	return lease, source, nil
}

func (p *backgroundResourcePool) reserveCompositionLocked(bytes uint64) error {
	if p.closed {
		return fmt.Errorf("background resource pool is closed")
	}
	if bytes > backgroundcore.MaxAggregateCPUBytes-p.composedBytes {
		return fmt.Errorf("background composition residency: exceeds limit")
	}
	if err := p.cache.TrimFor(p.composedBytes + bytes); err != nil {
		return fmt.Errorf("background composition residency: %w", err)
	}
	p.composedBytes += bytes
	return nil
}

func (p *backgroundResourcePool) releaseComposition(bytes uint64) {
	if p == nil || bytes == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if bytes <= p.composedBytes {
		p.composedBytes -= bytes
	} else {
		p.composedBytes = 0
	}
}

func (p *backgroundResourcePool) close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.composedBytes != 0 {
		return fmt.Errorf("background resource pool has prepared compositions")
	}
	if err := p.cache.Close(); err != nil {
		return err
	}
	p.closed = true
	return nil
}

func (p *backgroundResourcePool) isClosed() bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func backgroundCacheVariant(spec config.BackgroundLayer, width, height int, dpi float64) backgroundcore.CacheVariant {
	return backgroundcore.CacheVariant{Fit: spec.Fit, Horizontal: spec.HorizontalAlign, Vertical: spec.VerticalAlign, Width: width, Height: height, DPIBits: math.Float64bits(dpi)}
}
