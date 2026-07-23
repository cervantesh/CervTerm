package notificationpolicy

import (
	"testing"
	"time"
)

func TestGateRequiresConsentFreshnessFocusAndRate(t *testing.T) {
	now := time.Unix(100, 0)
	gate := NewGate(func() time.Time { return now })
	config := Config{Enabled: true, Unfocused: true, MinInterval: time.Second}
	if gate.Allow(Config{}, false, true) || gate.Allow(config, false, false) || gate.Allow(config, true, true) {
		t.Fatal("denied request passed policy")
	}
	if !gate.Allow(config, false, true) {
		t.Fatal("fresh consented unfocused request rejected")
	}
	now = now.Add(999 * time.Millisecond)
	if gate.Allow(config, false, true) {
		t.Fatal("rate-limited request passed")
	}
	now = now.Add(time.Millisecond)
	if !gate.Allow(config, false, true) {
		t.Fatal("request at interval boundary rejected")
	}
	gate.Reset()
	if !gate.Allow(config, false, true) {
		t.Fatal("reset did not clear rate state")
	}
}
