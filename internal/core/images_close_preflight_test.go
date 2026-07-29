package core

import (
	"errors"
	"reflect"
	"testing"

	"cervterm/internal/termimage"
)

type imageCloseFingerprint struct {
	store      *termimage.Store
	owner      *termimage.StoreOwner
	sidecars   *imageSidecars
	usage      termimage.Usage
	epoch      termimage.StoreEpoch
	refs       []termimage.ResourceRef
	generation uint64
}

func captureImageCloseFingerprint(terminal *Terminal) imageCloseFingerprint {
	result := imageCloseFingerprint{store: terminal.imageStore, owner: terminal.imageOwner, sidecars: terminal.imageSidecars}
	if terminal.imageStore != nil {
		result.usage = terminal.imageStore.Usage()
		result.epoch = terminal.imageStore.Epoch()
		result.refs = terminal.imageStore.ResourceRefs()
	}
	if terminal.imageSidecars != nil {
		result.generation = terminal.imageSidecars.generation
	}
	return result
}

func TestImageClosePreflightBlocksResetAndCloseWithoutStateChange(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	terminal.SetCursor(1, 2)
	terminal.SetTitle("preserved")
	before := captureImageCloseFingerprint(terminal)
	if err := terminal.ResetImages(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("reset during close preflight=%v", err)
	}
	if err := terminal.CloseImageStore(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("close during close preflight=%v", err)
	}
	terminal.Reset()
	if terminal.cursorRow != 1 || terminal.cursorCol != 2 || terminal.title != "preserved" {
		t.Fatal("rejected RIS reset changed terminal state")
	}
	if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected image lifecycle changed state: before=%#v after=%#v", before, after)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}

func TestImageClosePreflightRejectsWrongOwnerWithoutStoreSidecarDivergence(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	correct := terminal.imageOwner
	otherStore := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	other := otherStore.ClaimOwner()
	if other == nil {
		t.Fatal("other owner unavailable")
	}
	terminal.imageOwner = other
	before := captureImageCloseFingerprint(terminal)
	if prepared, err := terminal.PrepareCloseImageStore(); prepared != nil || !errors.Is(err, termimage.ErrWrongOwner) {
		t.Fatalf("prepared=%#v wrong-owner error=%v", prepared, err)
	}
	if err := terminal.CloseImageStore(); !errors.Is(err, termimage.ErrWrongOwner) {
		t.Fatalf("wrong-owner close=%v", err)
	}
	if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-owner close changed state: before=%#v after=%#v", before, after)
	}
	terminal.imageOwner = correct
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedImageStoreCloseRejectsExactStateMismatchWithoutConsumingResolution(t *testing.T) {
	resolutions := []struct {
		name      string
		resolve   func(*PreparedImageStoreClose) error
		repeat    func(*PreparedImageStoreClose) error
		committed bool
	}{
		{name: "commit", resolve: (*PreparedImageStoreClose).Commit, repeat: (*PreparedImageStoreClose).Abort, committed: true},
		{name: "abort", resolve: (*PreparedImageStoreClose).Abort, repeat: (*PreparedImageStoreClose).Commit},
	}
	mismatches := []struct {
		name   string
		mutate func(*Terminal) func()
	}{
		{name: "store", mutate: func(terminal *Terminal) func() {
			original := terminal.imageStore
			terminal.imageStore = nil
			return func() { terminal.imageStore = original }
		}},
		{name: "owner", mutate: func(terminal *Terminal) func() {
			original := terminal.imageOwner
			terminal.imageOwner = nil
			return func() { terminal.imageOwner = original }
		}},
		{name: "sidecars", mutate: func(terminal *Terminal) func() {
			original := terminal.imageSidecars
			terminal.imageSidecars = nil
			return func() { terminal.imageSidecars = original }
		}},
	}

	for _, resolution := range resolutions {
		for _, mismatch := range mismatches {
			t.Run(resolution.name+"/"+mismatch.name, func(t *testing.T) {
				store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
				terminal := newImageTerminalForTest(4, 2, 0, store)
				prepared, err := terminal.PrepareCloseImageStore()
				if err != nil {
					t.Fatal(err)
				}
				originalStore, originalOwner, originalSidecars := terminal.imageStore, terminal.imageOwner, terminal.imageSidecars
				restore := mismatch.mutate(terminal)
				before := captureImageCloseFingerprint(terminal)

				if err := resolution.resolve(prepared); !errors.Is(err, termimage.ErrPreparedState) {
					t.Fatalf("mismatched %s=%v", resolution.name, err)
				}
				if prepared.finished {
					t.Fatal("rejected resolution was marked finished")
				}
				if store.Closed() {
					t.Fatal("rejected resolution closed the store")
				}
				if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
					t.Fatalf("rejected resolution changed state: before=%#v after=%#v", before, after)
				}

				restore()
				if err := terminal.ResetImages(); !errors.Is(err, termimage.ErrOwnerBusy) {
					t.Fatalf("rejected resolution did not retain retryable close gate: %v", err)
				}
				if err := resolution.resolve(prepared); err != nil {
					t.Fatalf("retry %s=%v", resolution.name, err)
				}
				if !prepared.finished {
					t.Fatal("successful resolution was not marked finished")
				}
				if err := resolution.repeat(prepared); err != nil {
					t.Fatalf("repeated opposite resolution=%v", err)
				}

				if resolution.committed {
					if !store.Closed() || terminal.imageStore != nil || terminal.imageOwner != nil || terminal.imageSidecars != nil {
						t.Fatal("committed resolution did not detach the exact terminal image state")
					}
					return
				}
				if store.Closed() || terminal.imageStore != originalStore || terminal.imageOwner != originalOwner || terminal.imageSidecars != originalSidecars {
					t.Fatal("aborted resolution did not remain sticky")
				}
				if err := terminal.CloseImageStore(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestPreparedImageStoreCloseSerializesInnerCommitThroughOuterDetach(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	before := captureImageCloseFingerprint(terminal)

	type resolutionResult struct {
		operation string
		err       error
	}
	wrongThreadResults := make(chan resolutionResult, 2)
	startWrongThread := make(chan struct{})
	go func() {
		<-startWrongThread
		wrongThreadResults <- resolutionResult{operation: "commit", err: prepared.Commit()}
	}()
	go func() {
		<-startWrongThread
		wrongThreadResults <- resolutionResult{operation: "abort", err: prepared.Abort()}
	}()
	close(startWrongThread)
	for range 2 {
		result := <-wrongThreadResults
		if !errors.Is(result.err, termimage.ErrWrongOwnerThread) {
			t.Fatalf("wrong-thread %s=%v", result.operation, result.err)
		}
	}
	if prepared.finished {
		t.Fatal("wrong-thread resolution was marked finished")
	}
	if after := captureImageCloseFingerprint(terminal); !reflect.DeepEqual(before, after) {
		t.Fatalf("wrong-thread resolution changed state: before=%#v after=%#v", before, after)
	}
	if err := terminal.ResetImages(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("wrong-thread failures did not preserve retryability: %v", err)
	}

	windowResults := make(chan resolutionResult, 2)
	prepared.afterStoreCommit = func() {
		if !store.Closed() {
			t.Error("inner store commit seam ran before the store closed")
		}
		if terminal.imageStore != before.store || terminal.imageOwner != before.owner || terminal.imageSidecars != before.sidecars {
			t.Error("terminal detached before the inner commit seam")
		}
		if prepared.finished {
			t.Error("outer transaction finished before terminal detach")
		}
		if prepared.resolveMu.TryLock() {
			prepared.resolveMu.Unlock()
			t.Error("outer resolution lock did not span inner commit through terminal detach")
		}

		started := make(chan struct{}, 2)
		release := make(chan struct{})
		go func() {
			started <- struct{}{}
			<-release
			windowResults <- resolutionResult{operation: "commit", err: prepared.Commit()}
		}()
		go func() {
			started <- struct{}{}
			<-release
			windowResults <- resolutionResult{operation: "abort", err: prepared.Abort()}
		}()
		<-started
		<-started
		close(release)
	}

	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result := <-windowResults
		if result.err != nil {
			t.Fatalf("concurrent sticky %s=%v", result.operation, result.err)
		}
	}
	if !prepared.finished || !store.Closed() || terminal.imageStore != nil || terminal.imageOwner != nil || terminal.imageSidecars != nil {
		t.Fatal("successful outer commit did not finish with terminal state detached")
	}
	if err := prepared.Commit(); err != nil {
		t.Fatalf("repeated commit=%v", err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatalf("repeated abort=%v", err)
	}
}
