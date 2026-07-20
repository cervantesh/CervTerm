// Package notificationpolicy gates untrusted terminal notification metadata
// before a frontend may invoke a native adapter.
package notificationpolicy

import "time"

type Config struct {
	Enabled     bool
	Unfocused   bool
	MinInterval time.Duration
}

type Clock func() time.Time

type Gate struct {
	now  Clock
	last time.Time
}

func NewGate(clock Clock) *Gate {
	if clock == nil {
		clock = time.Now
	}
	return &Gate{now: clock}
}

// Allow requires explicit consent, a fresh (never deferred) request, the focus
// rule, and the per-window rate limit. Payload values are intentionally absent.
func (g *Gate) Allow(config Config, focused, fresh bool) bool {
	if !config.Enabled || !fresh || (config.Unfocused && focused) {
		return false
	}
	now := g.now()
	if !g.last.IsZero() && config.MinInterval > 0 && now.Sub(g.last) < config.MinInterval {
		return false
	}
	g.last = now
	return true
}

func (g *Gate) Reset() { g.last = time.Time{} }
