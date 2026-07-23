// Package bellpolicy decides whether a losslessly observed terminal bell may
// produce a throttled frontend sink effect. It has no OS or renderer dependency.
package bellpolicy

import "time"

type Mode string

const (
	Disabled Mode = "disabled"
	Audible  Mode = "audible"
	Visual   Mode = "visual"
	Taskbar  Mode = "taskbar"
)

type Config struct {
	Mode      Mode
	Unfocused bool
	Throttle  time.Duration
	VisualFor time.Duration
}

type Decision struct {
	Mode      Mode
	VisualFor time.Duration
	At        time.Time
}

type Clock func() time.Time

type Gate struct {
	now  Clock
	last map[Mode]time.Time
}

func NewGate(clock Clock) *Gate {
	if clock == nil {
		clock = time.Now
	}
	return &Gate{now: clock, last: make(map[Mode]time.Time)}
}

// Decide returns a sink decision for one already-observed bell. Rejection here
// never affects core counters, mux events, or scripting callbacks.
func (g *Gate) Decide(cfg Config, focused bool) (Decision, bool) {
	if cfg.Mode == Disabled || cfg.Mode == "" || (cfg.Unfocused && focused) {
		return Decision{}, false
	}
	if cfg.Mode != Audible && cfg.Mode != Visual && cfg.Mode != Taskbar {
		return Decision{}, false
	}
	now := g.now()
	if previous, ok := g.last[cfg.Mode]; ok && cfg.Throttle > 0 && now.Sub(previous) < cfg.Throttle {
		return Decision{}, false
	}
	g.last[cfg.Mode] = now
	return Decision{Mode: cfg.Mode, VisualFor: cfg.VisualFor, At: now}, true
}

// Reset forgets throttle state after an atomic policy replacement.
func (g *Gate) Reset() {
	clear(g.last)
}
