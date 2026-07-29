package core

import (
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"
	"time"

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
		conflict  func(*PreparedImageStoreClose) error
		outcome   preparedImageStoreCloseOutcome
		committed bool
	}{
		{
			name: "commit", resolve: (*PreparedImageStoreClose).Commit,
			repeat: (*PreparedImageStoreClose).Commit, conflict: (*PreparedImageStoreClose).Abort,
			outcome: preparedImageStoreCloseCommitted, committed: true,
		},
		{
			name: "abort", resolve: (*PreparedImageStoreClose).Abort,
			repeat: (*PreparedImageStoreClose).Abort, conflict: (*PreparedImageStoreClose).Commit,
			outcome: preparedImageStoreCloseAborted,
		},
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
				if prepared.resolution.outcome != preparedImageStoreClosePending {
					t.Fatal("rejected resolution consumed the shared outcome")
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
				if prepared.resolution.outcome != resolution.outcome {
					t.Fatalf("successful resolution outcome=%v", prepared.resolution.outcome)
				}
				if err := resolution.repeat(prepared); err != nil {
					t.Fatalf("repeated %s=%v", resolution.name, err)
				}
				if err := resolution.conflict(prepared); !errors.Is(err, termimage.ErrPreparedState) {
					t.Fatalf("conflicting resolution=%v", err)
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

func TestPreparedImageStoreCloseCopiesSerializeInnerCommitThroughOuterDetach(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	before := captureImageCloseFingerprint(terminal)
	repeatedCommit := *prepared
	conflictingAbort := *prepared
	if repeatedCommit.resolution != prepared.resolution || conflictingAbort.resolution != prepared.resolution {
		t.Fatal("value copies did not retain shared resolution state")
	}

	type resolutionResult struct {
		operation string
		err       error
	}
	windowResults := make(chan resolutionResult, 2)
	prepared.resolution.afterStoreCommit = func() {
		if !store.Closed() {
			t.Error("inner store commit seam ran before the store closed")
		}
		if terminal.imageStore != before.store || terminal.imageOwner != before.owner || terminal.imageSidecars != before.sidecars {
			t.Error("terminal detached before the inner commit seam")
		}
		if prepared.resolution.outcome != preparedImageStoreClosePending {
			t.Error("outer transaction resolved before terminal detach")
		}
		if prepared.resolution.mu.TryLock() {
			prepared.resolution.mu.Unlock()
			t.Error("shared resolution lock did not span inner commit through terminal detach")
		}

		started := make(chan struct{}, 2)
		release := make(chan struct{})
		go func() {
			started <- struct{}{}
			<-release
			windowResults <- resolutionResult{operation: "commit", err: repeatedCommit.Commit()}
		}()
		go func() {
			started <- struct{}{}
			<-release
			windowResults <- resolutionResult{operation: "abort", err: conflictingAbort.Abort()}
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
		switch result.operation {
		case "commit":
			if result.err != nil {
				t.Fatalf("concurrent copied commit=%v", result.err)
			}
		case "abort":
			if !errors.Is(result.err, termimage.ErrPreparedState) {
				t.Fatalf("concurrent copied abort=%v", result.err)
			}
		}
	}
	if prepared.resolution.outcome != preparedImageStoreCloseCommitted || !store.Closed() || terminal.imageStore != nil || terminal.imageOwner != nil || terminal.imageSidecars != nil {
		t.Fatal("successful outer commit did not finish with terminal state detached")
	}
	if err := repeatedCommit.Commit(); err != nil {
		t.Fatalf("repeated copied commit=%v", err)
	}
	if err := conflictingAbort.Abort(); !errors.Is(err, termimage.ErrPreparedState) {
		t.Fatalf("repeated copied abort=%v", err)
	}
}

func TestPreparedImageStoreCloseCopiesShareAbortOutcomeConcurrently(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}

	type resolutionResult struct {
		commit bool
		err    error
	}
	const copies = 16
	start := make(chan struct{})
	results := make(chan resolutionResult, copies)
	for i := range copies {
		copy := *prepared
		go func(commit bool) {
			<-start
			if commit {
				results <- resolutionResult{commit: true, err: copy.Commit()}
				return
			}
			results <- resolutionResult{err: copy.Abort()}
		}(i%2 == 0)
	}
	close(start)
	for range copies {
		result := <-results
		if result.commit {
			if !errors.Is(result.err, termimage.ErrPreparedState) {
				t.Fatalf("copied commit after abort=%v", result.err)
			}
		} else if result.err != nil {
			t.Fatalf("copied abort repeat=%v", result.err)
		}
	}
	if prepared.resolution.outcome != preparedImageStoreCloseAborted || store.Closed() {
		t.Fatal("copied calls changed the shared abort outcome")
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedImageStoreCloseWrongThreadRaceDoesNotReadTerminalState(t *testing.T) {
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	terminal := newImageTerminalForTest(4, 2, 0, store)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}

	type workerResult struct {
		operation string
		err       error
	}
	const workers = 8
	const callsPerWorker = 128
	start := make(chan struct{})
	results := make(chan workerResult, workers)
	for worker := range workers {
		copy := *prepared
		go func(commit bool) {
			<-start
			for range callsPerWorker {
				operation := "commit"
				var err error
				if commit {
					err = copy.Commit()
				} else {
					operation = "abort"
					err = copy.Abort()
				}
				if !errors.Is(err, termimage.ErrWrongOwnerThread) {
					results <- workerResult{operation: operation, err: err}
					return
				}
			}
			results <- workerResult{}
		}(worker%2 == 0)
	}
	close(start)

	completed := 0
	var failures []workerResult
	for completed < workers {
		select {
		case result := <-results:
			completed++
			if result.operation != "" {
				failures = append(failures, result)
			}
		default:
			terminal.imageStore = nil
			runtime.Gosched()
			terminal.imageStore = store
			runtime.Gosched()
		}
	}
	terminal.imageStore = store
	for _, failure := range failures {
		t.Errorf("wrong-thread copied %s=%v", failure.operation, failure.err)
	}
	if len(failures) != 0 {
		t.FailNow()
	}
	if prepared.resolution.outcome != preparedImageStoreClosePending {
		t.Fatal("wrong-thread validation consumed the shared outcome")
	}
	if err := terminal.ResetImages(); !errors.Is(err, termimage.ErrOwnerBusy) {
		t.Fatalf("wrong-thread racing failures did not preserve retryability: %v", err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedImageStoreCloseNoStoreCopiesResolveAtUnchangedPublication(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(*PreparedImageStoreClose) error
		outcome preparedImageStoreCloseOutcome
	}{
		{name: "commit", resolve: (*PreparedImageStoreClose).Commit, outcome: preparedImageStoreCloseCommitted},
		{name: "abort", resolve: (*PreparedImageStoreClose).Abort, outcome: preparedImageStoreCloseAborted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal := NewTerminal(4, 2)
			prepared, err := terminal.PrepareCloseImageStore()
			if err != nil {
				t.Fatal(err)
			}
			if prepared.store != nil {
				t.Fatal("no-store preflight captured a store")
			}

			const copies = 16
			start := make(chan struct{})
			results := make(chan error, copies)
			for range copies {
				copy := *prepared
				go func() {
					<-start
					results <- test.resolve(&copy)
				}()
			}
			close(start)
			for range copies {
				if err := <-results; err != nil {
					t.Fatalf("copied no-store %s=%v", test.name, err)
				}
			}
			if prepared.resolution.outcome != test.outcome {
				t.Fatalf("shared outcome=%v", prepared.resolution.outcome)
			}
			if generation := terminal.imagePublicationGeneration.Load(); generation != prepared.publicationGeneration {
				t.Fatalf("unchanged publication generation=%d want %d", generation, prepared.publicationGeneration)
			}
		})
	}
}

func TestPreparedImageStoreCloseNoStoreCopiesRejectAttachPublicationUnderRace(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(*PreparedImageStoreClose) error
	}{
		{name: "commit", resolve: (*PreparedImageStoreClose).Commit},
		{name: "abort", resolve: (*PreparedImageStoreClose).Abort},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal := NewTerminal(4, 2)
			prepared, err := terminal.PrepareCloseImageStore()
			if err != nil {
				t.Fatal(err)
			}

			const copies = 16
			start := make(chan struct{})
			results := make(chan error, copies)
			for range copies {
				copy := *prepared
				go func() {
					<-start
					results <- test.resolve(&copy)
				}()
			}

			store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
			attached := make(chan error, 1)
			releaseClose := make(chan struct{})
			closed := make(chan error, 1)
			go func() {
				attachErr := terminal.AttachImageStore(store)
				attached <- attachErr
				if attachErr != nil {
					closed <- nil
					return
				}
				<-releaseClose
				closed <- terminal.CloseImageStore()
			}()

			deadline := time.Now().Add(5 * time.Second)
			for terminal.imagePublicationGeneration.Load() == prepared.publicationGeneration && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			published := terminal.imagePublicationGeneration.Load() != prepared.publicationGeneration
			close(start)
			var resolutionErrors []error
			for range copies {
				resolutionErrors = append(resolutionErrors, <-results)
			}
			attachErr := <-attached
			close(releaseClose)
			closeErr := <-closed

			if !published {
				t.Fatal("attach did not publish an image generation before the deadline")
			}
			if attachErr != nil {
				t.Fatalf("attach=%v", attachErr)
			}
			for _, err := range resolutionErrors {
				if !errors.Is(err, termimage.ErrPreparedState) {
					t.Fatalf("copied stale no-store %s=%v", test.name, err)
				}
			}
			if prepared.resolution.outcome != preparedImageStoreClosePending {
				t.Fatal("stale no-store resolution consumed the shared outcome")
			}
			if closeErr != nil {
				t.Fatalf("close after attach=%v", closeErr)
			}
			if generation := terminal.imagePublicationGeneration.Load(); generation != prepared.publicationGeneration+2 {
				t.Fatalf("attach/detach publication generation=%d want %d", generation, prepared.publicationGeneration+2)
			}
		})
	}
}

func TestImageStorePublicationGenerationAdvancesOnlyOnAttachAndDetach(t *testing.T) {
	terminal := NewTerminal(4, 2)
	prepared, err := terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 0 {
		t.Fatalf("no-store abort generation=%d", generation)
	}
	if err := terminal.AttachImageStore(nil); !errors.Is(err, ErrImageStoreUnavailable) {
		t.Fatalf("nil attach=%v", err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 0 {
		t.Fatalf("failed attach generation=%d", generation)
	}

	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 1 {
		t.Fatalf("successful attach generation=%d", generation)
	}
	prepared, err = terminal.PrepareCloseImageStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 1 {
		t.Fatalf("attached abort generation=%d", generation)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 2 {
		t.Fatalf("successful detach generation=%d", generation)
	}
	if err := terminal.CloseImageStore(); err != nil {
		t.Fatal(err)
	}
	if generation := terminal.imagePublicationGeneration.Load(); generation != 2 {
		t.Fatalf("idempotent no-store close generation=%d", generation)
	}
}

func TestImageStorePublicationGenerationExhaustionFailsBeforePublication(t *testing.T) {
	t.Run("attach", func(t *testing.T) {
		terminal := NewTerminal(4, 2)
		terminal.imagePublicationGeneration.Store(math.MaxUint64)
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		if err := terminal.AttachImageStore(store); !errors.Is(err, termimage.ErrGenerationExhausted) {
			t.Fatalf("exhausted attach=%v", err)
		}
		if terminal.imageStore != nil || terminal.imageOwner != nil || terminal.imageSidecars != nil || terminal.imagePublicationGeneration.Load() != math.MaxUint64 {
			t.Fatal("exhausted attach partially published terminal image state")
		}
		owner := store.ClaimOwner()
		if owner == nil {
			t.Fatal("exhausted attach irreversibly claimed the store owner")
		}
		if err := owner.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("detach", func(t *testing.T) {
		store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
		terminal := newImageTerminalForTest(4, 2, 0, store)
		terminal.imagePublicationGeneration.Store(math.MaxUint64)
		prepared, err := terminal.PrepareCloseImageStore()
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.Commit(); !errors.Is(err, termimage.ErrGenerationExhausted) {
			t.Fatalf("exhausted detach=%v", err)
		}
		if store.Closed() || terminal.imageStore != store || terminal.imageOwner == nil || terminal.imageSidecars == nil || terminal.imagePublicationGeneration.Load() != math.MaxUint64 {
			t.Fatal("exhausted detach crossed the irreversible close boundary")
		}
		if prepared.resolution.outcome != preparedImageStoreClosePending {
			t.Fatal("exhausted detach consumed the shared outcome")
		}
		if err := prepared.Abort(); err != nil {
			t.Fatal(err)
		}
		terminal.imagePublicationGeneration.Store(math.MaxUint64 - 1)
		if err := terminal.CloseImageStore(); err != nil {
			t.Fatal(err)
		}
		if generation := terminal.imagePublicationGeneration.Load(); generation != math.MaxUint64 {
			t.Fatalf("last successful detach generation=%d", generation)
		}
	})
}
