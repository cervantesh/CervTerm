package mux

import (
	"io"
	"math"

	"cervterm/internal/termimage"
)

type replyEntry struct {
	sequence uint64
	data     []byte
	reserved uint64
	ready    bool
}

type replySlot struct {
	pane     PaneID
	sequence uint64
}

type ReplyCounters struct {
	Queued, Written, Suppressed uint64
	PendingBytes, ReservedSlots uint64
}

type replyQueue struct {
	entries []replyEntry
	next    uint64
	stats   ReplyCounters
}

func (p *pane) nextReplySequence() (uint64, bool) {
	if p.replies.next == math.MaxUint64 {
		return 0, false
	}
	p.replies.next++
	return p.replies.next, true
}

func (p *pane) queueReply(data []byte) bool {
	sequence, ok := p.nextReplySequence()
	bytes := uint64(len(data))
	if !ok || bytes == 0 || bytes > termimage.HardReplyBytes || bytes > termimage.HardPendingReplyBytesPane-p.replies.stats.PendingBytes {
		p.replies.stats.Suppressed++
		return false
	}
	p.replies.entries = append(p.replies.entries, replyEntry{sequence: sequence, data: append([]byte(nil), data...), reserved: bytes, ready: true})
	p.replies.stats.Queued++
	p.replies.stats.PendingBytes += bytes
	return true
}

func (p *pane) reserveImageReply() (replySlot, bool) {
	sequence, ok := p.nextReplySequence()
	if !ok || termimage.HardReplyBytes > termimage.HardPendingReplyBytesPane-p.replies.stats.PendingBytes {
		p.replies.stats.Suppressed++
		return replySlot{}, false
	}
	p.replies.entries = append(p.replies.entries, replyEntry{sequence: sequence, reserved: termimage.HardReplyBytes})
	p.replies.stats.PendingBytes += termimage.HardReplyBytes
	p.replies.stats.ReservedSlots++
	return replySlot{pane: p.id, sequence: sequence}, true
}

func (p *pane) completeImageReply(slot replySlot, data []byte) bool {
	if slot.pane != p.id || slot.sequence == 0 {
		return false
	}
	for index := range p.replies.entries {
		entry := &p.replies.entries[index]
		if entry.sequence != slot.sequence || entry.ready || entry.reserved == 0 {
			continue
		}
		p.replies.stats.PendingBytes -= entry.reserved
		p.replies.stats.ReservedSlots--
		entry.ready = true
		bytes := uint64(len(data))
		if bytes == 0 || bytes > termimage.HardReplyBytes {
			entry.reserved = 1 // bounded ordered tombstone until the owner flushes
			p.replies.stats.PendingBytes++
			p.replies.stats.Suppressed++
			return true
		}
		entry.reserved = bytes
		entry.data = append([]byte(nil), data...)
		p.replies.stats.PendingBytes += bytes
		p.replies.stats.Queued++
		return true
	}
	return false
}

func (p *pane) clearReplies() {
	p.replies.entries = nil
	p.replies.stats.PendingBytes = 0
	p.replies.stats.ReservedSlots = 0
}

func (p *pane) flushReplies() []Event {
	ready := 0
	for ready < len(p.replies.entries) && p.replies.entries[ready].ready {
		ready++
	}
	if ready == 0 {
		return nil
	}
	entries := append([]replyEntry(nil), p.replies.entries[:ready]...)
	copy(p.replies.entries, p.replies.entries[ready:])
	p.replies.entries = p.replies.entries[:len(p.replies.entries)-ready]
	for _, entry := range entries {
		p.replies.stats.PendingBytes -= entry.reserved
	}
	if p.session == nil {
		return nil
	}
	var events []Event
	for _, entry := range entries {
		if len(entry.data) == 0 {
			continue
		}
		n, err := p.session.Write(entry.data)
		if err == nil && n != len(entry.data) {
			err = io.ErrShortWrite
		}
		if err != nil {
			events = append(events, Event{Kind: PaneWriteFailed, Pane: p.id, Err: err})
		} else {
			p.replies.stats.Written++
		}
	}
	return events
}

func (m *Mux) ReplyCounters(id PaneID) (ReplyCounters, bool) {
	pane, ok := m.sessions.lookup(id)
	if !ok || m.model.tabForPane(id) == nil {
		return ReplyCounters{}, false
	}
	return pane.replies.stats, true
}
