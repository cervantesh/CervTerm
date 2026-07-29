package termimage

import (
	"errors"
	"testing"
)

func TestPreparedCandidateRevokesMutationAndClose(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	candidate, _ := store.NewDecodedCandidate(1, 1, 1)
	if err := candidate.WriteRGBAAt(0, []byte{3, 4, 5, 6}); err != nil {
		t.Fatal(err)
	}
	alias := candidate.RGBA()
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	alias[0] = 99
	candidate.Close()
	if candidate.RGBA() != nil || candidate.ValidFor(store) || !errors.Is(candidate.WriteRGBAAt(0, []byte{1}), ErrCandidateInvalid) {
		t.Fatal("claimed candidate remained mutable")
	}
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	} else if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	resource, ok := store.Acquire(ref)
	if !ok || resource.RGBA[0] != 3 {
		t.Fatalf("retained alias changed publication: %#v", resource.RGBA)
	}
}

func TestStoreAllowsOnlyOnePreparedStateAndAbortReleasesSlot(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	first, _ := store.NewDecodedCandidate(1, 1, 1)
	prepared, _, err := owner.PrepareCandidate(first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := store.NewDecodedCandidate(2, 1, 1)
	if _, _, err = owner.PrepareCandidate(second); !errors.Is(err, ErrPreparedState) {
		t.Fatalf("overlap error=%v", err)
	}
	if !second.ValidFor(store) {
		t.Fatal("rejected candidate ownership was consumed")
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	secondPrepared, _, err := owner.PrepareCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondPrepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if store.Usage() != (Usage{}) {
		t.Fatalf("abort usage=%#v", store.Usage())
	}
}

func TestResetAndCloseResolvePreparedWithoutResurrection(t *testing.T) {
	for _, closeStore := range []bool{false, true} {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		owner := claimTestStoreOwner(t, store)
		candidate, _ := store.NewDecodedCandidate(1, 1, 1)
		prepared, _, err := owner.PrepareCandidate(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.Abort(); err != nil {
			t.Fatal(err)
		}
		if closeStore {
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
		} else {
			reset, err := owner.PrepareReset()
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.PublishPrepared(reset); err != nil {
				t.Fatal(err)
			}
			if err := reset.Commit(); err != nil {
				t.Fatal(err)
			}
		}
		if store.Usage() != (Usage{}) {
			t.Fatalf("close=%v usage=%#v", closeStore, store.Usage())
		}
		if err := owner.PublishPrepared(prepared); err == nil {
			t.Fatal("stale publication succeeded")
		}
		if store.Usage() != (Usage{}) || len(store.state.resources) != 0 {
			t.Fatal("stale publication resurrected state")
		}
	}
}
