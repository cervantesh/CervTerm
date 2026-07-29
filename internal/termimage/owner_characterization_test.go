package termimage

import (
	"errors"
	"reflect"
	"testing"
)

func TestL302StoreOwnerRejectsSequentialCrossThreadPublication(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	before := captureL302StoreFingerprint(store)
	result := make(chan error, 1)
	go func() {
		_, _, prepareErr := owner.PrepareCandidate(candidate)
		result <- prepareErr
	}()
	if err := <-result; !errors.Is(err, ErrWrongOwnerThread) {
		t.Fatalf("cross-thread prepare error=%v", err)
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) || !candidate.ValidFor(store) {
		t.Fatalf("cross-thread publication changed state: before=%#v after=%#v", before, after)
	}
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Acquire(ref); !ok {
		t.Fatal("owner-thread publication missing")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

type l302StoreFingerprint struct {
	epoch  StoreEpoch
	usage  Usage
	refs   []ResourceRef
	closed bool
}

func captureL302StoreFingerprint(store *Store) l302StoreFingerprint {
	return l302StoreFingerprint{epoch: store.Epoch(), usage: store.Usage(), refs: store.ResourceRefs(), closed: store.Closed()}
}

func TestL302StoreOwnerRejectsConcurrentReentrantClosedStaleAndWrongOwner(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := owner.enter()
	if err != nil {
		t.Fatal(err)
	}
	attempts := map[string]func() error{
		"prepare-candidate": func() error { _, _, err := owner.PrepareCandidate(candidate); return err },
		"prepare-retention": func() error { _, _, err := owner.PrepareCandidateWithRetention(candidate, ResourceDurable); return err },
		"prepare-removal":   func() error { _, err := owner.PrepareResourceRemoval(nil); return err },
		"prepare-reset":     func() error { _, err := owner.PrepareReset(); return err },
		"publish":           func() error { return owner.PublishPrepared(nil) },
		"close":             owner.Close,
	}
	for name, attempt := range attempts {
		t.Run(name, func(t *testing.T) {
			before := captureL302StoreFingerprint(store)
			result := make(chan error, 1)
			go func() { result <- attempt() }()
			if err := <-result; !errors.Is(err, ErrWrongOwnerThread) {
				t.Fatalf("cross-thread error=%v", err)
			}
			after := captureL302StoreFingerprint(store)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("busy mutation changed state: before=%#v after=%#v", before, after)
			}
		})
	}
	owner.leave(scope)
	if !candidate.ValidFor(store) {
		t.Fatal("rejected owner mutation consumed candidate")
	}

	otherStore := NewStore(NewProcessBudget(), DefaultLimits())
	otherOwner := otherStore.ClaimOwner()
	fake := &StoreOwner{store: store, generation: owner.generation}
	before := captureL302StoreFingerprint(store)
	if _, err := fake.PrepareReset(); !errors.Is(err, ErrWrongOwner) {
		t.Fatalf("wrong owner error=%v", err)
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong owner changed state: before=%#v after=%#v", before, after)
	}
	_ = otherOwner.Close()

	store.ownerGeneration.Add(1)
	if _, err := owner.PrepareReset(); !errors.Is(err, ErrStaleOwner) {
		t.Fatalf("stale owner error=%v", err)
	}
	store.ownerGeneration.Store(owner.generation)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	before = captureL302StoreFingerprint(store)
	if _, err := owner.PrepareReset(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed owner error=%v", err)
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("closed owner changed state: before=%#v after=%#v", before, after)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("idempotent close=%v", err)
	}
}

func TestStoreOwnerPublishAndCloseRejectionRetainExactOwnership(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	publishErr := errors.New("publish rejected")
	store.ownerFault = func(stage string) error {
		if stage == "publish" {
			return publishErr
		}
		return nil
	}
	before := captureL302StoreFingerprint(store)
	if err := owner.PublishPrepared(prepared); !errors.Is(err, publishErr) {
		t.Fatalf("publish rejection=%v", err)
	}
	if _, ok := store.Acquire(ref); ok {
		t.Fatal("rejected publish exposed prepared resource")
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("publish rejection changed store: before=%#v after=%#v", before, after)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if usage := store.Usage(); usage != (Usage{}) {
		t.Fatalf("publish abort leaked usage=%#v", usage)
	}
	closeErr := errors.New("close rejected")
	store.ownerFault = func(stage string) error {
		if stage == "close" {
			return closeErr
		}
		return nil
	}
	before = captureL302StoreFingerprint(store)
	if err := owner.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("close rejection=%v", err)
	}
	if store.Closed() || store.owner.Load() != owner || owner.released.Load() {
		t.Fatal("close rejection released store ownership")
	}
	if after := captureL302StoreFingerprint(store); !reflect.DeepEqual(before, after) {
		t.Fatalf("close rejection changed store: before=%#v after=%#v", before, after)
	}
	store.ownerFault = nil
	if err := owner.Close(); err != nil {
		t.Fatalf("close retry=%v", err)
	}
	if !store.Closed() || store.owner.Load() != nil {
		t.Fatal("close retry did not finalize ownership")
	}
}
