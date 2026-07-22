package mux

import (
	"cervterm/internal/core"
	"cervterm/internal/kitty"
	"cervterm/internal/termimage"
	"time"
)

func (m *Mux) processKittyOutcomes(p *pane) []Event {
	outcomes := p.kittyOutcomes
	p.kittyOutcomes = nil
	var events []Event
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
					spec := termimage.PlacementSpec{ID: request.ID, Anchor: termimage.CellAnchor{Row: int64(p.terminal.CursorRow()), Col: uint32(p.terminal.CursorCol())}, Cols: request.Cols, Rows: request.Rows, Crop: request.Crop, Z: request.Z, Opacity: 255}
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
	owner := kittyDecodeOwner{paneID: p.id, pane: p, generation: p.snapshot.ImageGeneration, token: m.kittyNextToken, replySlot: slot, hasSlot: hasSlot, plan: outcome.Reply, command: ownerCommand, acceptUntil: m.options.Now().Add(termimage.HardAcceptanceDeadline), anchorRow: int64(p.terminal.CursorRow()), anchorCol: uint32(p.terminal.CursorCol())}
	work := kittyDecodeWork{owner: owner, job: job}
	if err := m.kittyScheduler.submit(work); err != nil {
		if hasSlot {
			p.completeImageReply(slot, outcome.Reply.Encode(kitty.ReplyLimit))
		}
		return
	}
	m.kittyPending[owner.token] = owner
}

func (m *Mux) applyKittyCompletion(completion kittyDecodeCompletion) []Event {
	owner := completion.owner
	defer m.kittyScheduler.finish(owner)
	result := completion.result
	pendingOwner, pending := m.kittyPending[owner.token]
	if !pending || pendingOwner.pane != owner.pane {
		result.Close()
		return nil
	}
	delete(m.kittyPending, owner.token)
	p, ok := m.sessions.lookup(owner.paneID)
	if !ok || p != owner.pane || m.model.tabForPane(owner.paneID) == nil {
		result.Close()
		return nil
	}
	code := result.Failure
	if code == kitty.ReplyNone && (m.options.Now().After(owner.acceptUntil) || p.snapshot.ImageGeneration != owner.generation || result.Candidate == nil || !result.Candidate.ValidFor(p.imageStore)) {
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
	if result.Candidate != nil {
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
	if m == nil || m.kittyScheduler == nil {
		return time.Time{}, false
	}
	m.sessions.mu.Lock()
	defer m.sessions.mu.Unlock()
	var earliest time.Time
	found := false
	for _, p := range m.sessions.panes {
		if p.kittyAdapter == nil {
			continue
		}
		if deadline, ok := p.kittyAdapter.NextExpiry(); ok && (!found || deadline.Before(earliest)) {
			earliest, found = deadline, true
		}
	}
	for _, owner := range m.kittyPending {
		if !found || owner.acceptUntil.Before(earliest) {
			earliest, found = owner.acceptUntil, true
		}
	}
	return earliest, found
}

func (m *Mux) expireKitty(now time.Time) []Event {
	if m == nil || m.kittyScheduler == nil {
		return nil
	}
	m.sessions.mu.Lock()
	panes := make([]*pane, 0, len(m.sessions.panes))
	for _, p := range m.sessions.panes {
		panes = append(panes, p)
	}
	m.sessions.mu.Unlock()
	var events []Event
	for _, p := range panes {
		if p.kittyAdapter == nil {
			continue
		}
		outcome := p.kittyAdapter.Expire(now)
		if outcome.Failure != kitty.ReplyNone {
			p.kittyOutcomes = append(p.kittyOutcomes, outcome)
			events = append(events, m.processKittyOutcomes(p)...)
		}
	}
	for token, owner := range m.kittyPending {
		if now.Before(owner.acceptUntil) {
			continue
		}
		delete(m.kittyPending, token)
		if p, ok := m.sessions.lookup(owner.paneID); ok && p == owner.pane && owner.hasSlot {
			p.completeImageReply(owner.replySlot, owner.plan.Encode(kitty.ReplyTimeout))
			events = append(events, p.flushReplies()...)
		}
	}
	return m.ResolveEventAddresses(events)
}
