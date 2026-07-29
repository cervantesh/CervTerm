package termimage

import (
	"errors"
	"reflect"
	"testing"
)

func TestStoreOwnerClosePreflightRejectsBeforeRetainingGateOrOwnership(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	injected := errors.New("close preflight rejected")
	store.ownerFault = func(stage string) error {
		if stage == "close" {
			return injected
		}
		return nil
	}
	before := captureL302StoreFingerprint(store)
	if prepared, err := owner.PrepareClose(store); prepared != nil || !errors.Is(err, injected) {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	if owner.activeScope.Load() != 0 || owner.released.Load() || store.owner.Load() != owner {
		t.Fatal("rejected close preflight changed owner state")
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected close preflight changed store: before=%#v after=%#v", before, after)
	}
	store.ownerFault = nil
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedStoreCloseReservesGateAndRejectsLeakedCrossThreadScope(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	prepared, err := owner.PrepareClose(store)
	if err != nil {
		t.Fatal(err)
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != prepared {
		t.Fatal("close preflight did not reserve the owner gate")
	}
	if _, err := owner.PrepareReset(); !errors.Is(err, ErrOwnerBusy) {
		t.Fatalf("mutation during close preflight=%v", err)
	}
	commitErr := make(chan error, 1)
	go func() { commitErr <- prepared.Commit() }()
	if err := <-commitErr; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread commit=%v", err)
	}
	abortErr := make(chan error, 1)
	go func() { abortErr <- prepared.Abort() }()
	if err := <-abortErr; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread abort=%v", err)
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != prepared || store.owner.Load() != owner || store.Closed() {
		t.Fatal("leaked close request changed ownership")
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != nil {
		t.Fatal("owner-thread abort retained close reservation")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedStoreCloseValidateReacquiresOwnerScopeWithoutMutation(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	prepared, err := owner.PrepareClose(store)
	if err != nil {
		t.Fatal(err)
	}
	before := captureL302StoreFingerprint(store)

	wrongThread := make(chan error, 1)
	go func() { wrongThread <- prepared.Validate() }()
	if err := <-wrongThread; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread validation=%v", err)
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != prepared {
		t.Fatal("cross-thread validation changed close ownership")
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("cross-thread validation changed store: before=%#v after=%#v", before, after)
	}

	beforeNonce := owner.nextScope.Load()
	if err := prepared.Validate(); err != nil {
		t.Fatal(err)
	}
	if owner.nextScope.Load() <= beforeNonce {
		t.Fatal("validation did not reacquire a fresh owner scope")
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != prepared {
		t.Fatal("validation retained its owner scope or released the close gate")
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("validation changed store: before=%#v after=%#v", before, after)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedStoreCloseStaleCommitLeavesGateAndStateForOwnerRecovery(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	prepared, err := owner.PrepareClose(store)
	if err != nil {
		t.Fatal(err)
	}
	store.ownerGeneration.Add(1)
	if err := prepared.Commit(); !errors.Is(err, ErrStaleOwner) {
		t.Fatalf("stale close commit=%v", err)
	}
	if owner.activeScope.Load() != 0 || store.preparedClose.Load() != prepared || owner.released.Load() || store.Closed() || store.owner.Load() != owner {
		t.Fatal("stale close commit changed ownership")
	}
	store.ownerGeneration.Store(owner.generation)
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}
