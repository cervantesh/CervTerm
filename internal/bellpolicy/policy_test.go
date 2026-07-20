package bellpolicy

import (
	"testing"
	"time"
)

func TestGateFocusThrottleAndModes(t *testing.T) {
	now := time.Unix(100, 0)
	gate := NewGate(func() time.Time { return now })
	cfg := Config{Mode: Visual, Unfocused: true, Throttle: 250 * time.Millisecond, VisualFor: 120 * time.Millisecond}
	if _, ok := gate.Decide(cfg, true); ok {
		t.Fatal("focused bell must not reach an unfocused-only sink")
	}
	got, ok := gate.Decide(cfg, false)
	if !ok || got.Mode != Visual || got.VisualFor != 120*time.Millisecond {
		t.Fatalf("decision = %#v, %t", got, ok)
	}
	now = now.Add(249 * time.Millisecond)
	if _, ok := gate.Decide(cfg, false); ok {
		t.Fatal("burst must be throttled")
	}
	now = now.Add(time.Millisecond)
	if _, ok := gate.Decide(cfg, false); !ok {
		t.Fatal("bell at throttle boundary must pass")
	}
	cfg.Mode = Audible
	if _, ok := gate.Decide(cfg, false); !ok {
		t.Fatal("independent sink mode must have independent throttle state")
	}
}

func TestGateDisabledAlwaysAndReset(t *testing.T) {
	now := time.Unix(200, 0)
	gate := NewGate(func() time.Time { return now })
	if _, ok := gate.Decide(Config{Mode: Disabled}, false); ok {
		t.Fatal("disabled mode must reject")
	}
	cfg := Config{Mode: Taskbar, Throttle: time.Second}
	if _, ok := gate.Decide(cfg, true); !ok {
		t.Fatal("always policy must permit focused window")
	}
	gate.Reset()
	if _, ok := gate.Decide(cfg, true); !ok {
		t.Fatal("reset must clear throttle state")
	}
}
