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

func (j *decodeSchedulerTestJob) Run(ctx context.Context) kitty.DecodeResult {
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
			return kitty.DecodeResult{Failure: kitty.ReplyCancelled}
		}
	}
	return kitty.DecodeResult{Failure: kitty.ReplyFailed}
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

func awaitSchedulerCompletion(t *testing.T, completions <-chan kittyDecodeCompletion) kittyDecodeCompletion {
	t.Helper()
	select {
	case completion, ok := <-completions:
		if !ok {
			t.Fatal("scheduler completion channel closed early")
		}
		return completion
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler completion")
		return kittyDecodeCompletion{}
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
		completion := awaitSchedulerCompletion(t, scheduler.completions())
		completion.close()
	}
	if got := maximum.Load(); got != kittyDecodeWorkerCount {
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
	completion := awaitSchedulerCompletion(t, scheduler.completions())
	completion.close()
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
	completion := awaitSchedulerCompletion(t, scheduler.completions())
	defer completion.close()
	awaitSchedulerSignals(t, wakes, 1)
	if completion.owner.paneID != owner.paneID || completion.owner.pane != paneToken || completion.owner.generation != 42 ||
		completion.owner.replySlot != slot || !completion.owner.hasSlot {
		t.Fatalf("completion owner = %#v, want %#v", completion.owner, owner)
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
	if _, ok := <-scheduler.completions(); ok {
		t.Fatal("completion channel remains open after close")
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
