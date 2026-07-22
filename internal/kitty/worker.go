package kitty

import (
	"context"
	"errors"
	"sync/atomic"

	"cervterm/internal/termimage"
)

type DecodeJob struct {
	store   *termimage.Store
	command Command
	state   atomic.Uint32
}
type DecodeResult struct {
	Candidate *termimage.DecodedCandidate
	Failure   ReplyCode
}

func NewDecodeJob(store *termimage.Store, command Command) (*DecodeJob, ReplyCode) {
	if store == nil || command.Transfer == nil || (command.Action != ActionTransmit && command.Action != ActionTransmitAndPlace && command.Action != ActionQuery) {
		if command.Transfer != nil {
			command.Transfer.Close()
		}
		return nil, ReplyInvalid
	}
	return &DecodeJob{store: store, command: command}, ReplyNone
}
func (j *DecodeJob) Run(ctx context.Context) DecodeResult {
	if j == nil || !j.state.CompareAndSwap(0, 1) {
		return DecodeResult{Failure: ReplyCancelled}
	}
	transfer := j.command.Transfer
	j.command.Transfer = nil
	payload, err := transfer.TakeSealedPayload(j.store)
	if err != nil {
		transfer.Close()
		return DecodeResult{Failure: ReplyCancelled}
	}
	defer payload.Close()
	header, epoch := payload.Header(), payload.Epoch()
	if err = ctx.Err(); err != nil {
		return DecodeResult{Failure: ReplyCancelled}
	}
	var candidate *termimage.DecodedCandidate
	if j.command.Decode.Format == FormatPNG {
		candidate, err = decodePNG(ctx, j.store, header.Image, payload)
	} else {
		candidate, err = decodeRaw(ctx, j.store, header.Image, j.command.Decode, payload)
	}
	if err != nil {
		return DecodeResult{Failure: decodeFailure(err)}
	}
	if candidate.Epoch() != epoch || !candidate.ValidFor(j.store) || !candidate.WritesSealed() {
		candidate.Close()
		return DecodeResult{Failure: ReplyCancelled}
	}
	return DecodeResult{Candidate: candidate}
}
func (j *DecodeJob) Close() {
	if j == nil || !j.state.CompareAndSwap(0, 2) {
		return
	}
	if j.command.Transfer != nil {
		j.command.Transfer.Close()
		j.command.Transfer = nil
	}
}
func (r *DecodeResult) Close() {
	if r == nil || r.Candidate == nil {
		return
	}
	r.Candidate.Close()
	r.Candidate = nil
}
func decodeFailure(err error) ReplyCode {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, termimage.ErrClosed) || errors.Is(err, termimage.ErrTransferClosed) {
		return ReplyCancelled
	}
	if errors.Is(err, errDecodeLimit) || errors.Is(err, termimage.ErrLimitExceeded) {
		return ReplyLimit
	}
	if errors.Is(err, termimage.ErrCandidateInvalid) || errors.Is(err, termimage.ErrInvalidChunk) {
		return ReplyInvalid
	}
	return ReplyFailed
}
