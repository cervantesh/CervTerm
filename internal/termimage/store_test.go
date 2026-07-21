package termimage

import (
	"bytes"
	"math"
	"sync"
	"testing"
	"time"
)

func TestTransferAppendCloseAndDuplicateLifecycle(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginTransfer(Header{Transfer: 1, Image: 3}); err != ErrDuplicateTransfer {
		t.Fatalf("duplicate error = %v", err)
	}
	input := []byte("payload")
	if err := transfer.Append(input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	copyOfEncoded, err := transfer.EncodedCopy()
	if err != nil || string(copyOfEncoded) != "payload" {
		t.Fatalf("encoded copy = %q, %v", copyOfEncoded, err)
	}
	copyOfEncoded[0] = 'Y'
	again, _ := transfer.EncodedCopy()
	if string(again) != "payload" {
		t.Fatalf("encoded storage aliased detached copy: %q", again)
	}
	transfer.Close()
	transfer.Close()
	if store.Usage() != (Usage{}) || process.Usage() != (Usage{}) {
		t.Fatalf("transfer leaked usage store=%#v process=%#v", store.Usage(), process.Usage())
	}
	if _, err := store.BeginTransfer(Header{Transfer: 1, Image: 3}); err != nil {
		t.Fatalf("closed transfer ID was not reusable: %v", err)
	}
}

func TestTransferPaneAndProcessCaps(t *testing.T) {
	process := NewProcessBudget()
	stores := make([]*Store, 5)
	var transfers []*CandidateTransfer
	var id TransferID = 1
	for i := range stores {
		stores[i] = NewStore(process, DefaultLimits())
		count := int(HardPendingTransfersPerPane)
		if i == 4 {
			count = 1
		}
		for j := 0; j < count; j++ {
			transfer, err := stores[i].BeginTransfer(Header{Transfer: id, Image: ImageID(id)})
			id++
			if i == 4 {
				if err == nil {
					t.Fatal("process transfer cap exceeded")
				}
				break
			}
			if err != nil {
				t.Fatalf("store %d transfer %d: %v", i, j, err)
			}
			transfers = append(transfers, transfer)
		}
	}
	for _, transfer := range transfers {
		transfer.Close()
	}
	if process.Usage() != (Usage{}) {
		t.Fatalf("process usage leaked: %#v", process.Usage())
	}
}

func TestTransferEncodedCapRejectsBeforeRetention(t *testing.T) {
	limits := DefaultLimits()
	limits.EncodedBytes = HardControlChunkBytes
	store := NewStore(NewProcessBudget(), limits)
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{'x'}, int(HardControlChunkBytes))
	if err := transfer.Append(chunk); err != nil {
		t.Fatal(err)
	}
	before, _ := transfer.EncodedCopy()
	if err := transfer.Append([]byte{'y'}); err != ErrLimitExceeded {
		t.Fatalf("over-cap error = %v", err)
	}
	after, _ := transfer.EncodedCopy()
	if !bytes.Equal(before, after) || store.Usage().EncodedBytes != HardControlChunkBytes {
		t.Fatal("rejected append retained bytes or changed usage")
	}
	transfer.Close()
}

func TestTransferChunkCapRejectsBeforeRetention(t *testing.T) {
	store := NewStore(NewProcessBudget(), DefaultLimits())
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < HardChunksPerTransfer; i++ {
		if err := transfer.Append([]byte{'x'}); err != nil {
			t.Fatalf("chunk %d: %v", i, err)
		}
	}
	before := store.Usage()
	if err := transfer.Append([]byte{'y'}); err != ErrTooManyChunks {
		t.Fatalf("chunk cap error = %v", err)
	}
	after := store.Usage()
	if before != after || after.EncodedBytes != HardChunksPerTransfer {
		t.Fatalf("chunk rejection changed usage before=%#v after=%#v", before, after)
	}
	transfer.Close()
}

func TestTransferExpiryResetCloseAndLateConcurrentReturn(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	now := time.Unix(1, 0)
	store.now = func() time.Time { return now }
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Append([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(HardTransferLifetime)
	if _, err := transfer.EncodedCopy(); err != ErrTransferExpired {
		t.Fatalf("expiry error = %v", err)
	}
	if !transfer.Closed() || store.Usage() != (Usage{}) {
		t.Fatal("expired transfer retained reservations")
	}

	transfer, err = store.BeginTransfer(Header{Transfer: 2, Image: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Append([]byte("held")); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); transfer.Close() }()
	store.Reset()
	group.Wait()
	if store.Epoch() != 2 || process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
		t.Fatalf("reset state epoch=%d process=%#v pane=%#v", store.Epoch(), process.Usage(), store.Usage())
	}
	store.Close()
	store.Close()
	if _, err := store.BeginTransfer(Header{Transfer: 3, Image: 3}); err != ErrClosed {
		t.Fatalf("closed begin error = %v", err)
	}
}

type fakeTimer struct{ stopped bool }

func (t *fakeTimer) Stop() bool {
	wasRunning := !t.stopped
	t.stopped = true
	return wasRunning
}

func TestTransferTimerAutonomouslyClosesAndRemovesPending(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	var expire func()
	store.after = func(_ time.Duration, callback func()) timerStopper {
		expire = callback
		return &fakeTimer{}
	}
	transfer, err := store.BeginTransfer(Header{Transfer: 1, Image: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Append([]byte("held")); err != nil {
		t.Fatal(err)
	}
	expire()
	if !transfer.Closed() || process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
		t.Fatalf("timer close leaked process=%#v pane=%#v", process.Usage(), store.Usage())
	}
	store.pendingMu.Lock()
	pending := len(store.pending)
	store.pendingMu.Unlock()
	if pending != 0 {
		t.Fatalf("closed transfer remained pending: %d", pending)
	}
	for id := TransferID(2); id < 1000; id++ {
		candidate, err := store.BeginTransfer(Header{Transfer: id, Image: ImageID(id)})
		if err != nil {
			t.Fatalf("unique transfer %d: %v", id, err)
		}
		candidate.Close()
	}
	store.pendingMu.Lock()
	pending = len(store.pending)
	store.pendingMu.Unlock()
	if pending != 0 || process.Usage() != (Usage{}) {
		t.Fatalf("unique closes retained map=%d usage=%#v", pending, process.Usage())
	}
}

func TestAcquireIsGenerationCheckedAndDetached(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	lease, err := reserve(process, &store.pane, Usage{DecodedBytes: 4, Images: 1})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.prepareNextRef(7)
	if err != nil {
		t.Fatal(err)
	}
	if store.state.nextGeneration != 0 || !store.consumePreparedRef(ref) {
		t.Fatal("prepared generation mutated early or failed publication consumption")
	}
	store.state.resources[7] = &resource{ref: ref, width: 1, height: 1, stride: 4, rgba: []byte{1, 2, 3, 4}, lease: lease}
	if _, ok := store.Acquire(ResourceRef{Image: 7, Generation: ref.Generation + 1}); ok {
		t.Fatal("stale generation acquired")
	}
	first, ok := store.Acquire(ref)
	if !ok {
		t.Fatal("resource not acquired")
	}
	first.RGBA[0] = 9
	second, ok := store.Acquire(ref)
	if !ok || second.RGBA[0] != 1 {
		t.Fatalf("acquire returned alias: %#v", second.RGBA)
	}
	store.Reset()
	if _, ok := store.Acquire(ref); ok || process.Usage() != (Usage{}) {
		t.Fatal("reset retained stale resource or reservation")
	}
	newRef, err := store.prepareNextRef(7)
	if err != nil || newRef.Generation == ref.Generation {
		t.Fatalf("image reuse generation = %#v, %v", newRef, err)
	}
}

func TestDecodedCandidateReservesBeforeAllocationAndReturnsLeaseOnce(t *testing.T) {
	process := NewProcessBudget()
	store := NewStore(process, DefaultLimits())
	candidate, err := store.NewDecodedCandidate(9, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	width, height, stride := candidate.Dimensions()
	if width != 2 || height != 3 || stride != 8 || len(candidate.RGBA()) != 24 || !candidate.ValidFor(store) {
		t.Fatalf("candidate dimensions/state = %d,%d,%d len=%d valid=%v", width, height, stride, len(candidate.RGBA()), candidate.ValidFor(store))
	}
	if got := store.Usage(); got.DecodedBytes != 24 || got.Images != 1 || process.Usage() != got {
		t.Fatalf("candidate usage pane=%#v process=%#v", got, process.Usage())
	}
	store.Reset()
	if candidate.ValidFor(store) {
		t.Fatal("pre-reset candidate remained valid")
	}
	if got := process.Usage(); got.DecodedBytes != 24 || got.Images != 1 {
		t.Fatalf("reset released worker-owned pixels early: %#v", got)
	}
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() { defer group.Done(); candidate.Close() }()
	}
	group.Wait()
	if candidate.RGBA() != nil || process.Usage() != (Usage{}) || store.Usage() != (Usage{}) {
		t.Fatalf("candidate close leaked process=%#v pane=%#v", process.Usage(), store.Usage())
	}
	if _, err := store.NewDecodedCandidate(1, 4097, 1); err == nil || process.Usage() != (Usage{}) {
		t.Fatal("invalid candidate dimensions retained reservation")
	}
}

func TestStoreIdentityAndCounterExhaustion(t *testing.T) {
	process := NewProcessBudget()
	first := NewStore(process, DefaultLimits())
	second := NewStore(process, DefaultLimits())
	ref1, _ := first.prepareNextRef(1)
	ref2, _ := second.prepareNextRef(1)
	if ref1 != ref2 {
		t.Fatalf("pane-local numeric identities may match: %#v %#v", ref1, ref2)
	}
	first.state.nextGeneration = ResourceGeneration(math.MaxUint64)
	if _, err := first.prepareNextRef(1); err != ErrGenerationExhausted {
		t.Fatalf("generation exhaustion error = %v", err)
	}
	first.epoch.Store(math.MaxUint64)
	first.Reset()
	if !first.closed.Load() || first.Epoch() != StoreEpoch(math.MaxUint64) {
		t.Fatal("epoch wrapped instead of permanently closing")
	}
}

func TestNewStoreRejectsInvalidInputs(t *testing.T) {
	if NewStore(nil, DefaultLimits()) != nil || NewStore(NewProcessBudget(), Limits{}) != nil {
		t.Fatal("invalid store inputs accepted")
	}
	store := NewStore(NewProcessBudget(), DefaultLimits())
	if _, err := store.BeginTransfer(Header{}); err != ErrInvalidID {
		t.Fatalf("zero IDs error = %v", err)
	}
}
