package mux

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cervterm/internal/kitty"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

type decodeSchedulerTestJob struct {
	started chan<- struct{}
	release <-chan struct{}
	active  *atomic.Int32
	maximum *atomic.Int32
	runs    atomic.Int32
	closes  atomic.Int32
}

func (j *decodeSchedulerTestJob) Run(ctx context.Context) *kitty.DecodeResult {
	j.runs.Add(1)
	if j.active != nil {
		active := j.active.Add(1)
		updateAtomicMaximum(j.maximum, active)
		defer j.active.Add(-1)
	}
	if j.started != nil {
		j.started <- struct{}{}
	}
	if j.release != nil {
		select {
		case <-j.release:
		case <-ctx.Done():
			return &kitty.DecodeResult{Failure: kitty.ReplyCancelled}
		}
	}
	return &kitty.DecodeResult{Failure: kitty.ReplyFailed}
}

func (j *decodeSchedulerTestJob) Close() { j.closes.Add(1) }

func updateAtomicMaximum(maximum *atomic.Int32, value int32) {
	if maximum == nil {
		return
	}
	for current := maximum.Load(); value > current; current = maximum.Load() {
		if maximum.CompareAndSwap(current, value) {
			return
		}
	}
}

func awaitSchedulerSignals(t *testing.T, signals <-chan struct{}, count int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for index := 0; index < count; index++ {
		select {
		case <-signals:
		case <-deadline.C:
			t.Fatalf("received %d of %d scheduler signals", index, count)
		}
	}
}

func awaitSchedulerCompletion(t *testing.T, scheduler *kittyDecodeScheduler) kittyDecodeCompletion {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-scheduler.ready():
			if completion, ok := scheduler.takeCompletion(); ok {
				kittyCompletion, valid := decodeKittyCompletion(completion)
				if !valid {
					completion.Close()
					scheduler.finish(completion.Key)
					t.Fatal("non-Kitty completion in Kitty scheduler test")
				}
				return kittyCompletion
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for scheduler completion")
			return kittyDecodeCompletion{}
		}
	}
}

func TestKittyDecodeSchedulerBoundsProcessConcurrency(t *testing.T) {
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	var active, maximum atomic.Int32
	scheduler := newKittyDecodeScheduler(nil)
	defer scheduler.close()

	jobs := make([]*decodeSchedulerTestJob, 8)
	for index := range jobs {
		jobs[index] = &decodeSchedulerTestJob{started: started, release: release, active: &active, maximum: &maximum}
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: PaneID(index + 1)}, job: jobs[index]}); err != nil {
			t.Fatalf("submit %d: %v", index, err)
		}
	}
	awaitSchedulerSignals(t, started, kittyDecodeWorkerCount)
	select {
	case <-started:
		t.Fatal("more than two decode jobs started while both workers were blocked")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range jobs {
		completion := awaitSchedulerCompletion(t, scheduler)
		completion.Close()
	}
	if got := maximum.Load(); got != int32(kittyDecodeWorkerCount) {
		t.Fatalf("maximum concurrent jobs = %d, want %d", got, kittyDecodeWorkerCount)
	}
	for index, job := range jobs {
		if job.runs.Load() != 1 || job.closes.Load() != 0 {
			t.Fatalf("job %d runs=%d closes=%d", index, job.runs.Load(), job.closes.Load())
		}
	}
}

func TestKittyDecodeSchedulerRejectsDuplicatePane(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	scheduler := newKittyDecodeScheduler(nil)
	defer scheduler.close()
	first := &decodeSchedulerTestJob{started: started, release: release}
	duplicate := &decodeSchedulerTestJob{}

	if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 7}, job: first}); err != nil {
		t.Fatal(err)
	}
	awaitSchedulerSignals(t, started, 1)
	if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 7}, job: duplicate}); !errors.Is(err, errKittyDecodePaneActive) {
		t.Fatalf("duplicate error = %v", err)
	}
	if duplicate.runs.Load() != 0 || duplicate.closes.Load() != 1 {
		t.Fatalf("duplicate runs=%d closes=%d", duplicate.runs.Load(), duplicate.closes.Load())
	}
	close(release)
	completion := awaitSchedulerCompletion(t, scheduler)
	completion.Close()
}

func TestKittyDecodeSchedulerRejectsSaturatedQueue(t *testing.T) {
	started := make(chan struct{}, kittyDecodeWorkerCount)
	block := make(chan struct{})
	scheduler := newKittyDecodeScheduler(nil)
	blockers := make([]*decodeSchedulerTestJob, kittyDecodeWorkerCount)
	for index := range blockers {
		blockers[index] = &decodeSchedulerTestJob{started: started, release: block}
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: PaneID(index + 1)}, job: blockers[index]}); err != nil {
			t.Fatal(err)
		}
	}
	awaitSchedulerSignals(t, started, kittyDecodeWorkerCount)

	queued := make([]*decodeSchedulerTestJob, kittyDecodeQueueCapacity)
	for index := range queued {
		queued[index] = &decodeSchedulerTestJob{}
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: PaneID(index + 100)}, job: queued[index]}); err != nil {
			t.Fatalf("queue slot %d: %v", index, err)
		}
	}
	overflow := &decodeSchedulerTestJob{}
	if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 999}, job: overflow}); !errors.Is(err, errKittyDecodeQueueFull) {
		t.Fatalf("saturation error = %v", err)
	}
	if overflow.runs.Load() != 0 || overflow.closes.Load() != 1 {
		t.Fatalf("overflow runs=%d closes=%d", overflow.runs.Load(), overflow.closes.Load())
	}

	scheduler.close()
	for index, job := range queued {
		if job.runs.Load() != 0 || job.closes.Load() != 1 {
			t.Fatalf("queued job %d runs=%d closes=%d", index, job.runs.Load(), job.closes.Load())
		}
	}
	for index, job := range blockers {
		if job.runs.Load() != 1 || job.closes.Load() != 0 {
			t.Fatalf("started job %d runs=%d closes=%d", index, job.runs.Load(), job.closes.Load())
		}
	}
}

func TestKittyDecodeSchedulerRetainsOwnerMetadataAndWakes(t *testing.T) {
	wakes := make(chan struct{}, 1)
	scheduler := newKittyDecodeScheduler(func() { wakes <- struct{}{} })
	defer scheduler.close()
	paneToken := &pane{id: 11}
	slot := replySlot{pane: 11, sequence: 1}
	owner := kittyDecodeOwner{paneID: 11, pane: paneToken, generation: 42, replySlot: slot, hasSlot: true}
	job := &decodeSchedulerTestJob{}
	if err := scheduler.submit(kittyDecodeWork{owner: owner, job: job}); err != nil {
		t.Fatal(err)
	}
	completion := awaitSchedulerCompletion(t, scheduler)
	defer completion.Close()
	awaitSchedulerSignals(t, wakes, 1)
	if completion.Owner.paneID != owner.paneID || completion.Owner.pane != paneToken || completion.Owner.generation != 42 ||
		completion.Owner.replySlot != slot || !completion.Owner.hasSlot {
		t.Fatalf("completion owner = %#v, want %#v", completion.Owner, owner)
	}
	if job.runs.Load() != 1 {
		t.Fatalf("runs = %d", job.runs.Load())
	}
}

func TestKittyDecodeSchedulerCloseSubmitRaceCleansEveryJobExactlyOnce(t *testing.T) {
	const jobCount = 128
	scheduler := newKittyDecodeScheduler(nil)
	jobs := make([]*decodeSchedulerTestJob, jobCount)
	start := make(chan struct{})
	var submitters sync.WaitGroup
	for index := range jobs {
		jobs[index] = &decodeSchedulerTestJob{release: make(chan struct{})}
		submitters.Add(1)
		go func(index int) {
			defer submitters.Done()
			<-start
			_ = scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: PaneID(index + 1)}, job: jobs[index]})
		}(index)
	}
	closed := make(chan struct{})
	go func() {
		<-start
		scheduler.close()
		close(closed)
	}()
	close(start)
	submitters.Wait()
	<-closed
	scheduler.close()

	for index, job := range jobs {
		runs, closes := job.runs.Load(), job.closes.Load()
		if runs+closes != 1 {
			t.Fatalf("job %d runs=%d closes=%d", index, runs, closes)
		}
	}
	if _, ok := scheduler.takeCompletion(); ok {
		t.Fatal("completion remains after close")
	}
}

func TestKittyDecodeSchedulerReleasesTransferAndCandidateOwnership(t *testing.T) {
	t.Run("rejected duplicate", func(t *testing.T) {
		started := make(chan struct{}, 1)
		block := make(chan struct{})
		scheduler := newKittyDecodeScheduler(nil)
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}, job: &decodeSchedulerTestJob{started: started, release: block}}); err != nil {
			t.Fatal(err)
		}
		awaitSchedulerSignals(t, started, 1)
		process, store, job := newOwnedSchedulerDecodeJob(t, 1)
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}, job: job}); !errors.Is(err, errKittyDecodePaneActive) {
			t.Fatalf("duplicate error = %v", err)
		}
		assertNoImageOwnership(t, process, store)
		scheduler.close()
	})

	t.Run("rejected closed", func(t *testing.T) {
		scheduler := newKittyDecodeScheduler(nil)
		scheduler.close()
		process, store, job := newOwnedSchedulerDecodeJob(t, 2)
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 2}, job: job}); !errors.Is(err, errKittyDecodeSchedulerClosed) {
			t.Fatalf("closed error = %v", err)
		}
		assertNoImageOwnership(t, process, store)
	})

	t.Run("queued on close", func(t *testing.T) {
		started := make(chan struct{}, kittyDecodeWorkerCount)
		block := make(chan struct{})
		scheduler := newKittyDecodeScheduler(nil)
		for index := 0; index < kittyDecodeWorkerCount; index++ {
			if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: PaneID(index + 1)}, job: &decodeSchedulerTestJob{started: started, release: block}}); err != nil {
				t.Fatal(err)
			}
		}
		awaitSchedulerSignals(t, started, kittyDecodeWorkerCount)
		process, store, job := newOwnedSchedulerDecodeJob(t, 3)
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 3}, job: job}); err != nil {
			t.Fatal(err)
		}
		scheduler.close()
		assertNoImageOwnership(t, process, store)
	})

	t.Run("buffered result on close", func(t *testing.T) {
		wakes := make(chan struct{}, 1)
		scheduler := newKittyDecodeScheduler(func() { wakes <- struct{}{} })
		process, store, job := newOwnedSchedulerDecodeJob(t, 4)
		if err := scheduler.submit(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 4}, job: job}); err != nil {
			t.Fatal(err)
		}
		awaitSchedulerSignals(t, wakes, 1)
		scheduler.close()
		assertNoImageOwnership(t, process, store)
	})
}

func newOwnedSchedulerDecodeJob(t *testing.T, image termimage.ImageID) (*termimage.ProcessBudget, *termimage.Store, *kitty.DecodeJob) {
	t.Helper()
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	transfer, err := store.BeginTransfer(termimage.Header{Transfer: termimage.TransferID(image), Image: image})
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, base64.StdEncoding.EncodedLen(4))
	base64.StdEncoding.Encode(payload, []byte{1, 2, 3, 4})
	if err = transfer.Append(payload); err != nil {
		t.Fatal(err)
	}
	if err = transfer.Seal(); err != nil {
		t.Fatal(err)
	}
	job, code := kitty.NewDecodeJob(store, kitty.Command{
		Action:   kitty.ActionTransmit,
		Image:    image,
		Transfer: transfer,
		Decode:   kitty.DecodeSpec{Format: kitty.FormatRGBA32, Width: 1, Height: 1},
	})
	if code != kitty.ReplyNone {
		t.Fatalf("decode job failure = %v", code)
	}
	return process, store, job
}

func assertNoImageOwnership(t *testing.T, process *termimage.ProcessBudget, store *termimage.Store) {
	t.Helper()
	if got := process.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("process ownership leaked: %#v", got)
	}
	if got := store.Usage(); got != (termimage.Usage{}) {
		t.Fatalf("pane ownership leaked: %#v", got)
	}
}

type mismatchedImageResult struct{ closes atomic.Int32 }

func (r *mismatchedImageResult) Close() {
	if r != nil {
		r.closes.Add(1)
	}
}

func TestSharedImageSchedulerRejectsNilKittyJob(t *testing.T) {
	scheduler := newImageDecodeScheduler(nil)
	defer scheduler.close()
	if err := scheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}}); !errors.Is(err, errKittyDecodeInvalidWork) {
		t.Fatalf("nil job error=%v", err)
	}
}

func TestSharedImageSchedulerClosesMismatchedErasedResult(t *testing.T) {
	m, _, _ := newKittyRuntimeMux(t, true)
	result := &mismatchedImageResult{}
	completion := imageDecodeCompletion{Key: 1, Owner: imageDecodeOwner{protocol: imageDecodeKitty, value: kittyDecodeOwner{paneID: 1}}, Result: result}
	if events := m.applyImageCompletion(completion); len(events) != 0 {
		t.Fatalf("mismatched completion events=%#v", events)
	}
	if result.closes.Load() != 1 {
		t.Fatalf("mismatched result closes=%d", result.closes.Load())
	}
}

type sixelSchedulerTestJob struct {
	started chan<- struct{}
	release <-chan struct{}
	active  *atomic.Int32
	maximum *atomic.Int32
	runs    atomic.Int32
	closes  atomic.Int32
}

func (j *sixelSchedulerTestJob) Run(ctx context.Context) *sixel.DecodeResult {
	j.runs.Add(1)
	if j.active != nil {
		active := j.active.Add(1)
		updateAtomicMaximum(j.maximum, active)
		defer j.active.Add(-1)
	}
	if j.started != nil {
		j.started <- struct{}{}
	}
	if j.release != nil {
		select {
		case <-j.release:
		case <-ctx.Done():
			return &sixel.DecodeResult{Failure: sixel.FailureCancelled}
		}
	}
	return &sixel.DecodeResult{Failure: sixel.FailureFailed}
}

func (j *sixelSchedulerTestJob) Close() { j.closes.Add(1) }

func awaitImageSchedulerCompletion(t *testing.T, scheduler *imageDecodeScheduler) imageDecodeCompletion {
	t.Helper()
	select {
	case <-scheduler.ready():
		completion, ok := scheduler.takeCompletion()
		if !ok {
			t.Fatal("scheduler signaled without a completion")
		}
		return completion
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for image completion")
		return imageDecodeCompletion{}
	}
}

func TestSharedImageSchedulerUsesOnePaneKeyAcrossProtocols(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	scheduler := newImageDecodeScheduler(nil)
	defer scheduler.close()
	kittyJob := &decodeSchedulerTestJob{started: started, release: release}
	if err := scheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 7}, job: kittyJob}); err != nil {
		t.Fatal(err)
	}
	awaitSchedulerSignals(t, started, 1)
	sixelDuplicate := &sixelSchedulerTestJob{}
	if err := scheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: 7}, job: sixelDuplicate}); !errors.Is(err, errKittyDecodePaneActive) {
		t.Fatalf("cross-protocol duplicate error=%v", err)
	}
	if sixelDuplicate.runs.Load() != 0 || sixelDuplicate.closes.Load() != 1 {
		t.Fatalf("duplicate runs=%d closes=%d", sixelDuplicate.runs.Load(), sixelDuplicate.closes.Load())
	}
	close(release)
	completion := awaitImageSchedulerCompletion(t, scheduler)
	if completion.Owner.protocol != imageDecodeKitty {
		t.Fatalf("protocol=%d", completion.Owner.protocol)
	}
	completion.Close()
	scheduler.finish(completion.Key)

	sixelJob := &sixelSchedulerTestJob{}
	if err := scheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: 7}, job: sixelJob}); err != nil {
		t.Fatalf("pane key not released after Finish: %v", err)
	}
	completion = awaitImageSchedulerCompletion(t, scheduler)
	if completion.Owner.protocol != imageDecodeSixel {
		t.Fatalf("protocol=%d", completion.Owner.protocol)
	}
	completion.Close()
	scheduler.finish(completion.Key)
}

func TestSharedImageSchedulerBoundsMixedProtocolConcurrency(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var active, maximum atomic.Int32
	scheduler := newImageDecodeScheduler(nil)
	defer scheduler.close()
	kittyOne := &decodeSchedulerTestJob{started: started, release: release, active: &active, maximum: &maximum}
	sixelTwo := &sixelSchedulerTestJob{started: started, release: release, active: &active, maximum: &maximum}
	kittyThree := &decodeSchedulerTestJob{started: started, release: release, active: &active, maximum: &maximum}
	if err := scheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}, job: kittyOne}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: 2}, job: sixelTwo}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 3}, job: kittyThree}); err != nil {
		t.Fatal(err)
	}
	awaitSchedulerSignals(t, started, imageDecodeWorkerCount)
	select {
	case <-started:
		t.Fatal("mixed protocols exceeded the shared two-worker bound")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range 3 {
		completion := awaitImageSchedulerCompletion(t, scheduler)
		completion.Close()
		scheduler.finish(completion.Key)
	}
	if maximum.Load() != int32(imageDecodeWorkerCount) {
		t.Fatalf("maximum=%d want=%d", maximum.Load(), imageDecodeWorkerCount)
	}
	if kittyOne.runs.Load() != 1 || sixelTwo.runs.Load() != 1 || kittyThree.runs.Load() != 1 {
		t.Fatalf("runs kitty1=%d sixel2=%d kitty3=%d", kittyOne.runs.Load(), sixelTwo.runs.Load(), kittyThree.runs.Load())
	}
}
