package workscheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testResult struct{ closes atomic.Int32 }

func (r *testResult) Close() {
	if r != nil {
		r.closes.CompareAndSwap(0, 1)
	}
}

type testJob struct {
	protocol        string
	started         chan<- string
	release         <-chan struct{}
	active, maximum *atomic.Int32
	runs, closes    atomic.Int32
	result          *testResult
}

func (j *testJob) Run(ctx context.Context) *testResult {
	j.runs.Add(1)
	if j.active != nil {
		current := j.active.Add(1)
		for prior := j.maximum.Load(); current > prior && !j.maximum.CompareAndSwap(prior, current); prior = j.maximum.Load() {
		}
		defer j.active.Add(-1)
	}
	if j.started != nil {
		j.started <- j.protocol
	}
	if j.release != nil {
		select {
		case <-j.release:
		case <-ctx.Done():
		}
	}
	if j.result == nil {
		j.result = &testResult{}
	}
	return j.result
}
func (j *testJob) Close() { j.closes.Add(1) }

func awaitCompletion(t *testing.T, scheduler *Scheduler[uint64, string, *testResult]) Completion[uint64, string, *testResult] {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-scheduler.Ready():
			if completion, ok := scheduler.TakeCompletion(); ok {
				return completion
			}
		case <-deadline.C:
			t.Fatal("completion timeout")
			return Completion[uint64, string, *testResult]{}
		}
	}
}

func TestSchedulerIsProtocolNeutralAndBoundsWorkers(t *testing.T) {
	started := make(chan string, 8)
	release := make(chan struct{})
	var active, maximum atomic.Int32
	scheduler, err := New[uint64, string, *testResult](Options{Workers: 2, QueueCapacity: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	jobs := make([]*testJob, 8)
	for index := range jobs {
		protocol := []string{"kitty", "sixel", "iterm"}[index%3]
		jobs[index] = &testJob{protocol: protocol, started: started, release: release, active: &active, maximum: &maximum}
		if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: uint64(index + 1), Owner: protocol, Job: jobs[index]}); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	<-started
	select {
	case protocol := <-started:
		t.Fatalf("third worker started for %s", protocol)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	seen := make(map[string]bool)
	for range jobs {
		completion := awaitCompletion(t, scheduler)
		seen[completion.Owner] = true
		scheduler.Finish(completion.Key)
		completion.Close()
	}
	if maximum.Load() != 2 || !seen["kitty"] || !seen["sixel"] || !seen["iterm"] {
		t.Fatalf("maximum=%d seen=%v", maximum.Load(), seen)
	}
}

func TestSchedulerCompletionTimeAndActivitySurviveWorkerReturn(t *testing.T) {
	var nanos atomic.Int64
	now := func() time.Time { return time.Unix(0, nanos.Load()) }
	scheduler, err := New[uint64, string, *testResult](Options{Workers: 1, QueueCapacity: 4, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	deadline := int64(250 * time.Millisecond)
	for _, test := range []struct {
		name     string
		finished int64
		before   bool
	}{{"before", deadline - 1, true}, {"equal", deadline, false}, {"after", deadline + 1, false}} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan string, 1)
			release := make(chan struct{})
			job := &testJob{protocol: test.name, started: started, release: release}
			if err := scheduler.Submit(Work[uint64, string, *testResult]{Key: 1, Owner: test.name, Job: job}); err != nil {
				t.Fatal(err)
			}
			<-started
			nanos.Store(test.finished)
			close(release)
			completion := awaitCompletion(t, scheduler)
			if completion.FinishedAt.UnixNano() != test.finished || completion.FinishedAt.Before(time.Unix(0, deadline)) != test.before {
				t.Fatalf("finished=%v", completion.FinishedAt)
			}
			if stamped, ok := scheduler.CompletionTime(1); !ok || stamped != completion.FinishedAt {
				t.Fatalf("stamp=%v ok=%v", stamped, ok)
			}
			duplicate := &testJob{}
			if err := scheduler.Submit(Work[uint64, string, *testResult]{Key: 1, Owner: "duplicate", Job: duplicate}); !errors.Is(err, ErrOwnerActive) || duplicate.closes.Load() != 1 {
				t.Fatalf("duplicate err=%v closes=%d", err, duplicate.closes.Load())
			}
			scheduler.Finish(1)
			completion.Close()
		})
	}
}

func TestSchedulerQueueTimeCountsTowardCompletionBoundary(t *testing.T) {
	deadline := int64(250 * time.Millisecond)
	for _, test := range []struct {
		name     string
		finished int64
		before   bool
	}{
		{"before", deadline - 1, true},
		{"equal", deadline, false},
		{"after", deadline + 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var nanos atomic.Int64
			scheduler, err := New[uint64, string, *testResult](Options{Workers: 1, QueueCapacity: 4, Now: func() time.Time { return time.Unix(0, nanos.Load()) }})
			if err != nil {
				t.Fatal(err)
			}
			defer scheduler.Close()
			started := make(chan string, 2)
			blockerRelease := make(chan struct{})
			measuredRelease := make(chan struct{})
			if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: 1, Owner: "blocker", Job: &testJob{protocol: "blocker", started: started, release: blockerRelease}}); err != nil {
				t.Fatal(err)
			}
			if <-started != "blocker" {
				t.Fatal("blocker did not start first")
			}
			if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: 2, Owner: "measured", Job: &testJob{protocol: "measured", started: started, release: measuredRelease}}); err != nil {
				t.Fatal(err)
			}
			nanos.Store(test.finished - 1)
			close(blockerRelease)
			if <-started != "measured" {
				t.Fatal("queued job did not start after blocker")
			}
			nanos.Store(test.finished)
			close(measuredRelease)
			first := awaitCompletion(t, scheduler)
			scheduler.Finish(first.Key)
			first.Close()
			completion := awaitCompletion(t, scheduler)
			if completion.Key != 2 || completion.FinishedAt.UnixNano() != test.finished || completion.FinishedAt.Before(time.Unix(0, deadline)) != test.before {
				t.Fatalf("completion=%#v", completion)
			}
			scheduler.Finish(completion.Key)
			completion.Close()
		})
	}
}

func TestSchedulerRejectsExactQueueOverflowAndCloseRaceCleansJobs(t *testing.T) {
	started := make(chan string, 2)
	block := make(chan struct{})
	scheduler, err := New[uint64, string, *testResult](Options{Workers: 2, QueueCapacity: 32})
	if err != nil {
		t.Fatal(err)
	}
	for key := uint64(1); key <= 2; key++ {
		if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: key, Job: &testJob{started: started, release: block}}); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	<-started
	queued := make([]*testJob, 32)
	for index := range queued {
		queued[index] = &testJob{}
		if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: uint64(index + 10), Job: queued[index]}); err != nil {
			t.Fatal(err)
		}
	}
	overflow := &testJob{}
	if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: 100, Job: overflow}); !errors.Is(err, ErrQueueFull) || overflow.closes.Load() != 1 {
		t.Fatalf("overflow err=%v closes=%d", err, overflow.closes.Load())
	}
	closed := make(chan struct{})
	go func() { scheduler.Close(); close(closed) }()
	close(block)
	<-closed
	for index, job := range queued {
		if job.runs.Load()+job.closes.Load() != 1 {
			t.Fatalf("job %d runs=%d closes=%d", index, job.runs.Load(), job.closes.Load())
		}
	}
}

func TestSchedulerCloseSubmitRaceOwnsEveryJobExactlyOnce(t *testing.T) {
	scheduler, err := New[uint64, string, *testResult](Options{Workers: 2, QueueCapacity: 32})
	if err != nil {
		t.Fatal(err)
	}
	const count = 128
	jobs := make([]*testJob, count)
	start := make(chan struct{})
	var submitters sync.WaitGroup
	for index := range jobs {
		jobs[index] = &testJob{release: make(chan struct{})}
		submitters.Add(1)
		go func(index int) {
			defer submitters.Done()
			<-start
			_ = scheduler.Submit(Work[uint64, string, *testResult]{Key: uint64(index + 1), Job: jobs[index]})
		}(index)
	}
	done := make(chan struct{})
	go func() { <-start; scheduler.Close(); close(done) }()
	close(start)
	submitters.Wait()
	<-done
	for index, job := range jobs {
		if job.runs.Load()+job.closes.Load() != 1 {
			t.Fatalf("job %d runs=%d closes=%d", index, job.runs.Load(), job.closes.Load())
		}
	}
}

func TestSchedulerCloseUnblocksSaturatedResultPublication(t *testing.T) {
	const workers, queued = 2, 32
	wakes := make(chan struct{}, workers+queued)
	started := make(chan string, workers)
	release := make(chan struct{})
	scheduler, err := New[uint64, string, *testResult](Options{Workers: workers, QueueCapacity: queued, Wake: func() { wakes <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	jobs := make([]*testJob, workers+queued)
	for index := 0; index < workers; index++ {
		jobs[index] = &testJob{started: started, release: release}
		if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: uint64(index + 1), Job: jobs[index]}); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	<-started
	for index := workers; index < len(jobs); index++ {
		jobs[index] = &testJob{release: release}
		if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: uint64(index + 1), Job: jobs[index]}); err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	for index := 0; index < queued; index++ {
		select {
		case <-wakes:
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d of %d buffered results", index, queued)
		}
	}
	done := make(chan struct{})
	go func() { scheduler.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close deadlocked behind saturated result publication")
	}
	for index, job := range jobs {
		if job.result == nil || job.result.closes.Load() != 1 {
			t.Fatalf("result %d not drained exactly once", index)
		}
	}
}

func TestSchedulerStampsCompletionBeforeBoundedResultBackpressure(t *testing.T) {
	var nanos atomic.Int64
	deadline := int64(250 * time.Millisecond)
	scheduler, err := New[uint64, string, *testResult](Options{Workers: 1, QueueCapacity: 1, Now: func() time.Time { return time.Unix(0, nanos.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: 1, Job: &testJob{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-scheduler.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("first result did not fill bounded result storage")
	}
	started := make(chan string, 1)
	release := make(chan struct{})
	if err = scheduler.Submit(Work[uint64, string, *testResult]{Key: 2, Job: &testJob{protocol: "blocked-result", started: started, release: release}}); err != nil {
		t.Fatal(err)
	}
	<-started
	nanos.Store(deadline - 1)
	close(release)
	limit := time.Now().Add(2 * time.Second)
	for {
		if stamped, ok := scheduler.CompletionTime(2); ok {
			if stamped.UnixNano() != deadline-1 {
				t.Fatalf("stamp=%v", stamped)
			}
			break
		}
		if time.Now().After(limit) {
			t.Fatal("completion was not stamped while result storage was full")
		}
		time.Sleep(time.Millisecond)
	}
	first, ok := scheduler.TakeCompletion()
	if !ok || first.Key != 1 {
		t.Fatalf("first completion=%#v ok=%v", first, ok)
	}
	scheduler.Finish(first.Key)
	first.Close()
	second := awaitCompletion(t, scheduler)
	if second.Key != 2 || !second.FinishedAt.Before(time.Unix(0, deadline)) {
		t.Fatalf("second completion=%#v", second)
	}
	scheduler.Finish(second.Key)
	second.Close()
}

type nilInterfaceResultJob struct{}

func (*nilInterfaceResultJob) Run(context.Context) Result { return nil }
func (*nilInterfaceResultJob) Close()                     {}

func TestSchedulerNilInterfaceResultCleanupIsSafe(t *testing.T) {
	scheduler, err := New[uint64, string, Result](Options{Workers: 1, QueueCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Submit(Work[uint64, string, Result]{Key: 1, Job: &nilInterfaceResultJob{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-scheduler.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("nil result completion timeout")
	}
	completion, ok := scheduler.TakeCompletion()
	if !ok {
		t.Fatal("nil result completion missing")
	}
	completion.Close()
	scheduler.Finish(completion.Key)
	scheduler.Close()
}

type typedNilResult struct{}

func (r *typedNilResult) Close() {
	if r == nil {
		panic("typed nil Close")
	}
}

type typedNilResultJob struct{}

func (*typedNilResultJob) Run(context.Context) *typedNilResult { return nil }
func (*typedNilResultJob) Close()                              {}

func TestSchedulerTypedNilResultCleanupIsSafe(t *testing.T) {
	scheduler, err := New[uint64, string, *typedNilResult](Options{Workers: 1, QueueCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Submit(Work[uint64, string, *typedNilResult]{Key: 1, Job: &typedNilResultJob{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-scheduler.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("typed nil result completion timeout")
	}
	completion, ok := scheduler.TakeCompletion()
	if !ok {
		t.Fatal("typed nil result completion missing")
	}
	completion.Close()
	scheduler.Finish(completion.Key)
	scheduler.Close()
}
