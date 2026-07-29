package termimage

import "testing"

type ownerCloseResult struct {
	firstErr  error
	secondErr error
	locked    bool
}

func TestClaimOwnerSerializesWithStandaloneCloseAndReset(t *testing.T) {
	t.Run("claim wins close", func(t *testing.T) {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		claimEntered := make(chan struct{})
		releaseClaim := make(chan struct{})
		store.ownerFault = func(stage string) error {
			if stage == "claim" {
				close(claimEntered)
				<-releaseClaim
			}
			return nil
		}

		claimed := make(chan *StoreOwner, 1)
		finishOwner := make(chan struct{})
		ownerDone := make(chan ownerCloseResult, 1)
		go func() {
			owner := store.ClaimOwner()
			claimed <- owner
			<-finishOwner
			result := ownerCloseResult{}
			if owner != nil {
				result.firstErr = owner.Close()
				result.secondErr = owner.Close()
				result.locked = owner.threadLocked.Load()
			}
			ownerDone <- result
		}()
		<-claimEntered

		closeStarted := make(chan struct{})
		closeDone := make(chan struct{})
		go func() {
			close(closeStarted)
			store.Close()
			close(closeDone)
		}()
		<-closeStarted
		close(releaseClaim)

		owner := <-claimed
		<-closeDone
		if owner == nil || store.owner.Load() != owner || store.Closed() {
			t.Fatal("standalone close overtook a serialized owner claim")
		}
		close(finishOwner)
		result := <-ownerDone
		if result.firstErr != nil || result.secondErr != nil || result.locked || !store.Closed() {
			t.Fatalf("owner close result=%#v closed=%v", result, store.Closed())
		}
	})

	t.Run("close wins claim", func(t *testing.T) {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		closeEntered := make(chan struct{})
		releaseClose := make(chan struct{})
		store.ownerFault = func(stage string) error {
			if stage == "standalone-close" {
				close(closeEntered)
				<-releaseClose
			}
			return nil
		}

		closeDone := make(chan struct{})
		go func() {
			store.Close()
			close(closeDone)
		}()
		<-closeEntered

		claimStarted := make(chan struct{})
		claimed := make(chan *StoreOwner, 1)
		go func() {
			close(claimStarted)
			claimed <- store.ClaimOwner()
		}()
		<-claimStarted
		close(releaseClose)
		<-closeDone

		if owner := <-claimed; owner != nil || !store.Closed() || store.owner.Load() != nil {
			t.Fatalf("owner=%p closed=%v published=%p", owner, store.Closed(), store.owner.Load())
		}
	})

	t.Run("claim wins reset", func(t *testing.T) {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
		if err != nil {
			t.Fatal(err)
		}
		claimEntered := make(chan struct{})
		releaseClaim := make(chan struct{})
		store.ownerFault = func(stage string) error {
			if stage == "claim" {
				close(claimEntered)
				<-releaseClaim
			}
			return nil
		}

		claimed := make(chan *StoreOwner, 1)
		finishOwner := make(chan struct{})
		ownerDone := make(chan ownerCloseResult, 1)
		go func() {
			owner := store.ClaimOwner()
			claimed <- owner
			<-finishOwner
			result := ownerCloseResult{}
			if owner != nil {
				result.firstErr = owner.Close()
				result.secondErr = owner.Close()
				result.locked = owner.threadLocked.Load()
			}
			ownerDone <- result
		}()
		<-claimEntered

		resetStarted := make(chan struct{})
		resetDone := make(chan struct{})
		go func() {
			close(resetStarted)
			store.Reset()
			close(resetDone)
		}()
		<-resetStarted
		close(releaseClaim)

		owner := <-claimed
		<-resetDone
		if owner == nil || store.owner.Load() != owner || store.Epoch() != 1 || transfer.Closed() {
			t.Fatalf("owner=%p published=%p epoch=%d transferClosed=%v", owner, store.owner.Load(), store.Epoch(), transfer.Closed())
		}
		close(finishOwner)
		result := <-ownerDone
		if result.firstErr != nil || result.secondErr != nil || result.locked || !store.Closed() || store.Usage() != (Usage{}) {
			t.Fatalf("owner close result=%#v closed=%v usage=%#v", result, store.Closed(), store.Usage())
		}
	})

	t.Run("reset completes before claim publication", func(t *testing.T) {
		store := NewStore(NewProcessBudget(), DefaultLimits())
		transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
		if err != nil {
			t.Fatal(err)
		}
		resetEntered := make(chan struct{})
		releaseReset := make(chan struct{})
		store.ownerFault = func(stage string) error {
			if stage == "standalone-reset" {
				close(resetEntered)
				<-releaseReset
			}
			return nil
		}

		resetDone := make(chan struct{})
		go func() {
			store.Reset()
			close(resetDone)
		}()
		<-resetEntered

		claimStarted := make(chan struct{})
		claimed := make(chan *StoreOwner, 1)
		finishOwner := make(chan struct{})
		ownerDone := make(chan ownerCloseResult, 1)
		go func() {
			close(claimStarted)
			owner := store.ClaimOwner()
			claimed <- owner
			<-finishOwner
			result := ownerCloseResult{}
			if owner != nil {
				result.firstErr = owner.Close()
				result.secondErr = owner.Close()
				result.locked = owner.threadLocked.Load()
			}
			ownerDone <- result
		}()
		<-claimStarted
		close(releaseReset)
		<-resetDone

		owner := <-claimed
		if owner == nil || store.owner.Load() != owner || store.Epoch() != 2 || !transfer.Closed() || store.Usage() != (Usage{}) {
			t.Fatalf("owner=%p published=%p epoch=%d transferClosed=%v usage=%#v", owner, store.owner.Load(), store.Epoch(), transfer.Closed(), store.Usage())
		}
		close(finishOwner)
		result := <-ownerDone
		if result.firstErr != nil || result.secondErr != nil || result.locked || !store.Closed() {
			t.Fatalf("owner close result=%#v closed=%v", result, store.Closed())
		}
	})
}

func TestAcquireSnapshotsLinearizeAcrossPublishAndClose(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	owner := store.ClaimOwner()
	if owner == nil {
		t.Fatal("store owner unavailable")
	}

	oldRef := publishConcurrencyResource(t, owner, 1, 1, 1, 1)
	replacement, newRef := prepareConcurrencyResource(t, owner, 1, 2, 1, 2)
	loaded := make(chan struct{})
	releaseRead := make(chan struct{})
	store.testReadHook = func(stage string) {
		if stage == "acquire-loaded" {
			close(loaded)
			<-releaseRead
		}
	}
	acquired := make(chan DetachedResource, 1)
	go func() {
		resource, _ := store.Acquire(oldRef)
		acquired <- resource
	}()
	<-loaded
	if err := owner.PublishPrepared(replacement); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Commit(); err != nil {
		t.Fatal(err)
	}
	close(releaseRead)
	oldCopy := <-acquired
	store.testReadHook = nil
	if oldCopy.Ref != oldRef || oldCopy.Width != 1 || len(oldCopy.RGBA) != 4 || oldCopy.RGBA[0] != 1 {
		t.Fatalf("in-flight acquire did not retain the old detached snapshot: %#v", oldCopy)
	}
	newCopy, ok := store.Acquire(newRef)
	if !ok || newCopy.Width != 2 || len(newCopy.RGBA) != 8 || newCopy.RGBA[0] != 2 {
		t.Fatalf("post-publication acquire did not observe the new snapshot: %#v ok=%v", newCopy, ok)
	}

	loaded = make(chan struct{})
	releaseRead = make(chan struct{})
	store.testReadHook = func(stage string) {
		if stage == "dimensions-loaded" {
			close(loaded)
			<-releaseRead
		}
	}
	dimensions := make(chan struct {
		width, height uint32
		ok            bool
	}, 1)
	go func() {
		width, height, found := store.ResourceDimensions(newRef)
		dimensions <- struct {
			width, height uint32
			ok            bool
		}{width: width, height: height, ok: found}
	}()
	<-loaded
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	close(releaseRead)
	oldDimensions := <-dimensions
	store.testReadHook = nil
	if !oldDimensions.ok || oldDimensions.width != 2 || oldDimensions.height != 1 {
		t.Fatalf("in-flight dimensions lost the old snapshot: %#v", oldDimensions)
	}
	if _, ok := store.Acquire(newRef); ok {
		t.Fatal("acquire after close publication succeeded")
	}
	if _, _, ok := store.ResourceDimensions(newRef); ok {
		t.Fatal("dimensions after close publication succeeded")
	}
	if _, ok := store.ResourceRef(newRef.Image); ok || len(store.ResourceRefs()) != 0 {
		t.Fatal("resource lookup after close publication succeeded")
	}
	if _, ok := store.ResourceRetention(newRef); ok {
		t.Fatal("retention lookup after close publication succeeded")
	}
	if store.Usage() != (Usage{}) {
		t.Fatalf("close leaked usage=%#v", store.Usage())
	}
}

func publishConcurrencyResource(t testing.TB, owner *StoreOwner, image ImageID, width, height uint32, fill byte) ResourceRef {
	t.Helper()
	prepared, ref := prepareConcurrencyResource(t, owner, image, width, height, fill)
	if err := owner.PublishPrepared(prepared); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	return ref
}

func prepareConcurrencyResource(t testing.TB, owner *StoreOwner, image ImageID, width, height uint32, fill byte) (*PreparedStoreState, ResourceRef) {
	t.Helper()
	candidate, err := owner.store.NewDecodedCandidate(image, width, height)
	if err != nil {
		t.Fatal(err)
	}
	_, size, err := CheckedRGBABytes(width, height)
	if err != nil {
		t.Fatal(err)
	}
	pixels := make([]byte, int(size))
	for index := range pixels {
		pixels[index] = fill
	}
	if err := candidate.WriteRGBAAt(0, pixels); err != nil {
		t.Fatal(err)
	}
	prepared, ref, err := owner.PrepareCandidate(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, ref
}
