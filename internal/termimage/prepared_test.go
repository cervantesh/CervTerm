package termimage

import "testing"

func TestPreparedCandidateAbortAndPublishOwnership(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.WriteRGBAAt(0, []byte{7}); err != nil {
		t.Fatal(err)
	}
	prepared, ref, err := store.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if store.state.nextGeneration != 0 {
		t.Fatal("preparation consumed generation")
	}
	if _, ok := store.Acquire(ref); ok {
		t.Fatal("prepared resource visible early")
	}
	prepared.Abort()
	if process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
		t.Fatal("abort leaked candidate reservation")
	}

	candidate, _ = store.NewDecodedCandidate(1, 1, 1)
	if err := candidate.WriteRGBAAt(0, []byte{9}); err != nil {
		t.Fatal(err)
	}
	prepared, ref, err = store.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	store.PublishPrepared(prepared)
	prepared.Finalize()
	resource, ok := store.Acquire(ref)
	if !ok || resource.RGBA[0] != 9 || candidate.RGBA() != nil {
		t.Fatalf("published=%#v candidate=%v", resource, candidate.RGBA())
	}
	if usage := store.Usage(); usage.Images != 1 || usage.DecodedBytes != 4 {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestPreparedReplacementAndRemovalReleaseAfterFinalize(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	first, _ := store.NewDecodedCandidate(2, 1, 1)
	prepared, oldRef, _ := store.PrepareCandidate(first)
	store.PublishPrepared(prepared)
	prepared.Finalize()
	second, _ := store.NewDecodedCandidate(2, 1, 1)
	replacement, newRef, err := store.PrepareCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage.Images != 2 || usage.DecodedBytes != 8 {
		t.Fatalf("pre-publication usage=%#v", usage)
	}
	store.PublishPrepared(replacement)
	if _, ok := store.Acquire(newRef); !ok {
		t.Fatal("replacement not published")
	}
	if usage := store.Usage(); usage.Images != 2 {
		t.Fatal("retired resource released before finalize")
	}
	replacement.Finalize()
	if usage := store.Usage(); usage.Images != 1 || usage.DecodedBytes != 4 {
		t.Fatalf("final usage=%#v", usage)
	}
	if _, ok := store.Acquire(oldRef); ok {
		t.Fatal("old generation survived")
	}
	removal, err := store.PrepareResourceRemoval([]ResourceRef{newRef, newRef})
	if err != nil {
		t.Fatal(err)
	}
	store.PublishPrepared(removal)
	removal.Finalize()
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
