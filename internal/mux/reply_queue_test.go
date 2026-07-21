package mux

import (
	"bytes"
	"math"
	"testing"

	"cervterm/internal/termimage"
)

func replyTestPane() (*pane, *fakeSession) {
	p := newPane(1, 8, 2, nil, nil)
	s := newFakeSession()
	p.session = s
	return p, s
}

func TestReplyQueuePreservesOrderAndAccounting(t *testing.T) {
	p, s := replyTestPane()
	if !p.queueReply([]byte("A")) || !p.queueReply([]byte("B")) {
		t.Fatal("queue rejected")
	}
	p.flushReplies()
	if got := string(s.written()); got != "AB" {
		t.Fatalf("wire=%q", got)
	}
	if q := p.replies.stats; q.Queued != 2 || q.Written != 2 || q.PendingBytes != 0 {
		t.Fatalf("stats=%#v", q)
	}
}

func TestReplyReservationPreventsOvertake(t *testing.T) {
	p, s := replyTestPane()
	slot, ok := p.reserveImageReply()
	if !ok || !p.queueReply([]byte("DSR")) {
		t.Fatal("setup failed")
	}
	p.flushReplies()
	if len(s.written()) != 0 {
		t.Fatal("later reply overtook reservation")
	}
	if !p.completeImageReply(slot, []byte("IMG")) {
		t.Fatal("completion rejected")
	}
	p.flushReplies()
	if got := string(s.written()); got != "IMGDSR" {
		t.Fatalf("wire=%q", got)
	}
}

func TestReplyQueueBoundsAndOversizeSuppression(t *testing.T) {
	p, s := replyTestPane()
	capBytes := int(termimage.HardPendingReplyBytesPane)
	payload := bytes.Repeat([]byte{'x'}, int(termimage.HardReplyBytes))
	for queued := 0; queued < capBytes/len(payload); queued++ {
		if !p.queueReply(payload) {
			t.Fatalf("reply %d rejected before cap", queued)
		}
	}
	if p.queueReply([]byte{'y'}) || p.queueReply(bytes.Repeat([]byte{'z'}, int(termimage.HardReplyBytes)+1)) {
		t.Fatal("cap/per-reply enforcement failed")
	}
	p.flushReplies()
	if len(s.written()) != capBytes {
		t.Fatal("cap payload not written")
	}
	slot, ok := p.reserveImageReply()
	if !ok || !p.queueReply([]byte("next")) {
		t.Fatal("reservation setup failed")
	}
	if !p.completeImageReply(slot, bytes.Repeat([]byte{'q'}, int(termimage.HardReplyBytes)+1)) {
		t.Fatal("oversize completion not consumed")
	}
	p.flushReplies()
	if got := string(s.written()[capBytes:]); got != "next" {
		t.Fatalf("wire tail=%q", got)
	}
	if p.replies.stats.Suppressed != 3 || p.replies.stats.PendingBytes != 0 {
		t.Fatalf("stats=%#v", p.replies.stats)
	}
	slot, ok = p.reserveImageReply()
	if !ok || !p.completeImageReply(slot, nil) {
		t.Fatal("empty completion not consumed")
	}
	if p.replies.stats.PendingBytes != 1 || len(p.replies.entries) != 1 {
		t.Fatal("empty completion was not bounded")
	}
	p.flushReplies()
	if p.replies.stats.PendingBytes != 0 {
		t.Fatal("empty completion tombstone leaked")
	}
}

func TestReplyQueueSequenceExhaustionAndCloseCleanup(t *testing.T) {
	p, _ := replyTestPane()
	if p.queueReply(nil) || len(p.replies.entries) != 0 {
		t.Fatal("empty reply created an uncharged entry")
	}
	p.replies.next = math.MaxUint64
	if p.queueReply([]byte("x")) {
		t.Fatal("sequence wrapped")
	}
	p.replies.next = 0
	if _, ok := p.reserveImageReply(); !ok {
		t.Fatal("reservation failed")
	}
	_ = p.close()
	if p.replies.stats.PendingBytes != 0 || p.replies.stats.ReservedSlots != 0 {
		t.Fatalf("stats=%#v", p.replies.stats)
	}
}
