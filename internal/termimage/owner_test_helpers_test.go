package termimage

import "testing"

func claimTestStoreOwner(t testing.TB, store *Store) *StoreOwner {
	t.Helper()
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Errorf("close store owner: %v", err)
		}
	})
	return owner
}
