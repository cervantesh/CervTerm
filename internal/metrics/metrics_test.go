package metrics

import "testing"

func TestMeterSnapshotIncludesCounters(t *testing.T) {
	var m Meter
	m.AddBytes(128)
	m.AddFrame()
	m.AddFrame()

	s := m.Snapshot()
	if s.Bytes != 128 {
		t.Fatalf("bytes mismatch: got %d", s.Bytes)
	}
	if s.Frames != 2 {
		t.Fatalf("frames mismatch: got %d", s.Frames)
	}
	if s.At.IsZero() {
		t.Fatalf("expected snapshot timestamp")
	}
	if s.Allocs == 0 || s.HeapAlloc == 0 {
		t.Fatalf("expected runtime memory counters, got allocs=%d heap=%d", s.Allocs, s.HeapAlloc)
	}
}
