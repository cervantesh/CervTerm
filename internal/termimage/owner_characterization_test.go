package termimage

import "testing"

func TestL302StoreOwnerPublicationCharacterization(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	before := store.ResourceRefs()
	result := make(chan error, 1)
	go func() {
		prepared, ref, prepareErr := owner.PrepareCandidate(candidate)
		if prepareErr != nil {
			result <- prepareErr
			return
		}
		owner.PublishPrepared(prepared)
		prepared.Finalize()
		if _, ok := store.Acquire(ref); !ok {
			result <- ErrCandidateInvalid
			return
		}
		result <- nil
	}()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if after := store.ResourceRefs(); len(before) != 0 || len(after) != 1 {
		t.Fatalf("publication before=%#v after=%#v", before, after)
	}
	owner.Close()
}
