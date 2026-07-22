package termimage

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

func TestInternalIDNamespacesAndWirePredicates(t *testing.T) {
	if !IsWireImageID(1) || !IsWireImageID(MaxWireImageID) || IsWireImageID(0) || IsWireImageID(MinInternalImageID) {
		t.Fatal("image namespace predicate mismatch")
	}
	if !IsWirePlacementID(1) || !IsWirePlacementID(MaxWirePlacementID) || IsWirePlacementID(0) || IsWirePlacementID(MinInternalPlacementID) {
		t.Fatal("placement namespace predicate mismatch")
	}
	store := NewStore(NewProcessBudget(), DefaultLimits())
	imageOne, err := store.AllocateInternalImageID()
	if err != nil || imageOne != MinInternalImageID {
		t.Fatalf("image=%#x err=%v", imageOne, err)
	}
	imageTwo, err := store.AllocateInternalImageID()
	if err != nil || imageTwo != MinInternalImageID+1 {
		t.Fatalf("image=%#x err=%v", imageTwo, err)
	}
	placementOne, err := store.AllocateInternalPlacementID()
	if err != nil || placementOne != MinInternalPlacementID {
		t.Fatalf("placement=%#x err=%v", placementOne, err)
	}

	store.Reset()
	imageThree, err := store.AllocateInternalImageID()
	if err != nil || imageThree != MinInternalImageID+2 {
		t.Fatalf("reset reused image identity=%#x err=%v", imageThree, err)
	}
	store.Close()
	if _, err := store.AllocateInternalImageID(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed image allocation err=%v", err)
	}
	if _, err := store.AllocateInternalPlacementID(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed placement allocation err=%v", err)
	}
}

func TestInternalIDExhaustionNeverWrapsOrCrossesNamespace(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	store.nextInternalImage = ImageID(math.MaxUint32 - 1)
	store.nextInternalPlacement = PlacementID(math.MaxUint32 - 1)
	image, err := store.AllocateInternalImageID()
	if err != nil || image != ImageID(math.MaxUint32) {
		t.Fatalf("last image=%#x err=%v", image, err)
	}
	if image, err = store.AllocateInternalImageID(); image != 0 || !errors.Is(err, ErrInternalIDExhausted) {
		t.Fatalf("wrapped image=%#x err=%v", image, err)
	}
	placement, err := store.AllocateInternalPlacementID()
	if err != nil || placement != PlacementID(math.MaxUint32) {
		t.Fatalf("last placement=%#x err=%v", placement, err)
	}
	if placement, err = store.AllocateInternalPlacementID(); placement != 0 || !errors.Is(err, ErrInternalIDExhausted) {
		t.Fatalf("wrapped placement=%#x err=%v", placement, err)
	}
}

func TestInternalIDAllocationIsUniqueUnderContention(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	const count = 64
	ids := make(chan ImageID, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			id, err := store.AllocateInternalImageID()
			if err != nil {
				t.Errorf("allocate: %v", err)
				return
			}
			ids <- id
		}()
	}
	group.Wait()
	close(ids)
	seen := make(map[ImageID]struct{}, count)
	for id := range ids {
		if id < MinInternalImageID {
			t.Fatalf("wire-range id=%#x", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id=%#x", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("allocated=%d", len(seen))
	}
}

func TestLowHalfPhase13StoreBehaviorRemainsValid(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	candidate, err := store.NewDecodedCandidate(MaxWireImageID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Close()
	if _, err := ValidatePlacementSpec(PlacementSpec{ID: MaxWirePlacementID, Cols: 1, Rows: 1}, 1, 1); err != nil {
		t.Fatal(err)
	}
}

func TestInternalIDAllocationRacingCloseFailsBeforeCloseReturns(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	store.identityMu.Lock()
	started := make(chan struct{})
	allocated := make(chan error, 1)
	go func() {
		close(started)
		_, err := store.AllocateInternalImageID()
		allocated <- err
	}()
	<-started
	closed := make(chan struct{})
	go func() {
		store.Close()
		close(closed)
	}()
	deadline := time.After(2 * time.Second)
	for !store.closed.Load() {
		select {
		case <-deadline:
			store.identityMu.Unlock()
			t.Fatal("close did not publish closed state")
		default:
		}
	}
	store.identityMu.Unlock()
	if err := <-allocated; !errors.Is(err, ErrClosed) {
		t.Fatalf("allocation err=%v", err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not finish")
	}
}
