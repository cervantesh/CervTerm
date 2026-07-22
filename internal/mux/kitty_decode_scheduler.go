package mux

import (
	"context"
	"errors"
	"sync"
	"time"

	"cervterm/internal/kitty"
)

const (
	kittyDecodeQueueCapacity = 32
	kittyDecodeWorkerCount   = 2
)

var (
	errKittyDecodeSchedulerClosed = errors.New("mux: kitty decode scheduler closed")
	errKittyDecodePaneActive      = errors.New("mux: kitty decode pane already active")
	errKittyDecodeQueueFull       = errors.New("mux: kitty decode queue full")
	errKittyDecodeInvalidWork     = errors.New("mux: invalid kitty decode work")
)

// kittyDecodeJob is implemented by *kitty.DecodeJob. Keeping the worker input
// behind this package-private seam permits deterministic scheduler tests without
// wiring the parser, core, or pane lifecycle into this foundation.
type kittyDecodeJob interface {
	Run(context.Context) kitty.DecodeResult
	Close()
}

// kittyDecodeOwner is opaque owner-thread state. The scheduler only preserves
// it across the worker boundary; it never dereferences or validates the tokens.
type kittyDecodeOwner struct {
	paneID      PaneID
	pane        *pane
	generation  uint64
	reflowGen   uint64
	anchorGen   uint64
	token       uint64
	replySlot   replySlot
	hasSlot     bool
	plan        kitty.ReplyPlan
	command     kitty.Command
	acceptUntil time.Time
	anchorRow   int64
	anchorCol   uint32
}

type kittyDecodeWork struct {
	owner kittyDecodeOwner
	job   kittyDecodeJob
}

type kittyDecodeCompletion struct {
	owner  kittyDecodeOwner
	result kitty.DecodeResult
}

func (c *kittyDecodeCompletion) close() {
	if c == nil {
		return
	}
	c.result.Close()
}

// kittyDecodeScheduler owns exactly two process-wide decode workers. submit
// transfers job ownership on success and closes every rejected job. Workers run
// DecodeJob.Run synchronously and transfer successful candidate ownership to the
// buffered completion channel.
type kittyDecodeScheduler struct {
	ctx    context.Context
	cancel context.CancelFunc
	wake   func()

	mu     sync.Mutex
	ready  *sync.Cond
	queue  []kittyDecodeWork
	active map[PaneID]struct{}
	closed bool

	results   chan kittyDecodeCompletion
	workers   sync.WaitGroup
	closeOnce sync.Once
}

func newKittyDecodeScheduler(wake func()) *kittyDecodeScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &kittyDecodeScheduler{
		ctx:     ctx,
		cancel:  cancel,
		wake:    wake,
		queue:   make([]kittyDecodeWork, 0, kittyDecodeQueueCapacity),
		active:  make(map[PaneID]struct{}, kittyDecodeQueueCapacity+kittyDecodeWorkerCount),
		results: make(chan kittyDecodeCompletion, kittyDecodeQueueCapacity),
	}
	scheduler.ready = sync.NewCond(&scheduler.mu)
	scheduler.workers.Add(kittyDecodeWorkerCount)
	for index := 0; index < kittyDecodeWorkerCount; index++ {
		go scheduler.runWorker()
	}
	return scheduler
}

func (s *kittyDecodeScheduler) submit(work kittyDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.owner.paneID == 0 || work.job == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeInvalidWork
	}

	s.mu.Lock()
	var err error
	switch {
	case s.closed:
		err = errKittyDecodeSchedulerClosed
	case s.paneActiveLocked(work.owner.paneID):
		err = errKittyDecodePaneActive
	case len(s.queue) == kittyDecodeQueueCapacity:
		err = errKittyDecodeQueueFull
	default:
		s.active[work.owner.paneID] = struct{}{}
		s.queue = append(s.queue, work)
		s.ready.Signal()
	}
	s.mu.Unlock()
	if err != nil {
		work.job.Close()
	}
	return err
}

func (s *kittyDecodeScheduler) completions() <-chan kittyDecodeCompletion {
	if s == nil {
		return nil
	}
	return s.results
}

func (s *kittyDecodeScheduler) finish(owner kittyDecodeOwner) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.active, owner.paneID)
	s.mu.Unlock()
}

func (s *kittyDecodeScheduler) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		queued := s.queue
		s.queue = nil
		for _, work := range queued {
			delete(s.active, work.owner.paneID)
		}
		s.ready.Broadcast()
		s.mu.Unlock()

		for _, work := range queued {
			work.job.Close()
		}
		s.workers.Wait()
		close(s.results)
		for completion := range s.results {
			completion.close()
		}
	})
}

func (s *kittyDecodeScheduler) paneActiveLocked(paneID PaneID) bool {
	_, active := s.active[paneID]
	return active
}

func (s *kittyDecodeScheduler) runWorker() {
	defer s.workers.Done()
	for {
		work, ok := s.takeWork()
		if !ok {
			return
		}

		completion := kittyDecodeCompletion{owner: work.owner, result: work.job.Run(s.ctx)}
		delivered := false
		select {
		case s.results <- completion:
			delivered = true
		case <-s.ctx.Done():
			completion.close()
		}
		if delivered && s.wake != nil {
			s.wake()
		}
	}
}

func (s *kittyDecodeScheduler) takeWork() (kittyDecodeWork, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closed {
		s.ready.Wait()
	}
	if s.closed {
		return kittyDecodeWork{}, false
	}
	work := s.queue[0]
	copy(s.queue, s.queue[1:])
	s.queue[len(s.queue)-1] = kittyDecodeWork{}
	s.queue = s.queue[:len(s.queue)-1]
	return work, true
}
