package workscheduler

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
)

var (
	ErrClosed      = errors.New("work scheduler closed")
	ErrOwnerActive = errors.New("work scheduler owner already active")
	ErrQueueFull   = errors.New("work scheduler queue full")
	ErrInvalidWork = errors.New("work scheduler invalid work")
	ErrInvalidOpts = errors.New("work scheduler invalid options")
)

type Result interface {
	Close()
}

type Job[R Result] interface {
	Run(context.Context) R
	Close()
}

type Work[K comparable, O any, R Result] struct {
	Key   K
	Owner O
	Job   Job[R]
}

type Completion[K comparable, O any, R Result] struct {
	Key        K
	Owner      O
	Result     R
	FinishedAt time.Time
}

func (c *Completion[K, O, R]) Close() {
	if c == nil {
		return
	}
	closeResult(c.Result)
}

func closeResult[R Result](result R) {
	value := reflect.ValueOf(result)
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return
		}
	}
	result.Close()
}

type Options struct {
	Workers       int
	QueueCapacity int
	Wake          func()
	Now           func() time.Time
}

type activeState struct {
	finishedAt time.Time
}

type Scheduler[K comparable, O any, R Result] struct {
	ctx    context.Context
	cancel context.CancelFunc
	wake   func()
	now    func() time.Time

	mu          sync.Mutex
	workReady   *sync.Cond
	resultSpace *sync.Cond
	queue       []Work[K, O, R]
	active      map[K]activeState
	closed      bool

	results     []Completion[K, O, R]
	resultReady chan struct{}
	workers     sync.WaitGroup
	closeOnce   sync.Once
}

func New[K comparable, O any, R Result](options Options) (*Scheduler[K, O, R], error) {
	if options.Workers <= 0 || options.QueueCapacity <= 0 {
		return nil, ErrInvalidOpts
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &Scheduler[K, O, R]{
		ctx: ctx, cancel: cancel, wake: options.Wake, now: options.Now,
		queue:       make([]Work[K, O, R], 0, options.QueueCapacity),
		active:      make(map[K]activeState, options.QueueCapacity+options.Workers),
		results:     make([]Completion[K, O, R], 0, options.QueueCapacity),
		resultReady: make(chan struct{}, 1),
	}
	scheduler.workReady = sync.NewCond(&scheduler.mu)
	scheduler.resultSpace = sync.NewCond(&scheduler.mu)
	scheduler.workers.Add(options.Workers)
	for index := 0; index < options.Workers; index++ {
		go scheduler.runWorker()
	}
	return scheduler, nil
}

func (s *Scheduler[K, O, R]) Submit(work Work[K, O, R]) error {
	var zero K
	if s == nil {
		if work.Job != nil {
			work.Job.Close()
		}
		return ErrClosed
	}
	if work.Key == zero || work.Job == nil {
		if work.Job != nil {
			work.Job.Close()
		}
		return ErrInvalidWork
	}
	s.mu.Lock()
	var err error
	switch {
	case s.closed:
		err = ErrClosed
	case s.ownerActiveLocked(work.Key):
		err = ErrOwnerActive
	case len(s.queue) == cap(s.queue):
		err = ErrQueueFull
	default:
		s.active[work.Key] = activeState{}
		s.queue = append(s.queue, work)
		s.workReady.Signal()
	}
	s.mu.Unlock()
	if err != nil {
		work.Job.Close()
	}
	return err
}

func (s *Scheduler[K, O, R]) Ready() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.resultReady
}

func (s *Scheduler[K, O, R]) TakeCompletion() (Completion[K, O, R], bool) {
	if s == nil {
		return Completion[K, O, R]{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return Completion[K, O, R]{}, false
	}
	completion := s.results[0]
	copy(s.results, s.results[1:])
	s.results[len(s.results)-1] = Completion[K, O, R]{}
	s.results = s.results[:len(s.results)-1]
	s.resultSpace.Signal()
	if len(s.results) > 0 {
		select {
		case s.resultReady <- struct{}{}:
		default:
		}
	}
	return completion, true
}

func (s *Scheduler[K, O, R]) Finish(key K) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.active, key)
	s.mu.Unlock()
}

func (s *Scheduler[K, O, R]) CompletionTime(key K) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	s.mu.Lock()
	state, ok := s.active[key]
	s.mu.Unlock()
	return state.finishedAt, ok && !state.finishedAt.IsZero()
}

func (s *Scheduler[K, O, R]) Close() {
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
			delete(s.active, work.Key)
		}
		s.workReady.Broadcast()
		s.resultSpace.Broadcast()
		s.mu.Unlock()
		for _, work := range queued {
			work.Job.Close()
		}
		s.workers.Wait()
		s.mu.Lock()
		results := s.results
		s.results = nil
		s.mu.Unlock()
		for index := range results {
			results[index].Close()
		}
	})
}

func (s *Scheduler[K, O, R]) ownerActiveLocked(key K) bool {
	_, active := s.active[key]
	return active
}

func (s *Scheduler[K, O, R]) runWorker() {
	defer s.workers.Done()
	for {
		work, ok := s.takeWork()
		if !ok {
			return
		}
		result := work.Job.Run(s.ctx)
		if s.publishCompletion(work, result) && s.wake != nil {
			s.wake()
		}
	}
}

func (s *Scheduler[K, O, R]) publishCompletion(work Work[K, O, R], result R) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		closeResult(result)
		return false
	}
	finishedAt := s.now()
	if _, active := s.active[work.Key]; active {
		s.active[work.Key] = activeState{finishedAt: finishedAt}
	}
	for len(s.results) == cap(s.results) && !s.closed {
		s.resultSpace.Wait()
	}
	if s.closed {
		s.mu.Unlock()
		closeResult(result)
		return false
	}
	completion := Completion[K, O, R]{Key: work.Key, Owner: work.Owner, Result: result, FinishedAt: finishedAt}
	s.results = append(s.results, completion)
	select {
	case s.resultReady <- struct{}{}:
	default:
	}
	s.mu.Unlock()
	return true
}

func (s *Scheduler[K, O, R]) takeWork() (Work[K, O, R], bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closed {
		s.workReady.Wait()
	}
	if s.closed {
		return Work[K, O, R]{}, false
	}
	work := s.queue[0]
	copy(s.queue, s.queue[1:])
	s.queue[len(s.queue)-1] = Work[K, O, R]{}
	s.queue = s.queue[:len(s.queue)-1]
	return work, true
}
