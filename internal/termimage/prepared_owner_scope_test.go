package termimage

import (
	"errors"
	"reflect"
	"testing"
)

func prepareOwnedCandidateForScopeTest(t *testing.T) (*Store, *StoreOwner, *PreparedStoreState, ResourceRef) {
	t.Helper()
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
	return store, owner, prepared, ref
}

func TestPreparedStoreStateCrossThreadTransitionsLeaveState(t *testing.T) {
	store, owner, prepared, ref := prepareOwnedCandidateForScopeTest(t)
	before := captureL302StoreFingerprint(store)
	abortResult := make(chan error, 1)
	go func() { abortResult <- prepared.Abort() }()
	if err := <-abortResult; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread abort=%v", err)
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- prepared.Close() }()
	if err := <-closeResult; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread close=%v", err)
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) || store.prepared.Load() != prepared || prepared.finished.Load() {
		t.Fatalf("cross-thread abort/close changed state: before=%#v after=%#v", before, after)
	}
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	}
	commitResult := make(chan error, 1)
	go func() { commitResult <- prepared.Commit() }()
	if err := <-commitResult; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread commit=%v", err)
	}
	if _, ok := store.Acquire(ref); !ok || prepared.finished.Load() {
		t.Fatal("cross-thread commit changed published state")
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedStoreStateRejectsBusyStaleClosedAndZeroState(t *testing.T) {
	store, owner, prepared, _ := prepareOwnedCandidateForScopeTest(t)
	scope, err := owner.enter()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(); !errors.Is(err, ErrOwnerBusy) {
		t.Fatalf("busy abort=%v", err)
	}
	if store.prepared.Load() != prepared || prepared.finished.Load() {
		t.Fatal("busy abort changed prepared state")
	}
	owner.leave(scope)

	store.ownerGeneration.Add(1)
	if err := prepared.Abort(); !errors.Is(err, ErrStaleOwner) {
		t.Fatalf("stale abort=%v", err)
	}
	if store.prepared.Load() != prepared || prepared.finished.Load() {
		t.Fatal("stale abort changed prepared state")
	}
	store.ownerGeneration.Store(owner.generation)

	owner.released.Store(true)
	if err := prepared.Close(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed close=%v", err)
	}
	if store.prepared.Load() != prepared || prepared.finished.Load() {
		t.Fatal("closed close changed prepared state")
	}
	owner.released.Store(false)
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}

	var zero PreparedStoreState
	if err := zero.Commit(); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("zero commit=%v", err)
	}
	if err := zero.Abort(); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("zero abort=%v", err)
	}
	if err := zero.Close(); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("zero close=%v", err)
	}
}
