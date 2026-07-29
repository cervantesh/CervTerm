package termimage

import "testing"

func TestPreparedCandidateAbortAndPublishOwnership(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.WriteRGBAAt(0, []byte{7}); err != nil {
		t.Fatal(err)
	}
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if store.state.Load().nextGeneration != 0 {
		t.Fatal("preparation consumed generation")
	}
	if _, ok := store.Acquire(ref); ok {
		t.Fatal("prepared resource visible early")
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
		t.Fatal("abort leaked candidate reservation")
	}

	candidate, _ = store.NewDecodedCandidate(1, 1, 1)
	if err := candidate.WriteRGBAAt(0, []byte{9}); err != nil {
		t.Fatal(err)
	}
	prepared, ref, err = owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	resource, ok := store.Acquire(ref)
	if !ok || resource.RGBA[0] != 9 || candidate.RGBA() != nil {
		t.Fatalf("published=%#v candidate=%v", resource, candidate.RGBA())
	}
	if usage := store.Usage(); usage.Images != 1 || usage.DecodedBytes != 4 {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestPreparedReplacementAndRemovalReleaseAfterCommit(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	first, _ := store.NewDecodedCandidate(2, 1, 1)
	prepared, oldRef, _ := owner.PrepareCandidate(first)
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	second, _ := store.NewDecodedCandidate(2, 1, 1)
	replacement, newRef, err := owner.PrepareCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage.Images != 2 || usage.DecodedBytes != 8 {
		t.Fatalf("pre-publication usage=%#v", usage)
	}
	if err := owner.PublishPrepared(replacement); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Acquire(newRef); !ok {
		t.Fatal("replacement not published")
	}
	if usage := store.Usage(); usage.Images != 2 {
		t.Fatal("retired resource released before commit")
	}
	if err := replacement.Commit(); err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage.Images != 1 || usage.DecodedBytes != 4 {
		t.Fatalf("final usage=%#v", usage)
	}
	if _, ok := store.Acquire(oldRef); ok {
		t.Fatal("old generation survived")
	}
	removal, err := owner.PrepareResourceRemoval([]ResourceRef{newRef, newRef})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PublishPrepared(removal); err != nil {
		t.Fatal(err)
	}
	if err := removal.Commit(); err != nil {
		t.Fatal(err)
	}
	if store.Usage() != (Usage{}) {
		t.Fatalf("removal usage=%#v", store.Usage())
	}
}

func TestPlacementReservationExactlyOnce(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	lease, err := store.ReservePlacements(1)
	if err != nil {
		t.Fatal(err)
	}
	if store.Usage().Placements != 1 {
		t.Fatal("placement not reserved")
	}
	lease.Close()
	lease.Close()
	if store.Usage().Placements != 0 {
		t.Fatal("placement reservation leaked")
	}
}
