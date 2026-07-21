package termimage

import (
	"errors"
	"testing"
)

func TestPreparedCandidateRevokesMutationAndClose(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	candidate, _ := store.NewDecodedCandidate(1, 1, 1)
	if err := candidate.WriteRGBAAt(0, []byte{3, 4, 5, 6}); err != nil {
		t.Fatal(err)
	}
	alias := candidate.RGBA()
	prepared, ref, err := store.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	alias[0] = 99
	candidate.Close()
	if candidate.RGBA() != nil || candidate.ValidFor(store) || !errors.Is(candidate.WriteRGBAAt(0, []byte{1}), ErrCandidateInvalid) {
		t.Fatal("claimed candidate remained mutable")
	}
	store.PublishPrepared(prepared)
	prepared.Finalize()
	resource, ok := store.Acquire(ref)
	if !ok || resource.RGBA[0] != 3 {
		t.Fatalf("retained alias changed publication: %#v", resource.RGBA)
	}
}

func TestStoreAllowsOnlyOnePreparedStateAndAbortReleasesSlot(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	first, _ := store.NewDecodedCandidate(1, 1, 1)
	prepared, _, err := store.PrepareCandidate(first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := store.NewDecodedCandidate(2, 1, 1)
	if _, _, err = store.PrepareCandidate(second); !errors.Is(err, ErrPreparedState) {
		t.Fatalf("overlap error=%v", err)
	}
	if !second.ValidFor(store) {
		t.Fatal("rejected candidate ownership was consumed")
	}
	prepared.Abort()
	if _, _, err = store.PrepareCandidate(second); err != nil {
		t.Fatal(err)
	}
	store.abortPrepared()
	if store.Usage() != (Usage{}) {
		t.Fatalf("abort usage=%#v", store.Usage())
	}
}

func TestResetAndCloseAbortPreparedWithoutResurrection(t *testing.T) {
	for _, closeStore := range []bool{false, true} {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		candidate, _ := store.NewDecodedCandidate(1, 1, 1)
		prepared, _, err := store.PrepareCandidate(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if closeStore {
			store.Close()
		} else {
			store.Reset()
		}
		if store.Usage() != (Usage{}) {
			t.Fatalf("close=%v usage=%#v", closeStore, store.Usage())
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("stale publication did not panic before swap")
				}
			}()
			store.PublishPrepared(prepared)
		}()
		if store.Usage() != (Usage{}) || len(store.state.resources) != 0 {
			t.Fatal("stale publication resurrected state")
		}
	}
}
