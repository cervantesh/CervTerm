package face

import "testing"

func TestOwnerCloseIsIdempotent(t *testing.T) {
	closes := 0
	owner := NewOwner(42, func(value int) {
		if value != 42 {
			t.Fatalf("closed value = %d", value)
		}
		closes++
	})
	if got := owner.Value(); got != 42 {
		t.Fatalf("Value() = %d", got)
	}
	owner.Close()
	owner.Close()
	if closes != 1 {
		t.Fatalf("close calls = %d, want 1", closes)
	}
	var nilOwner *Owner[int]
	nilOwner.Close()
	if got := nilOwner.Value(); got != 0 {
		t.Fatalf("nil owner value = %d", got)
	}
}
