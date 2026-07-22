package itermimage

import (
	"errors"
	"time"

	"cervterm/internal/termimage"
)

type Adapter struct {
	store      *termimage.Store
	scan       scanner
	active     *termimage.CandidateTransfer
	image      termimage.ImageID
	deadline   time.Time
	closed     bool
	discarding bool
}

func NewAdapter(store *termimage.Store) *Adapter { return &Adapter{store: store} }

func (a *Adapter) Advance(now time.Time, event OSCEvent) Outcome {
	_ = now
	if a == nil || a.closed {
		return Outcome{}
	}
	if a.discarding {
		if event.Final || event.Cancelled || event.Overflow {
			a.discarding = false
		}
		return Outcome{}
	}
	if event.Overflow {
		return a.reject(FailureLimit, !event.Final)
	}
	if event.Cancelled {
		return a.reject(FailureCancelled, !event.Final)
	}
	if uint64(len(event.Data)) > termimage.HardControlChunkBytes {
		return a.reject(FailureLimit, !event.Final)
	}
	if len(event.Data) != 0 {
		encoded, failure := a.scan.feed(event.Data)
		if failure != FailureNone {
			return a.reject(failure, !event.Final)
		}
		if len(encoded) != 0 {
			if a.active == nil {
				if failure := a.begin(); failure != FailureNone {
					return a.reject(failure, !event.Final)
				}
			}
			if err := a.active.Append(encoded); err != nil {
				return a.reject(mapStoreError(err), !event.Final)
			}
			a.captureDeadline()
		}
	}
	if !event.Final {
		return Outcome{}
	}
	if failure := a.scan.finish(); failure != FailureNone {
		return a.reject(failure, false)
	}
	if a.active == nil {
		return a.reject(FailureInvalid, false)
	}
	transfer := a.active
	if err := transfer.Seal(); err != nil {
		return a.reject(mapStoreError(err), false)
	}
	placement, err := a.store.AllocateInternalPlacementID()
	if err != nil {
		transfer.Close()
		a.active = nil
		a.reset()
		return Outcome{Failure: mapStoreError(err)}
	}
	command := &Command{
		Image:     a.image,
		Placement: placement,
		Metadata:  a.scan.metadata,
		Transfer:  transfer,
	}
	a.active = nil
	a.reset()
	return Outcome{Command: command}
}

func (a *Adapter) begin() Failure {
	if a.store == nil || a.store.Closed() {
		return FailureFailed
	}
	image, err := a.store.AllocateInternalImageID()
	if err != nil {
		return mapStoreError(err)
	}
	transfer, err := a.store.BeginTransfer(termimage.Header{
		Transfer: termimage.TransferID(image),
		Image:    image,
	})
	if err != nil {
		return mapStoreError(err)
	}
	a.image = image
	a.active = transfer
	a.captureDeadline()
	return FailureNone
}

func (a *Adapter) NextExpiry() (time.Time, bool) {
	if a == nil || a.active == nil || a.deadline.IsZero() {
		return time.Time{}, false
	}
	return a.deadline, true
}

func (a *Adapter) Expire(now time.Time) Outcome {
	deadline, ok := a.NextExpiry()
	if !ok || now.Before(deadline) {
		return Outcome{}
	}
	failure := FailureTimeout
	if a.active != nil && !a.active.Expire(now) {
		if !a.active.Closed() {
			a.captureDeadline()
			return Outcome{}
		}
		failure = FailureCancelled
	}
	a.active = nil
	a.reset()
	a.discarding = true
	return Outcome{Failure: failure}
}

func (a *Adapter) Close() {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	if a.active != nil {
		a.active.Close()
	}
	a.active = nil
	a.reset()
}

func (a *Adapter) reject(failure Failure, discard bool) Outcome {
	if a.active != nil {
		a.active.Close()
	}
	a.active = nil
	a.reset()
	a.discarding = discard
	return Outcome{Failure: failure}
}

func (a *Adapter) captureDeadline() {
	if a.active == nil {
		return
	}
	if deadline, ok := a.active.Deadline(); ok {
		a.deadline = deadline
	}
}

func (a *Adapter) reset() {
	a.scan = scanner{}
	a.image = 0
	a.deadline = time.Time{}
}

func mapStoreError(err error) Failure {
	switch {
	case err == nil:
		return FailureNone
	case errors.Is(err, termimage.ErrLimitExceeded), errors.Is(err, termimage.ErrTooManyChunks), errors.Is(err, termimage.ErrInternalIDExhausted), errors.Is(err, termimage.ErrInvalidChunk):
		return FailureLimit
	case errors.Is(err, termimage.ErrTransferExpired):
		return FailureTimeout
	case errors.Is(err, termimage.ErrClosed), errors.Is(err, termimage.ErrTransferClosed):
		return FailureCancelled
	default:
		return FailureFailed
	}
}
