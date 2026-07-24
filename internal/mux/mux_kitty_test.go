package mux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
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
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: input}
	events := m.Drain(1)
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
	if m.imageScheduler != nil || pane.kittyAdapter != nil || len(session.written()) != 0 {
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
	_ = m.applyKittyCompletion(kittyDecodeCompletion{Owner: owner, Result: result, FinishedAt: owner.acceptUntil.Add(-time.Nanosecond)})
	want := out.Reply.Encode(kitty.ReplyCancelled)
	if got := session.written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	if m.imageBudget.Usage() != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", m.imageBudget.Usage())
	}
}

func seedKittyAnchorHistory(t *testing.T, p *pane) {
	t.Helper()
	for range p.terminal.Rows() + 2 {
		p.terminal.PutRune('x')
		p.terminal.CarriageReturn()
		p.terminal.NewLine()
	}
	if p.terminal.ScrollbackLines() == 0 {
		t.Fatal("setup did not create scrollback")
	}
}

func TestKittyAsyncPlacementCapturesCanonicalAnchorBeforeLaterPTYMovement(t *testing.T) {
	m, _, wakes := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	seedKittyAnchorHistory(t, p)
	p.terminal.SetCursor(1, 2)
	want := p.terminal.ImageCursorAnchor()
	m.advancePane(p, []byte("\x1b_Ga=T,i=1,p=1,c=1,r=1,s=1,v=1;AQIDBA==\x1b\\\x1b[1;1H"))
	if len(m.kittyPending) != 1 {
		t.Fatalf("pending=%d", len(m.kittyPending))
	}
	for _, owner := range m.kittyPending {
		if owner.anchorRow != want.Row || owner.anchorCol != want.Col {
			t.Fatalf("captured=(%d,%d) want=%#v", owner.anchorRow, owner.anchorCol, want)
		}
	}
	if p.terminal.CursorRow() != 0 || p.terminal.CursorCol() != 0 {
		t.Fatalf("later PTY cursor movement was not applied: (%d,%d)", p.terminal.CursorRow(), p.terminal.CursorCol())
	}
	select {
	case <-wakes:
	case <-time.After(2 * time.Second):
		t.Fatal("decode did not wake owner")
	}
	_ = m.Drain(16)
	viewportTop := p.terminal.ScrollbackLines()
	projection := p.terminal.ImageProjection(viewportTop, p.terminal.Rows())
	if len(projection.Placements) != 1 || projection.Placements[0].ID != 1 || projection.Placements[0].Anchor.Row != want.Row-int64(viewportTop) || projection.Placements[0].Anchor.Col != want.Col {
		t.Fatalf("projection=%#v want=%#v viewportTop=%d", projection, want, viewportTop)
	}
	if usage := p.imageStore.Usage(); usage.Images != 1 || usage.Placements != 1 {
		t.Fatalf("usage=%#v", usage)
	}
	if p.terminal.CursorRow() != 0 || p.terminal.CursorCol() != 0 {
		t.Fatal("image completion moved the later cursor")
	}
}

func TestKittySynchronousPlaceUsesCanonicalPrimaryAndAlternateAnchors(t *testing.T) {
	m, _, wakes := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	m.advancePane(p, []byte("\x1b_Ga=t,i=1,s=1,v=1;AQIDBA==\x1b\\"))
	select {
	case <-wakes:
	case <-time.After(2 * time.Second):
		t.Fatal("decode did not wake owner")
	}
	_ = m.Drain(16)
	seedKittyAnchorHistory(t, p)
	p.terminal.SetCursor(1, 2)
	m.advancePane(p, []byte("\x1b_Ga=p,i=1,p=2,c=1,r=1;\x1b\\"))
	viewportTop := p.terminal.ScrollbackLines()
	primary := p.terminal.ImageProjection(viewportTop, p.terminal.Rows())
	if len(primary.Placements) != 1 || primary.Placements[0].ID != 2 || primary.Placements[0].Anchor.Row != 1 || primary.Placements[0].Anchor.Col != 2 {
		t.Fatalf("primary projection=%#v viewportTop=%d", primary, viewportTop)
	}

	p.terminal.SetAlternateScreenMode(true)
	p.terminal.SetCursor(1, 3)
	m.advancePane(p, []byte("\x1b_Ga=p,i=1,p=3,c=1,r=1;\x1b\\"))
	alternate := p.terminal.ImageProjection(99, p.terminal.Rows())
	if len(alternate.Placements) != 1 || alternate.Placements[0].ID != 3 || alternate.Placements[0].Anchor.Row != 1 || alternate.Placements[0].Anchor.Col != 3 {
		t.Fatalf("alternate projection=%#v", alternate)
	}
	if usage := p.imageStore.Usage(); usage.Images != 1 || usage.Placements != 2 {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestKittyAsyncPlacementRejectsCapturedAnchorAfterReflow(t *testing.T) {
	m, _, wakes := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	p.terminal.SetCursor(1, 2)
	m.advancePane(p, []byte("\x1b_Ga=T,i=1,p=1,c=1,r=1,s=1,v=1;AQIDBA==\x1b\\"))
	if len(m.kittyPending) != 1 {
		t.Fatalf("pending=%d", len(m.kittyPending))
	}
	if _, err := m.ResizeGrid(PixelRect{Width: 400, Height: 240}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wakes:
	case <-time.After(2 * time.Second):
		t.Fatal("decode did not wake owner")
	}
	_ = m.Drain(16)
	if _, ok := p.imageStore.ResourceRef(1); ok || len(p.terminal.ImageProjection(p.terminal.ScrollbackLines(), p.terminal.Rows()).Placements) != 0 {
		t.Fatal("completion published a stale pre-reflow anchor")
	}
	if usage := p.imageStore.Usage(); usage != (termimage.Usage{}) {
		t.Fatalf("usage=%#v", usage)
	}
}

func TestKittyAsyncPlacementRejectsCoordinateMutationAfterTerminator(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*pane)
		suffix string
		verify func(*testing.T, *pane)
	}{
		{
			name: "history ring eviction",
			setup: func(p *pane) {
				seedKittyAnchorHistory(t, p)
				p.terminal.SetScrollbackCapacity(1)
				p.terminal.SetCursor(p.terminal.Rows()-1, 2)
			},
			suffix: "\n",
		},
		{
			name: "alternate screen entry",
			setup: func(p *pane) {
				p.terminal.SetCursor(1, 2)
			},
			suffix: "\x1b[?1049h",
			verify: func(t *testing.T, p *pane) {
				t.Helper()
				if !p.terminal.AlternateScreenMode() {
					t.Fatal("alternate screen suffix was not applied")
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, _, wakes := newKittyRuntimeMux(t, true)
			p, _ := m.sessions.lookup(1)
			test.setup(p)
			before := p.terminal.ImageAnchorGeneration()
			input := []byte("\x1b_Ga=T,i=1,p=1,c=1,r=1,s=1,v=1;AQIDBA==\x1b\\" + test.suffix)
			m.advancePane(p, input)
			if test.verify != nil {
				test.verify(t, p)
			}
			if got := p.terminal.ImageAnchorGeneration(); got == before {
				t.Fatal("coordinate mutation did not invalidate captured anchor")
			}
			select {
			case <-wakes:
			case <-time.After(2 * time.Second):
				t.Fatal("decode did not wake owner")
			}
			_ = m.Drain(16)
			if _, ok := p.imageStore.ResourceRef(1); ok {
				t.Fatal("completion published after coordinate mutation")
			}
			if usage := p.imageStore.Usage(); usage != (termimage.Usage{}) {
				t.Fatalf("usage=%#v", usage)
			}
		})
	}
}

func TestKittyRuntimeUsesWorkerCompletionBoundary(t *testing.T) {
	deadlineNanos := int64(termimage.HardAcceptanceDeadline)
	for _, test := range []struct {
		name     string
		finished int64
		want     kitty.ReplyCode
	}{
		{"before", deadlineNanos - 1, kitty.ReplyFailed},
		{"equal", deadlineNanos, kitty.ReplyTimeout},
		{"after", deadlineNanos + 1, kitty.ReplyTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := termimage.DefaultLimits()
			factory := &fakeFactory{}
			var nanos atomic.Int64
			wakes := make(chan struct{}, 1)
			m := New(factory, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true, Now: func() time.Time { return time.Unix(0, nanos.Load()) }, Wake: func() { wakes <- struct{}{} }})
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
			owner := kittyDecodeOwner{paneID: 1, pane: p, token: 1, replySlot: slot, hasSlot: true, acceptUntil: time.Unix(0, deadlineNanos)}
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			job := &decodeSchedulerTestJob{started: started, release: release}
			if err = m.imageScheduler.submitKitty(kittyDecodeWork{owner: owner, job: job}); err != nil {
				t.Fatal(err)
			}
			m.kittyPending[1] = owner
			awaitSchedulerSignals(t, started, 1)
			nanos.Store(test.finished)
			close(release)
			awaitSchedulerSignals(t, wakes, 1)
			if test.finished < deadlineNanos {
				nanos.Store(deadlineNanos)
			}
			_ = m.Drain(8)
			want := owner.plan.Encode(test.want)
			if got := factory.sessions[0].written(); !bytes.Equal(got, want) {
				t.Fatalf("wire=%q want=%q", got, want)
			}
			if len(m.kittyPending) != 0 {
				t.Fatal("pending owner survived drain")
			}
		})
	}
}

type transferHoldingDecodeJob struct {
	transfer *termimage.CandidateTransfer
	started  chan<- struct{}
	release  <-chan struct{}
}

func (j *transferHoldingDecodeJob) Run(ctx context.Context) *kitty.DecodeResult {
	if j.started != nil {
		j.started <- struct{}{}
	}
	select {
	case <-j.release:
	case <-ctx.Done():
	}
	j.transfer.Close()
	return &kitty.DecodeResult{Failure: kitty.ReplyCancelled}
}

func (j *transferHoldingDecodeJob) Close() {
	if j != nil && j.transfer != nil {
		j.transfer.Close()
	}
}

func TestKittyExpiryRetainsPaneActivityUntilWorkerReturnAndCleanup(t *testing.T) {
	limits := termimage.DefaultLimits()
	var nanos atomic.Int64
	m := New(&fakeFactory{}, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true, Now: func() time.Time { return time.Unix(0, nanos.Load()) }})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	p, _ := m.sessions.lookup(1)
	owner := kittyDecodeOwner{paneID: 1, pane: p, token: 1, acceptUntil: time.Unix(0, int64(termimage.HardAcceptanceDeadline))}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	transfer, err := p.imageStore.BeginTransfer(termimage.Header{Transfer: 1, Image: 1})
	if err != nil {
		t.Fatal(err)
	}
	job := &transferHoldingDecodeJob{transfer: transfer, started: started, release: release}
	if err = m.imageScheduler.submitKitty(kittyDecodeWork{owner: owner, job: job}); err != nil {
		t.Fatal(err)
	}
	m.kittyPending[1] = owner
	awaitSchedulerSignals(t, started, 1)
	nanos.Store(int64(termimage.HardAcceptanceDeadline))
	_ = m.expireKitty(time.Unix(0, nanos.Load()))
	if usage := p.imageStore.Usage(); usage.PendingTransfers != 1 {
		t.Fatalf("expiry released running transfer: %#v", usage)
	}
	duplicate := &decodeSchedulerTestJob{}
	if err = m.imageScheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}, job: duplicate}); !errors.Is(err, errKittyDecodePaneActive) || duplicate.closes.Load() != 1 {
		t.Fatalf("duplicate err=%v closes=%d", err, duplicate.closes.Load())
	}
	close(release)
	completion := awaitSchedulerCompletion(t, m.imageScheduler)
	_ = m.applyKittyCompletion(completion)
	m.imageScheduler.finish(completion.Owner.paneID)
	if usage := p.imageStore.Usage(); usage.PendingTransfers != 0 {
		t.Fatalf("worker cleanup leaked transfer: %#v", usage)
	}
	accepted := &decodeSchedulerTestJob{}
	if err = m.imageScheduler.submitKitty(kittyDecodeWork{owner: kittyDecodeOwner{paneID: 1}, job: accepted}); err != nil {
		t.Fatalf("pane not released after cleanup: %v", err)
	}
}

func TestKittyRuntimeTypedNilCompletionCancelsWithoutPanic(t *testing.T) {
	m, session, _ := newKittyRuntimeMux(t, true)
	p, _ := m.sessions.lookup(1)
	slot, ok := p.reserveImageReply()
	if !ok {
		t.Fatal("reserve")
	}
	owner := kittyDecodeOwner{paneID: 1, pane: p, token: 99, replySlot: slot, hasSlot: true, acceptUntil: time.Now().Add(time.Second)}
	m.kittyPending[99] = owner
	_ = m.applyKittyCompletion(kittyDecodeCompletion{Owner: owner, FinishedAt: owner.acceptUntil.Add(-time.Nanosecond)})
	if got, want := session.written(), owner.plan.Encode(kitty.ReplyCancelled); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
}

func TestKittyRuntimeQueuedLateCandidateCannotPublish(t *testing.T) {
	limits := termimage.DefaultLimits()
	factory := &fakeFactory{}
	var nanos atomic.Int64
	wakes := make(chan struct{}, 8)
	m := New(factory, Options{IngressCapacity: 8, ImageLimits: &limits, KittyEnabled: true, Now: func() time.Time { return time.Unix(0, nanos.Load()) }, Wake: func() { wakes <- struct{}{} }})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	started := make(chan struct{}, kittyDecodeWorkerCount)
	release := make(chan struct{})
	for index := 0; index < kittyDecodeWorkerCount; index++ {
		owner := kittyDecodeOwner{paneID: PaneID(index + 100)}
		if err = m.imageScheduler.submitKitty(kittyDecodeWork{owner: owner, job: &decodeSchedulerTestJob{started: started, release: release}}); err != nil {
			t.Fatal(err)
		}
	}
	awaitSchedulerSignals(t, started, kittyDecodeWorkerCount)
	p, _ := m.sessions.lookup(1)
	m.advancePane(p, []byte("\x1b_Ga=t,i=1,s=1,v=1;AQIDBA==\x1b\\"))
	if len(m.kittyPending) != 1 || p.imageStore.Usage().PendingTransfers != 1 {
		t.Fatalf("queued decode ownership pending=%d usage=%#v", len(m.kittyPending), p.imageStore.Usage())
	}
	nanos.Store(int64(termimage.HardAcceptanceDeadline) + 1)
	close(release)
	awaitSchedulerSignals(t, wakes, kittyDecodeWorkerCount+1)
	_ = m.Drain(16)
	if _, ok := p.imageStore.ResourceRef(1); ok {
		t.Fatal("late candidate published")
	}
	if usage := p.imageStore.Usage(); usage != (termimage.Usage{}) {
		t.Fatalf("late candidate ownership leaked: %#v", usage)
	}
	want := (kitty.ReplyPlan{}).Encode(kitty.ReplyTimeout)
	if got := factory.sessions[0].written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
}
