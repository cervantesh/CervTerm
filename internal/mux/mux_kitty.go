package mux

import (
	"cervterm/internal/core"
	"cervterm/internal/itermimage"
	"cervterm/internal/kitty"
	"cervterm/internal/termimage"
	"time"
)

type muxProtocolSchedulingDispatchOperationAdapter struct {
	mux  *Mux
	pane *pane
}

func (m *Mux) processKittyOutcomes(p *pane) []Event {
	return (muxProtocolSchedulingDispatchOperationAdapter{mux: m, pane: p}).dispatchKitty(nil)
}

func (a muxProtocolSchedulingDispatchOperationAdapter) dispatchKitty(events []Event) []Event {
	return dispatchKittyOperation(events, a.mux, a.pane)
}

func dispatchKittyOperation(events []Event, m *Mux, p *pane) []Event {
	outcomes := p.kittyOutcomes
	p.kittyOutcomes = nil
	for _, outcome := range outcomes {
		if outcome.Failure != kitty.ReplyNone {
			p.queueReply(outcome.Reply.Encode(outcome.Failure))
			continue
		}
		if outcome.Command == nil {
			continue
		}
		command := *outcome.Command
		switch command.Action {
		case kitty.ActionDelete:
			code := kitty.ReplyOK
			if command.Delete == nil {
				code = kitty.ReplyInvalid
			} else if _, err := p.terminal.DeleteImages(*command.Delete); err != nil {
				code = kitty.ReplyFailed
			}
			p.queueReply(outcome.Reply.Encode(code))
			p.capture()
			events = append(events, Event{Kind: PaneDirty, Pane: p.id})
		case kitty.ActionPlace:
			code := kitty.ReplyInvalid
			if request := command.Placement; request != nil {
				if ref, ok := p.imageStore.ResourceRef(command.Image); ok {
					spec := termimage.PlacementSpec{ID: request.ID, Anchor: p.terminal.ImageCursorAnchor(), Cols: request.Cols, Rows: request.Rows, Crop: request.Crop, Z: request.Z, Opacity: 255}
					if _, err := p.terminal.CommitImage(core.ImageCommit{Existing: &ref, Placement: &spec}); err == nil {
						code = kitty.ReplyOK
					} else {
						code = kitty.ReplyFailed
					}
				} else {
					code = kitty.ReplyNotFound
				}
			}
			p.queueReply(outcome.Reply.Encode(code))
			p.capture()
			events = append(events, Event{Kind: PaneDirty, Pane: p.id})
		default:
			m.submitKittyDecode(p, outcome)
		}
	}
	events = append(events, p.flushReplies()...)
	return events
}

func (m *Mux) submitKittyDecode(p *pane, outcome kitty.Outcome) {
	if outcome.Command == nil {
		return
	}
	command := *outcome.Command
	var slot replySlot
	hasSlot := outcome.Reply.NeedsReservation()
	if hasSlot {
		var ok bool
		slot, ok = p.reserveImageReply()
		if !ok {
			command.Transfer.Close()
			return
		}
	}
	job, code := kitty.NewDecodeJob(p.imageStore, command)
	if code != kitty.ReplyNone {
		if hasSlot {
			p.completeImageReply(slot, outcome.Reply.Encode(code))
		}
		return
	}
	ownerCommand := command
	ownerCommand.Transfer = nil
	m.kittyNextToken++
	anchor := p.terminal.ImageCursorAnchor()
	owner := kittyDecodeOwner{paneID: p.id, pane: p, generation: p.snapshot.ImageGeneration, reflowGen: p.reflowGen, anchorGen: p.terminal.ImageAnchorGeneration(), token: m.kittyNextToken, replySlot: slot, hasSlot: hasSlot, plan: outcome.Reply, command: ownerCommand, acceptUntil: m.options.Now().Add(termimage.HardAcceptanceDeadline), anchorRow: anchor.Row, anchorCol: anchor.Col}
	work := kittyDecodeWork{owner: owner, job: job}
	if err := m.imageScheduler.submitKitty(work); err != nil {
		if hasSlot {
			p.completeImageReply(slot, outcome.Reply.Encode(kitty.ReplyLimit))
		}
		return
	}
	m.kittyPending[owner.token] = owner
}

type muxProtocolSchedulingApplyOperationAdapter struct {
	mux        *Mux
	now        time.Time
	completion *imageDecodeCompletion
}

func (m *Mux) applyImageCompletion(completion imageDecodeCompletion) []Event {
	return (&muxProtocolSchedulingApplyOperationAdapter{mux: m, completion: &completion}).applyCompletion(nil)
}

func (a *muxProtocolSchedulingApplyOperationAdapter) applyCompletion(events []Event) []Event {
	completed := applyImageCompletionOperation(a.mux, *a.completion)
	if len(events) == 0 {
		return completed
	}
	return append(events, completed...)
}

func applyImageCompletionOperation(m *Mux, completion imageDecodeCompletion) []Event {
	defer m.imageScheduler.finish(completion.Key)
	switch completion.Owner.protocol {
	case imageDecodeKitty:
		kittyCompletion, ok := decodeKittyCompletion(completion)
		if !ok {
			completion.Close()
			return nil
		}
		return m.applyKittyCompletion(kittyCompletion)
	case imageDecodeSixel:
		sixelCompletion, ok := decodeSixelCompletion(completion)
		if !ok {
			startedAt := time.Time{}
			if owner, ownerOK := completion.Owner.value.(sixelDecodeOwner); ownerOK {
				if pendingOwner, pending := m.sixelPending[owner.token]; !pending || pendingOwner != owner {
					completion.Close()
					return nil
				}
				delete(m.sixelPending, owner.token)
				startedAt = owner.startedAt
			}
			completion.Close()
			m.emitImageDiagnosticNow(ImageDiagnosticProtocolSixel, ImageDiagnosticReasonFailed, startedAt)
			return nil
		}
		return m.applySixelCompletion(sixelCompletion)
	case imageDecodeITerm:
		itermCompletion, ok := decodeITermCompletion(completion)
		if !ok {
			startedAt := time.Time{}
			if owner, ownerOK := completion.Owner.value.(itermDecodeOwner); ownerOK {
				if pendingOwner, pending := m.itermPending[owner.token]; !pending || pendingOwner != owner {
					completion.Close()
					return nil
				}
				delete(m.itermPending, owner.token)
				startedAt = owner.startedAt
			}
			completion.Close()
			m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonFailed, startedAt)
			return nil
		}
		return m.applyITermCompletion(itermCompletion)
	default:
		completion.Close()
		return nil
	}
}

func (m *Mux) applyKittyCompletion(completion kittyDecodeCompletion) []Event {
	owner := completion.Owner
	result := completion.Result
	pendingOwner, pending := m.kittyPending[owner.token]
	if !pending || pendingOwner.pane != owner.pane {
		if result != nil {
			result.Close()
		}
		return nil
	}
	delete(m.kittyPending, owner.token)
	p, ok := m.sessions.lookup(owner.paneID)
	if !ok || p != owner.pane || m.model.tabForPane(owner.paneID) == nil {
		if result != nil {
			result.Close()
		}
		return nil
	}
	code := kitty.ReplyCancelled
	if result != nil {
		code = result.Failure
	}
	if code == kitty.ReplyNone && (!completion.FinishedAt.Before(owner.acceptUntil) || p.snapshot.ImageGeneration != owner.generation || p.reflowGen != owner.reflowGen || p.terminal.ImageAnchorGeneration() != owner.anchorGen || result.Candidate == nil || !result.Candidate.ValidFor(p.imageStore)) {
		result.Close()
		code = kitty.ReplyCancelled
	}
	if code == kitty.ReplyNone {
		switch owner.command.Action {
		case kitty.ActionQuery:
			result.Close()
			code = kitty.ReplyOK
		case kitty.ActionTransmit, kitty.ActionTransmitAndPlace:
			var placement *termimage.PlacementSpec
			if owner.command.Action == kitty.ActionTransmitAndPlace && owner.command.Placement != nil {
				request := owner.command.Placement
				spec := termimage.PlacementSpec{ID: request.ID, Anchor: termimage.CellAnchor{Row: owner.anchorRow, Col: owner.anchorCol}, Cols: request.Cols, Rows: request.Rows, Crop: request.Crop, Z: request.Z, Opacity: 255}
				placement = &spec
			}
			_, err := p.terminal.CommitImage(core.ImageCommit{Candidate: result.Candidate, Placement: placement})
			result.Candidate = nil
			if err != nil {
				code = kitty.ReplyFailed
			} else {
				code = kitty.ReplyOK
			}
		default:
			result.Close()
			code = kitty.ReplyUnsupported
		}
	}
	if result != nil && result.Candidate != nil {
		result.Close()
	}
	if owner.hasSlot {
		p.completeImageReply(owner.replySlot, owner.plan.Encode(code))
	}
	p.capture()
	events := p.flushReplies()
	events = append(events, Event{Kind: PaneDirty, Pane: p.id})
	return m.ResolveEventAddresses(events)
}

func (m *Mux) NextImageDeadline() (time.Time, bool) {
	if m == nil || m.imageScheduler == nil {
		return time.Time{}, false
	}
	m.sessions.mu.Lock()
	defer m.sessions.mu.Unlock()
	var earliest time.Time
	found := false
	for _, p := range m.sessions.panes {
		if p.kittyAdapter != nil {
			if deadline, ok := p.kittyAdapter.NextExpiry(); ok && (!found || deadline.Before(earliest)) {
				earliest, found = deadline, true
			}
		}
		if p.sixelAdapter != nil {
			if deadline, ok := p.sixelAdapter.NextExpiry(); ok && (!found || deadline.Before(earliest)) {
				earliest, found = deadline, true
			}
		}
		if p.itermAdapter != nil {
			if deadline, ok := p.itermAdapter.NextExpiry(); ok && (!found || deadline.Before(earliest)) {
				earliest, found = deadline, true
			}
		}
	}
	for _, owner := range m.kittyPending {
		if !found || owner.acceptUntil.Before(earliest) {
			earliest, found = owner.acceptUntil, true
		}
	}
	for _, owner := range m.sixelPending {
		if !found || owner.acceptUntil.Before(earliest) {
			earliest, found = owner.acceptUntil, true
		}
	}
	for _, owner := range m.itermPending {
		if !found || owner.acceptUntil.Before(earliest) {
			earliest, found = owner.acceptUntil, true
		}
	}
	return earliest, found
}

func (m *Mux) expireImages(now time.Time) []Event {
	return (&muxProtocolSchedulingApplyOperationAdapter{mux: m, now: now}).applyExpiry(nil)
}

func (a *muxProtocolSchedulingApplyOperationAdapter) applyExpiry(events []Event) []Event {
	return applyImageExpiryOperation(events, a.mux, a.now)
}

func applyImageExpiryOperation(events []Event, m *Mux, now time.Time) []Event {
	if m == nil || m.imageScheduler == nil {
		return events
	}
	m.sessions.mu.Lock()
	panes := make([]*pane, 0, len(m.sessions.panes))
	for _, p := range m.sessions.panes {
		panes = append(panes, p)
	}
	m.sessions.mu.Unlock()
	for _, p := range panes {
		if p.kittyAdapter != nil {
			outcome := p.kittyAdapter.Expire(now)
			if outcome.Failure != kitty.ReplyNone {
				p.kittyOutcomes = append(p.kittyOutcomes, outcome)
				events = append(events, m.processKittyOutcomes(p)...)
			}
		}
		if p.sixelAdapter != nil {
			outcome := p.sixelAdapter.Expire(now)
			if outcome.Command != nil || outcome.Failure != 0 {
				p.sixelOutcomes = append(p.sixelOutcomes, outcome)
				m.processSixelOutcomes(p)
			}
		}
		if p.itermAdapter != nil {
			outcome := p.itermAdapter.Expire(now)
			if outcome.Command != nil || outcome.Failure != itermimage.FailureNone {
				p.itermOutcomes = append(p.itermOutcomes, outcome)
				m.processITermOutcomes(p)
			}
		}
	}
	for token, owner := range m.kittyPending {
		if now.Before(owner.acceptUntil) {
			continue
		}
		if m.imageCompletionBefore(owner.paneID, owner.acceptUntil) {
			continue
		}
		delete(m.kittyPending, token)
		if p, ok := m.sessions.lookup(owner.paneID); ok && p == owner.pane && owner.hasSlot {
			p.completeImageReply(owner.replySlot, owner.plan.Encode(kitty.ReplyTimeout))
			events = append(events, p.flushReplies()...)
		}
	}
	for token, owner := range m.sixelPending {
		if now.Before(owner.acceptUntil) {
			continue
		}
		if m.imageCompletionBefore(owner.paneID, owner.acceptUntil) {
			continue
		}
		delete(m.sixelPending, token)
		m.emitImageDiagnostic(ImageDiagnosticProtocolSixel, ImageDiagnosticReasonTimeout, owner.startedAt, now)
	}
	for token, owner := range m.itermPending {
		if now.Before(owner.acceptUntil) {
			continue
		}
		if m.imageCompletionBefore(owner.paneID, owner.acceptUntil) {
			continue
		}
		delete(m.itermPending, token)
		m.emitImageDiagnostic(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonTimeout, owner.startedAt, now)
	}
	return m.ResolveEventAddresses(events)
}

func (m *Mux) imageCompletionBefore(paneID PaneID, deadline time.Time) bool {
	finishedAt, completed := m.imageScheduler.completionTime(paneID)
	return completed && finishedAt.Before(deadline)
}

// expireKitty is retained for focused Phase 13 tests; expiration is now shared.
func (m *Mux) expireKitty(now time.Time) []Event { return m.expireImages(now) }
