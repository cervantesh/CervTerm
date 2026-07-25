// Package face owns parsed face data and any native handles attached to it.
// It is private to the fontglyph subsystem so public subsystem packages can
// share ownership without importing the compatibility facade.
package face

import "sync"

// Owner keeps one size-independent parsed face value and closes its attached
// resources exactly once. Cache leases, rather than callers, control how long
// an Owner remains reachable.
type Owner[T any] struct {
	value     T
	closeOnce sync.Once
	close     func(T)
}

// NewOwner adopts value and its optional resource closer.
func NewOwner[T any](value T, close func(T)) *Owner[T] {
	return &Owner[T]{value: value, close: close}
}

// Value returns the immutable parsed face value owned by o.
func (o *Owner[T]) Value() T {
	if o == nil {
		var zero T
		return zero
	}
	return o.value
}

// Close releases resources attached to the parsed face exactly once.
func (o *Owner[T]) Close() {
	if o == nil {
		return
	}
	o.closeOnce.Do(func() {
		if o.close != nil {
			o.close(o.value)
		}
	})
}
