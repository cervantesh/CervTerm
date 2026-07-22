package kitty

import (
	"errors"
	"math"
	"time"

	"cervterm/internal/termimage"
)

type Adapter struct {
	store        *termimage.Store
	frame        []byte
	active       *termimage.CandidateTransfer
	activeFrames uint64
	command      Command
	reply        ReplyPlan
	deadline     time.Time
	nextTransfer uint64
	closed       bool
	discarding   bool
}

func NewAdapter(store *termimage.Store) *Adapter { return &Adapter{store: store} }

func (a *Adapter) Advance(now time.Time, event APCEvent) Outcome {
	if a == nil || a.closed {
		return Outcome{}
	}
	if a.discarding {
		if event.Final || event.Cancelled || event.Overflow {
			a.discarding = false
		}
		return Outcome{}
	}
	if event.Cancelled || event.Overflow {
		out := a.reject(ReplyCancelled, a.replyForFrame())
		if !event.Final {
			a.discarding = true
		}
		return out
	}
	if len(event.Data) != 0 {
		if len(a.frame)+len(event.Data) > maxHeaderBytes+maxPayloadBytes+2 {
			out := a.reject(ReplyLimit, a.replyForFrame())
			if !event.Final {
				a.discarding = true
			}
			return out
		}
		a.frame = append(a.frame, event.Data...)
		if a.active != nil {
			if err := a.active.Touch(); err != nil {
				return a.reject(mapStoreError(err), a.reply)
			}
			if !a.captureDeadline(a.active) {
				return a.reject(ReplyTimeout, a.reply)
			}
		}
	}
	if !event.Final {
		return Outcome{}
	}
	frame := a.frame
	a.frame = nil
	parsed, failure := parseCompleteFrame(frame, a.active != nil)
	if failure != nil {
		if a.active != nil {
			return a.reject(failure.code, a.reply)
		}
		return a.reject(failure.code, failure.reply)
	}
	if a.active != nil {
		if a.activeFrames >= termimage.HardChunksPerTransfer {
			return a.reject(ReplyLimit, a.reply)
		}
		a.activeFrames++
		if !frameHasQuiet(frame) {
			parsed.reply = a.reply
		} else if parsed.reply.quiet != a.reply.quiet {
			return a.reject(ReplyInvalid, a.reply)
		}
		if len(parsed.payload) != 0 {
			if err := a.active.Append(parsed.payload); err != nil {
				return a.reject(mapStoreError(err), a.reply)
			}
		}
		if !a.captureDeadline(a.active) {
			return a.reject(ReplyTimeout, a.reply)
		}
		if parsed.more {
			return Outcome{}
		}
		transfer := a.active
		if err := transfer.Seal(); err != nil {
			return a.reject(mapStoreError(err), a.reply)
		}
		a.active = nil
		a.activeFrames = 0
		a.deadline = time.Time{}
		command := a.command
		command.Transfer = transfer
		reply := a.reply
		a.command = Command{}
		a.reply = ReplyPlan{}
		return Outcome{Command: &command, Reply: reply}
	}
	if parsed.command.Action == ActionPlace || parsed.command.Action == ActionDelete {
		command := parsed.command
		return Outcome{Command: &command, Reply: parsed.reply}
	}
	transfer, code := a.beginTransfer(now, parsed.command.Image)
	if code != ReplyNone {
		return a.reject(code, parsed.reply)
	}
	if len(parsed.payload) != 0 {
		if err := transfer.Append(parsed.payload); err != nil {
			transfer.Close()
			return Outcome{Reply: parsed.reply, Failure: mapStoreError(err)}
		}
	}
	if parsed.more {
		a.active = transfer
		a.activeFrames = 1
		a.command = parsed.command
		a.reply = parsed.reply
		if !a.captureDeadline(transfer) {
			transfer.Close()
			return Outcome{Reply: parsed.reply, Failure: ReplyTimeout}
		}
		return Outcome{}
	}
	if err := transfer.Seal(); err != nil {
		transfer.Close()
		return Outcome{Reply: parsed.reply, Failure: mapStoreError(err)}
	}
	command := parsed.command
	command.Transfer = transfer
	return Outcome{Command: &command, Reply: parsed.reply}
}

func (a *Adapter) beginTransfer(now time.Time, image termimage.ImageID) (*termimage.CandidateTransfer, ReplyCode) {
	if a.store == nil || a.store.Closed() {
		return nil, ReplyFailed
	}
	if a.nextTransfer >= math.MaxUint32 {
		return nil, ReplyLimit
	}
	a.nextTransfer++
	transfer, err := a.store.BeginTransfer(termimage.Header{Transfer: termimage.TransferID(a.nextTransfer), Image: image})
	if err != nil {
		return nil, mapStoreError(err)
	}
	if err = transfer.Touch(); err != nil {
		transfer.Close()
		return nil, mapStoreError(err)
	}
	_ = now
	return transfer, ReplyNone
}

func (a *Adapter) NextExpiry() (time.Time, bool) {
	if a == nil || a.active == nil || a.deadline.IsZero() {
		return time.Time{}, false
	}
	return a.deadline, true
}
func (a *Adapter) captureDeadline(transfer *termimage.CandidateTransfer) bool {
	deadline, ok := transfer.Deadline()
	if ok {
		a.deadline = deadline
	}
	return ok
}

func (a *Adapter) Expire(now time.Time) Outcome {
	if deadline, ok := a.NextExpiry(); !ok || now.Before(deadline) {
		return Outcome{}
	}
	reply := a.reply
	if a.active != nil {
		a.active.Close()
	}
	a.active = nil
	a.activeFrames = 0
	a.command = Command{}
	a.reply = ReplyPlan{}
	a.deadline = time.Time{}
	a.frame = nil
	return Outcome{Reply: reply, Failure: ReplyTimeout}
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
	a.activeFrames = 0
	a.frame = nil
	a.command = Command{}
	a.reply = ReplyPlan{}
	a.deadline = time.Time{}
}
func (a *Adapter) reject(code ReplyCode, reply ReplyPlan) Outcome {
	if a.active != nil {
		a.active.Close()
	}
	a.active = nil
	a.activeFrames = 0
	a.frame = nil
	a.command = Command{}
	a.reply = ReplyPlan{}
	a.deadline = time.Time{}
	return Outcome{Reply: reply, Failure: code}
}
func (a *Adapter) replyForFrame() ReplyPlan {
	if a.active != nil {
		return a.reply
	}
	if len(a.frame) > 1 {
		control := string(a.frame[1:])
		for i, c := range control {
			if c == ';' {
				control = control[:i]
				break
			}
		}
		return ReplyPlan{quiet: scanQuiet(control)}
	}
	return ReplyPlan{}
}
func frameHasQuiet(frame []byte) bool {
	if len(frame) < 2 {
		return false
	}
	control := string(frame[1:])
	for i, c := range control {
		if c == ';' {
			control = control[:i]
			break
		}
	}
	for _, pair := range splitComma(control) {
		if len(pair) >= 2 && pair[:2] == "q=" {
			return true
		}
	}
	return false
}
func splitComma(value string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ',' {
			out = append(out, value[start:i])
			start = i + 1
		}
	}
	return out
}
func mapStoreError(err error) ReplyCode {
	if errors.Is(err, termimage.ErrLimitExceeded) || errors.Is(err, termimage.ErrTooManyChunks) {
		return ReplyLimit
	}
	if errors.Is(err, termimage.ErrTransferExpired) || errors.Is(err, termimage.ErrTransferClosed) {
		return ReplyTimeout
	}
	if errors.Is(err, termimage.ErrInvalidChunk) || errors.Is(err, termimage.ErrInvalidID) {
		return ReplyInvalid
	}
	return ReplyFailed
}
