package mux

import (
	"context"
	"time"

	"cervterm/internal/kitty"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
	"cervterm/internal/workscheduler"
)

const (
	imageDecodeQueueCapacity = int(termimage.HardPendingTransfersProcess)
	imageDecodeWorkerCount   = int(termimage.HardDecodeWorkersProcess)

	// Compatibility aliases keep the Phase 13 focused scheduler tests stable.
	kittyDecodeQueueCapacity = imageDecodeQueueCapacity
	kittyDecodeWorkerCount   = imageDecodeWorkerCount
)

var (
	errKittyDecodeSchedulerClosed = workscheduler.ErrClosed
	errKittyDecodePaneActive      = workscheduler.ErrOwnerActive
	errKittyDecodeQueueFull       = workscheduler.ErrQueueFull
	errKittyDecodeInvalidWork     = workscheduler.ErrInvalidWork
)

type imageDecodeProtocol uint8

const (
	imageDecodeKitty imageDecodeProtocol = iota + 1
	imageDecodeSixel
)

type imageDecodeOwner struct {
	protocol imageDecodeProtocol
	value    any
}

type imageDecodeCompletion = workscheduler.Completion[PaneID, imageDecodeOwner, workscheduler.Result]

type imageDecodeScheduler struct {
	inner *workscheduler.Scheduler[PaneID, imageDecodeOwner, workscheduler.Result]
}

// Kitty alias keeps focused compatibility tests on the shared instance.
type kittyDecodeScheduler = imageDecodeScheduler

// kittyDecodeOwner remains owner-thread-only protocol metadata. The shared
// scheduler sees it only as an opaque value in imageDecodeOwner.
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

type sixelDecodeOwner struct {
	paneID          PaneID
	pane            *pane
	model           *Model
	store           *termimage.Store
	storeEpoch      termimage.StoreEpoch
	imageGeneration uint64
	reflowGen       uint64
	anchorGen       uint64
	token           uint64
	metrics         CellMetrics
	image           termimage.ImageID
	placement       termimage.PlacementID
	raster          sixel.Raster
	acceptUntil     time.Time
	anchor          termimage.CellAnchor
}

type kittyDecodeJob interface {
	Run(context.Context) *kitty.DecodeResult
	Close()
}

type kittyDecodeJobAdapter struct{ job kittyDecodeJob }

func (j *kittyDecodeJobAdapter) Run(ctx context.Context) workscheduler.Result { return j.job.Run(ctx) }
func (j *kittyDecodeJobAdapter) Close()                                       { j.job.Close() }

type kittyDecodeWork struct {
	owner kittyDecodeOwner
	job   kittyDecodeJob
}

type kittyDecodeCompletion struct {
	Owner      kittyDecodeOwner
	Result     *kitty.DecodeResult
	FinishedAt time.Time
}

func (c *kittyDecodeCompletion) Close() {
	if c != nil && c.Result != nil {
		c.Result.Close()
	}
}

type sixelDecodeJob interface {
	Run(context.Context) *sixel.DecodeResult
	Close()
}

type sixelDecodeJobAdapter struct{ job sixelDecodeJob }

func (j *sixelDecodeJobAdapter) Run(ctx context.Context) workscheduler.Result { return j.job.Run(ctx) }
func (j *sixelDecodeJobAdapter) Close()                                       { j.job.Close() }

type sixelDecodeWork struct {
	owner sixelDecodeOwner
	job   sixelDecodeJob
}

type sixelDecodeCompletion struct {
	Owner      sixelDecodeOwner
	Result     *sixel.DecodeResult
	FinishedAt time.Time
}

func (c *sixelDecodeCompletion) Close() {
	if c != nil && c.Result != nil {
		c.Result.Close()
	}
}

func newImageDecodeScheduler(wake func(), now ...func() time.Time) *imageDecodeScheduler {
	var clock func() time.Time
	if len(now) > 0 {
		clock = now[0]
	}
	inner, err := workscheduler.New[PaneID, imageDecodeOwner, workscheduler.Result](workscheduler.Options{
		Workers: imageDecodeWorkerCount, QueueCapacity: imageDecodeQueueCapacity, Wake: wake, Now: clock,
	})
	if err != nil {
		panic(err)
	}
	return &imageDecodeScheduler{inner: inner}
}

func newKittyDecodeScheduler(wake func(), now ...func() time.Time) *kittyDecodeScheduler {
	return newImageDecodeScheduler(wake, now...)
}

func (s *imageDecodeScheduler) submitKitty(work kittyDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.job == nil {
		return errKittyDecodeInvalidWork
	}
	adapter := &kittyDecodeJobAdapter{job: work.job}
	return s.inner.Submit(workscheduler.Work[PaneID, imageDecodeOwner, workscheduler.Result]{
		Key: work.owner.paneID, Owner: imageDecodeOwner{protocol: imageDecodeKitty, value: work.owner}, Job: adapter,
	})
}

func (s *imageDecodeScheduler) submit(work kittyDecodeWork) error {
	return s.submitKitty(work)
}

func (s *imageDecodeScheduler) submitSixel(work sixelDecodeWork) error {
	if s == nil {
		if work.job != nil {
			work.job.Close()
		}
		return errKittyDecodeSchedulerClosed
	}
	if work.job == nil {
		return errKittyDecodeInvalidWork
	}
	adapter := &sixelDecodeJobAdapter{job: work.job}
	return s.inner.Submit(workscheduler.Work[PaneID, imageDecodeOwner, workscheduler.Result]{
		Key: work.owner.paneID, Owner: imageDecodeOwner{protocol: imageDecodeSixel, value: work.owner}, Job: adapter,
	})
}

func (s *imageDecodeScheduler) ready() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.inner.Ready()
}

func (s *imageDecodeScheduler) takeCompletion() (imageDecodeCompletion, bool) {
	if s == nil {
		return imageDecodeCompletion{}, false
	}
	return s.inner.TakeCompletion()
}

func decodeKittyCompletion(completion imageDecodeCompletion) (kittyDecodeCompletion, bool) {
	if completion.Owner.protocol != imageDecodeKitty {
		return kittyDecodeCompletion{}, false
	}
	owner, ok := completion.Owner.value.(kittyDecodeOwner)
	if !ok {
		return kittyDecodeCompletion{}, false
	}
	result, ok := completion.Result.(*kitty.DecodeResult)
	if !ok {
		return kittyDecodeCompletion{}, false
	}
	return kittyDecodeCompletion{Owner: owner, Result: result, FinishedAt: completion.FinishedAt}, true
}

func decodeSixelCompletion(completion imageDecodeCompletion) (sixelDecodeCompletion, bool) {
	if completion.Owner.protocol != imageDecodeSixel {
		return sixelDecodeCompletion{}, false
	}
	owner, ok := completion.Owner.value.(sixelDecodeOwner)
	if !ok {
		return sixelDecodeCompletion{}, false
	}
	result, ok := completion.Result.(*sixel.DecodeResult)
	if !ok {
		return sixelDecodeCompletion{}, false
	}
	return sixelDecodeCompletion{Owner: owner, Result: result, FinishedAt: completion.FinishedAt}, true
}

func (s *imageDecodeScheduler) finish(paneID PaneID) {
	if s != nil {
		s.inner.Finish(paneID)
	}
}

func (s *imageDecodeScheduler) completionTime(paneID PaneID) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	return s.inner.CompletionTime(paneID)
}

func (s *imageDecodeScheduler) close() {
	if s != nil {
		s.inner.Close()
	}
}
