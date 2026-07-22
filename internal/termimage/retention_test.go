package termimage

import (
	"errors"
	"testing"
)

func TestPreparedRetentionPublishesAndAbortPreservesOldGeneration(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	candidate, _ := store.NewDecodedCandidate(MinInternalImageID, 1, 1)
	prepared, ref, err := owner.PrepareCandidateWithRetention(candidate, ResourceEphemeral)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.ResourceRetention(ref); ok {
		t.Fatal("retention visible before publish")
	}
	owner.PublishPrepared(prepared)
	if retention, ok := store.ResourceRetention(ref); !ok || retention != ResourceEphemeral {
		t.Fatalf("retention=%v ok=%v", retention, ok)
	}
	prepared.Finalize()

	replacement, _ := store.NewDecodedCandidate(MinInternalImageID, 1, 1)
	pending, _, err := owner.PrepareCandidateWithRetention(replacement, ResourceDurable)
	if err != nil {
		t.Fatal(err)
	}
	pending.Abort()
	if retention, ok := store.ResourceRetention(ref); !ok || retention != ResourceEphemeral {
		t.Fatal("abort changed prior retention")
	}
}

func TestInvalidRetentionRejectsAndReleasesCandidate(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	owner := store.ClaimOwner()
	candidate, _ := store.NewDecodedCandidate(1, 1, 1)
	if _, _, err := owner.PrepareCandidateWithRetention(candidate, ResourceRetention(99)); !errors.Is(err, ErrInvalidRetention) {
		t.Fatalf("err=%v", err)
	}
	if store.Usage() != (Usage{}) || process.Usage() != (Usage{}) {
		t.Fatalf("usage=%#v", store.Usage())
	}
}
