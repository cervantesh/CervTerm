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
		if store.Usage() != (Usage{}) || len(store.state.Load().resources) != 0 {
			t.Fatal("stale publication resurrected state")
		}
	}
}

func TestPublishedPreparedStateRemainsLifecycleAccountedThroughOwnerCloseAndReset(t *testing.T) {
	t.Run("close", func(t *testing.T) {
		fixture := newPublishedLifecycleFixture(t)
		assertPublishedLifecycleFixture(t, fixture)

		if err := fixture.owner.Close(); err != nil {
			t.Fatal(err)
		}
		if !fixture.store.Closed() || fixture.store.prepared.Load() != nil {
			t.Fatal("owner close did not resolve the retained lifecycle gate")
		}
		assertLifecycleFixtureReleasedExactly(t, fixture)
		assertPreparedTransitionStaleSafe(t, fixture.published)
	})

	t.Run("reset", func(t *testing.T) {
		fixture := newPublishedLifecycleFixture(t)
		assertPublishedLifecycleFixture(t, fixture)

		reset, err := fixture.owner.PrepareReset()
		if err != nil {
			t.Fatal(err)
		}
		if !fixture.published.finished.Load() || fixture.store.prepared.Load() != reset {
			t.Fatal("owner reset bypassed the published transition")
		}
		if err := fixture.owner.PublishPrepared(reset); err != nil {
			t.Fatal(err)
		}
		if err := reset.Commit(); err != nil {
			t.Fatal(err)
		}
		if fixture.store.Closed() || fixture.store.prepared.Load() != nil {
			t.Fatal("owner reset did not finalize the retained lifecycle gate")
		}
		assertLifecycleFixtureReleasedExactly(t, fixture)
		assertPreparedTransitionStaleSafe(t, fixture.published)
		assertPreparedTransitionStaleSafe(t, reset)
		if err := fixture.owner.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPublishStateCASFailureLeavesDeterministicCheckedLifecycleGate(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	base := store.state.Load()
	replacedBase := false
	store.ownerFault = func(stage string) error {
		if stage == "publish-state-cas" {
			replacement := &storeState{
				resources: base.resources, nextGeneration: base.nextGeneration,
				epoch: base.epoch, closed: base.closed,
			}
			replacedBase = store.state.CompareAndSwap(base, replacement)
		}
		return nil
	}
	if err := owner.PublishPrepared(prepared); !errors.Is(err, ErrPreparedState) {
		t.Fatalf("state CAS failure error=%v", err)
	}
	store.ownerFault = nil
	if !replacedBase {
		t.Fatal("test did not force the state CAS failure")
	}
	if store.prepared.Load() != prepared || prepared.published.Load() || prepared.finished.Load() {
		t.Fatal("failed state CAS lost or resolved the lifecycle gate")
	}
	if _, ok := store.Acquire(ref); ok {
		t.Fatal("failed state CAS exposed the prepared resource")
	}

	conflict, err := store.NewDecodedCandidate(2, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := owner.PrepareCandidate(conflict); !errors.Is(err, ErrPreparedState) {
		t.Fatalf("prepare while failed publication unresolved=%v", err)
	}
	if !conflict.ValidFor(store) {
		t.Fatal("conflicting prepare consumed candidate ownership")
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if store.prepared.Load() != nil || !prepared.finished.Load() {
		t.Fatal("abort did not deterministically release the failed publication gate")
	}
	retry, _, err := owner.PrepareCandidate(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Abort(); err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage != (Usage{}) {
		t.Fatalf("failed state CAS cleanup leaked usage=%#v", usage)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

type publishedLifecycleFixture struct {
	process   *ProcessBudget
	store     *Store
	owner     *StoreOwner
	published *PreparedStoreState
	oldRef    ResourceRef
	newRef    ResourceRef
	transfer  *CandidateTransfer
	placement *PlacementReservation
	conflict  *DecodedCandidate
}

func newPublishedLifecycleFixture(t testing.TB) publishedLifecycleFixture {
	t.Helper()
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	owner := claimTestStoreOwner(t, store)
	oldRef := publishConcurrencyResource(t, owner, 1, 1, 1, 1)
	published, newRef := prepareConcurrencyResource(t, owner, 1, 2, 1, 2)
	if err := owner.PublishPrepared(published); err != nil {
		t.Fatal(err)
	}
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Append([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	placement, err := store.ReservePlacements(1)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := store.NewDecodedCandidate(3, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	return publishedLifecycleFixture{
		process: process, store: store, owner: owner, published: published,
		oldRef: oldRef, newRef: newRef, transfer: transfer, placement: placement, conflict: conflict,
	}
}

func assertPublishedLifecycleFixture(t testing.TB, fixture publishedLifecycleFixture) {
	t.Helper()
	if fixture.store.prepared.Load() != fixture.published || !fixture.published.published.Load() || fixture.published.finished.Load() {
		t.Fatal("published transition was not retained as the active lifecycle gate")
	}
	if _, ok := fixture.store.Acquire(fixture.oldRef); ok {
		t.Fatal("retired resource remained visible after publication")
	}
	if _, ok := fixture.store.Acquire(fixture.newRef); !ok {
		t.Fatal("new resource was not visible after publication")
	}
	if _, _, err := fixture.owner.PrepareCandidate(fixture.conflict); !errors.Is(err, ErrPreparedState) {
		t.Fatalf("conflicting prepare while publication unresolved=%v", err)
	}
	if !fixture.conflict.ValidFor(fixture.store) {
		t.Fatal("conflicting prepare consumed candidate ownership")
	}
	want := Usage{EncodedBytes: 3, DecodedBytes: 24, Images: 3, Placements: 1, PendingTransfers: 1}
	if got := fixture.store.Usage(); got != want {
		t.Fatalf("published retained usage=%#v want=%#v", got, want)
	}
	if got := fixture.process.Usage(); got != want {
		t.Fatalf("published retained process usage=%#v want=%#v", got, want)
	}
}

func assertLifecycleFixtureReleasedExactly(t testing.TB, fixture publishedLifecycleFixture) {
	t.Helper()
	if !fixture.transfer.Closed() || !fixture.placement.closed.Load() || fixture.conflict.ValidFor(fixture.store) || fixture.conflict.RGBA() != nil {
		t.Fatal("lifecycle resolution did not close transfers, placements, and candidates")
	}
	if got := fixture.store.Usage(); got != (Usage{}) {
		t.Fatalf("lifecycle resolution leaked pane usage=%#v", got)
	}
	if got := fixture.process.Usage(); got != (Usage{}) {
		t.Fatalf("lifecycle resolution leaked process usage=%#v", got)
	}
	fixture.transfer.Close()
	fixture.placement.Close()
	fixture.conflict.Close()
	if fixture.store.Usage() != (Usage{}) || fixture.process.Usage() != (Usage{}) {
		t.Fatal("idempotent closes released lifecycle ownership more than once")
	}
}

func assertPreparedTransitionStaleSafe(t testing.TB, prepared *PreparedStoreState) {
	t.Helper()
	for name, transition := range map[string]func() error{
		"commit": prepared.Commit,
		"abort":  prepared.Abort,
		"close":  prepared.Close,
	} {
		if err := transition(); err != nil {
			t.Fatalf("stale %s transition=%v", name, err)
		}
	}
}
