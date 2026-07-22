package mux

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"cervterm/internal/kitty"
	"cervterm/internal/termimage"
)

func newKittyRuntimeMux(t *testing.T, enabled bool) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	limits := termimage.DefaultLimits()
	factory := &fakeFactory{}
	wakes := make(chan struct{}, 16)
	m := New(factory, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: enabled, Wake: func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	}})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, factory.sessions[0], wakes
}

func TestKittyRuntimeAsyncReplyPrecedesLaterDSRAndCommits(t *testing.T) {
	m, session, wakes := newKittyRuntimeMux(t, true)
	pane, _ := m.sessions.lookup(1)
	input := []byte("\x1b_Ga=t,i=1,s=1,v=1;AQIDBA==\x1b\\\x1b[5n")
	events := m.advancePane(pane, input)
	if len(events) == 0 {
		t.Fatal("no output events")
	}
	if got := session.written(); len(got) != 0 {
		t.Fatalf("DSR overtook Kitty reply: %q", got)
	}
	select {
	case <-wakes:
	case <-time.After(2 * time.Second):
		t.Fatal("decode did not wake owner")
	}
	_ = m.Drain(16)
	want := []byte("\x1b_Ga=t;OK\x1b\\\x1b[0n")
	if got := session.written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	ref, ok := pane.imageStore.ResourceRef(1)
	if !ok {
		t.Fatal("resource not committed")
	}
	resource, ok := m.AcquireImageResource(1, ref)
	if !ok || !bytes.Equal(resource.RGBA, []byte{1, 2, 3, 4}) {
		t.Fatalf("resource=%#v ok=%v", resource, ok)
	}
}

func TestKittyRuntimeDisabledInstallsNoSinkOrScheduler(t *testing.T) {
	m, session, _ := newKittyRuntimeMux(t, false)
	pane, _ := m.sessions.lookup(1)
	m.advancePane(pane, []byte("\x1b_Ga=t,i=1,s=1,v=1;AQIDBA==\x1b\\"))
	if m.kittyScheduler != nil || pane.kittyAdapter != nil || len(session.written()) != 0 {
		t.Fatal("disabled runtime allocated or replied")
	}
}

func TestKittyRuntimeSynchronousReplyPreservesParserOrder(t *testing.T) {
	m, session, _ := newKittyRuntimeMux(t, true)
	pane, _ := m.sessions.lookup(1)
	m.advancePane(pane, []byte("\x1b_Ga=d,d=A\x1b\\\x1b[5n"))
	want := []byte("\x1b_Ga=d;OK\x1b\\\x1b[0n")
	if got := session.written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
}

func TestKittyRuntimeAcceptanceDeadlineUnblocksOrderedReplies(t *testing.T) {
	limits := termimage.DefaultLimits()
	factory := &fakeFactory{}
	now := time.Now()
	m := New(factory, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true, Now: func() time.Time { return now }})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	p, _ := m.sessions.lookup(1)
	slot, ok := p.reserveImageReply()
	if !ok {
		t.Fatal("reserve")
	}
	p.queueReply([]byte("later"))
	owner := kittyDecodeOwner{paneID: 1, pane: p, token: 1, replySlot: slot, hasSlot: true, acceptUntil: now.Add(termimage.HardAcceptanceDeadline)}
	m.kittyPending[1] = owner
	deadline, ok := m.NextImageDeadline()
	if !ok {
		t.Fatal("missing acceptance deadline")
	}
	now = deadline
	_ = m.Drain(8)
	want := append(owner.plan.Encode(kitty.ReplyTimeout), []byte("later")...)
	if got := factory.sessions[0].written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	if len(m.kittyPending) != 0 {
		t.Fatal("pending decode survived deadline")
	}
}

func TestKittyRuntimeDeadlineExpiresWithoutIngress(t *testing.T) {
	limits := termimage.DefaultLimits()
	factory := &fakeFactory{}
	now := time.Now()
	m := New(factory, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true, Now: func() time.Time { return now }})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	pane, _ := m.sessions.lookup(1)
	m.advancePane(pane, []byte("\x1b_Ga=t,i=1,s=1,v=1,m=1;AQID\x1b\\"))
	deadline, ok := m.NextImageDeadline()
	if !ok {
		t.Fatal("missing deadline")
	}
	now = deadline
	_ = m.Drain(8)
	want := []byte("\x1b_Ga=t;ETIME\x1b\\")
	if got := factory.sessions[0].written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	if _, ok = m.NextImageDeadline(); ok {
		t.Fatal("deadline survived expiry")
	}
	if m.imageBudget.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", m.imageBudget.Usage())
	}
}

func TestKittyRuntimeEOFDiscardsPartialTransfer(t *testing.T) {
	m, session, _ := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	m.advancePane(p, []byte("\x1b_Ga=t,i=1,s=1,v=1,m=1;AQID\x1b\\"))
	m.sessions.incoming <- ingressRecord{pane: p.id, owner: p, err: io.EOF}
	_ = m.Drain(8)
	if got := session.written(); len(got) != 0 {
		t.Fatalf("unexpected EOF reply=%q", got)
	}
	if m.imageBudget.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", m.imageBudget.Usage())
	}
}

func TestKittyRuntimeResetRejectsStaleCompletionAndReleasesCandidate(t *testing.T) {
	m, session, _ := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	adapter := kitty.NewAdapter(p.imageStore)
	defer adapter.Close()
	out := adapter.Advance(time.Now(), kitty.APCEvent{Data: []byte("Ga=t,i=1,s=1,v=1;AQIDBA=="), Final: true})
	job, code := kitty.NewDecodeJob(p.imageStore, *out.Command)
	if code != kitty.ReplyNone {
		t.Fatal(code)
	}
	result := job.Run(context.Background())
	if result.Failure != kitty.ReplyNone {
		t.Fatal(result.Failure)
	}
	slot, ok := p.reserveImageReply()
	if !ok {
		t.Fatal("reserve")
	}
	owner := kittyDecodeOwner{paneID: 1, pane: p, generation: p.snapshot.ImageGeneration, token: 77, replySlot: slot, hasSlot: true, plan: out.Reply, acceptUntil: time.Now().Add(time.Second)}
	m.kittyPending[77] = owner
	p.terminal.ResetImages()
	p.capture()
	_ = m.applyKittyCompletion(kittyDecodeCompletion{owner: owner, result: result})
	want := out.Reply.Encode(kitty.ReplyCancelled)
	if got := session.written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	if m.imageBudget.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", m.imageBudget.Usage())
	}
}
