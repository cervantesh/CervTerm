package cache

import (
	"testing"

	"cervterm/internal/fontglyph/internal/face"
)

func TestLeaseOwnsOnePinAndClosesIdempotently(t *testing.T) {
	manager := &Manager[int]{}
	item := &entry[int]{owner: face.NewOwner(7, nil), pins: 1}
	lease := &Lease[int]{manager: manager, entry: item}
	lease.Close()
	lease.Close()
	if item.pins != 0 {
		t.Fatalf("pins = %d, want 0", item.pins)
	}
	var nilLease *Lease[int]
	nilLease.Close()
}
