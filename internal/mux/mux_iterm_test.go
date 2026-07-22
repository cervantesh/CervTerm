package mux

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/itermimage"
	"cervterm/internal/kitty"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

func newITermRuntimeMux(t *testing.T, kittyEnabled, sixelEnabled, itermEnabled bool, limits *termimage.Limits, now func() time.Time) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	factory := &fakeFactory{}
	wakes := make(chan struct{}, 64)
	m := New(factory, Options{
		IngressCapacity: 8, ImageLimits: limits, KittyEnabled: kittyEnabled, SixelEnabled: sixelEnabled, ITermEnabled: itermEnabled, Now: now,
		Wake: func() {
			select {
			case wakes <- struct{}{}:
			default:
			}
		},
	})
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, factory.sessions[0], wakes
}

func defaultITermRuntimeMux(t *testing.T) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	limits := termimage.DefaultLimits()
	return newITermRuntimeMux(t, false, false, true, &limits, nil)
}

func itermRuntimePNG(t testing.TB, width, height int) ([]byte, []byte) {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(x*17 + y*3), G: uint8(x*5 + y*29), B: uint8(x*11 + y*7), A: uint8(255 - (x+y)%17)})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes(), append([]byte(nil), pixels.Pix...)
}

func itermRuntimeFrame(raw []byte, sizing string, terminator byte) string {
	metadata := "File=inline=1;size=" + strconv.Itoa(len(raw))
	if sizing != "" {
		metadata += ";" + sizing
	}
	frame := "\x1b]1337;" + metadata + ":" + base64.StdEncoding.EncodeToString(raw)
	if terminator == 0x07 {
		return frame + "\x07"
	}
	return frame + "\x1b\\"
}

func drainITermCompletion(t *testing.T, m *Mux, wakes <-chan struct{}) []Event {
	t.Helper()
	awaitSchedulerSignals(t, wakes, 1)
	return m.Drain(16)
}

func itermOwnerForTest(t *testing.T, m *Mux) itermDecodeOwner {
	t.Helper()
	if len(m.itermPending) != 1 {
		t.Fatalf("pending=%d want=1", len(m.itermPending))
	}
	for _, owner := range m.itermPending {
		return owner
	}
	return itermDecodeOwner{}
}

func TestITermProgrammaticOptionAllProtocolCombinations(t *testing.T) {
	limits := termimage.DefaultLimits()
	for mask := 0; mask < 8; mask++ {
		kittyEnabled := mask&1 != 0
		sixelEnabled := mask&2 != 0
		itermEnabled := mask&4 != 0
		t.Run(fmt.Sprintf("mask-%d", mask), func(t *testing.T) {
			m, session, _ := newITermRuntimeMux(t, kittyEnabled, sixelEnabled, itermEnabled, &limits, nil)
			p, _ := m.sessions.lookup(1)
			wantScheduler := kittyEnabled || sixelEnabled || itermEnabled
			if (m.imageScheduler != nil) != wantScheduler || p.imageStore == nil ||
				(p.kittyAdapter != nil) != kittyEnabled || (p.sixelAdapter != nil) != sixelEnabled || (p.itermAdapter != nil) != itermEnabled {
				t.Fatalf("scheduler=%v store=%v kitty=%v sixel=%v iterm=%v", m.imageScheduler != nil, p.imageStore != nil, p.kittyAdapter != nil, p.sixelAdapter != nil, p.itermAdapter != nil)
			}
			if !itermEnabled {
				raw, _ := itermRuntimePNG(t, 1, 1)
				m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
				if p.imageStore.Usage() != (termimage.Usage{}) || len(session.written()) != 0 {
					t.Fatalf("disabled iTerm retained or replied: usage=%#v wire=%q", p.imageStore.Usage(), session.written())
				}
			}
		})
	}

	m, _, _ := newITermRuntimeMux(t, true, true, true, nil, nil)
	p, _ := m.sessions.lookup(1)
	if m.imageScheduler != nil || p.imageStore != nil || p.kittyAdapter != nil || p.sixelAdapter != nil || p.itermAdapter != nil {
		t.Fatal("test-only flags activated without validated image limits")
	}
}

func TestImageControlSinkDispatchesKittySixelAndITerm(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, session, wakes := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)
	raw, _ := itermRuntimePNG(t, 2, 2)

	m.advancePane(p, []byte("\x1b_Ga=d,d=A\x1b\\"))
	if got, want := session.written(), []byte("\x1b_Ga=d;OK\x1b\\"); !bytes.Equal(got, want) {
		t.Fatalf("Kitty route wire=%q want=%q", got, want)
	}
	m.advancePane(p, []byte(sixelRuntimeFrame))
	sixelOwner := sixelOwnerForTest(t, m)
	drainSixelCompletion(t, m, wakes)
	if _, ok := p.imageStore.ResourceRef(sixelOwner.image); !ok {
		t.Fatal("DCS Sixel route did not publish")
	}
	m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
	itermOwner := itermOwnerForTest(t, m)
	drainITermCompletion(t, m, wakes)
	if _, ok := p.imageStore.ResourceRef(itermOwner.image); !ok {
		t.Fatal("selected OSC 1337 iTerm route did not publish")
	}
	if got, want := session.written(), []byte("\x1b_Ga=d;OK\x1b\\"); !bytes.Equal(got, want) {
		t.Fatalf("Phase 14 routes changed Kitty wire=%q", got)
	}
}

func TestITermRuntimeSuccessCapturesOwnerSizeSpanAnchorAndNoReply(t *testing.T) {
	m, session, wakes := defaultITermRuntimeMux(t)
	p, _ := m.sessions.lookup(1)
	raw, wantRGBA := itermRuntimePNG(t, 13, 17)
	seedKittyAnchorHistory(t, p)
	p.terminal.SetCursor(1, 2)
	wantAnchor := p.terminal.ImageCursorAnchor()
	wantGeneration := p.terminal.ImageGeneration()
	wantAnchorGeneration := p.terminal.ImageAnchorGeneration()
	frame := itermRuntimeFrame(raw, "width=3", 0x07)
	m.advancePane(p, []byte(frame+"\x1b[1;1H"))
	owner := itermOwnerForTest(t, m)
	wantMetadata := itermimage.Metadata{Size: uint64(len(raw)), Axis: itermimage.SizingWidth, Cells: 3, PreserveAspectRatio: true}
	if owner.pane != p || owner.model != m.model || owner.store != p.imageStore || owner.storeEpoch != p.imageStore.Epoch() ||
		owner.imageGeneration != wantGeneration || owner.anchorGen != wantAnchorGeneration || owner.reflowGen != p.reflowGen || owner.anchor != wantAnchor ||
		owner.metrics != (CellMetrics{CellWidth: 8, CellHeight: 16}) || owner.image != termimage.MinInternalImageID ||
		owner.placement != termimage.MinInternalPlacementID || owner.metadata != wantMetadata ||
		owner.startedAt.IsZero() || owner.acceptUntil.Sub(owner.startedAt) != termimage.HardAcceptanceDeadline {
		t.Fatalf("captured owner=%#v", owner)
	}
	events := drainITermCompletion(t, m, wakes)
	if len(events) != 1 || events[0].Kind != PaneDirty || events[0].Pane != p.id {
		t.Fatalf("completion events=%#v", events)
	}
	ref, ok := p.imageStore.ResourceRef(owner.image)
	if !ok {
		t.Fatal("iTerm resource not committed")
	}
	resource, ok := m.AcquireImageResource(p.id, ref)
	if !ok || resource.Width != 13 || resource.Height != 17 || resource.Stride != 52 || !bytes.Equal(resource.RGBA, wantRGBA) {
		t.Fatalf("resource=%#v ok=%v", resource, ok)
	}
	viewportTop := p.terminal.ScrollbackLines()
	projection := p.terminal.ImageProjection(viewportTop, p.terminal.Rows())
	if len(projection.Placements) != 1 {
		t.Fatalf("projection=%#v", projection)
	}
	placement := projection.Placements[0]
	if placement.ID != owner.placement || placement.Anchor.Row != wantAnchor.Row-int64(viewportTop) || placement.Anchor.Col != wantAnchor.Col ||
		placement.Cols != 3 || placement.Rows != 2 || placement.Opacity != 255 {
		t.Fatalf("placement=%#v wantAnchor=%#v viewportTop=%d", placement, wantAnchor, viewportTop)
	}
	if p.terminal.CursorRow() != 0 || p.terminal.CursorCol() != 0 {
		t.Fatalf("completion moved cursor to (%d,%d)", p.terminal.CursorRow(), p.terminal.CursorCol())
	}
	if len(session.written()) != 0 || p.replies.stats.ReservedSlots != 0 {
		t.Fatalf("iTerm replied or reserved a reply: wire=%q stats=%#v", session.written(), p.replies.stats)
	}
	if p.snapshot.ImageGeneration != p.terminal.ImageGeneration() {
		t.Fatal("successful completion did not capture fresh image generation")
	}
}

func TestITermCaptureUsesFreshCoreImageGeneration(t *testing.T) {
	m, _, wakes := defaultITermRuntimeMux(t)
	p, _ := m.sessions.lookup(1)
	candidate, err := p.imageStore.NewDecodedCandidate(7, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.SealWrites(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.terminal.CommitImage(core.ImageCommit{Candidate: candidate}); err != nil {
		t.Fatal(err)
	}
	if p.snapshot.ImageGeneration == p.terminal.ImageGeneration() {
		t.Fatal("test requires a stale render snapshot")
	}
	raw, _ := itermRuntimePNG(t, 1, 1)
	m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
	owner := itermOwnerForTest(t, m)
	if owner.imageGeneration != p.terminal.ImageGeneration() {
		t.Fatalf("captured=%d fresh=%d snapshot=%d", owner.imageGeneration, p.terminal.ImageGeneration(), p.snapshot.ImageGeneration)
	}
	drainITermCompletion(t, m, wakes)
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("fresh owner generation was rejected")
	}
}

func TestITermRuntimeRejectsStaleResetRISReflowMetricsAndPointers(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Mux, *pane)
	}{
		{name: "anchor content", mutate: func(t *testing.T, m *Mux, p *pane) { p.terminal.PutRune('x') }},
		{name: "reset epoch", mutate: func(t *testing.T, m *Mux, p *pane) { p.terminal.ResetImages() }},
		{name: "RIS", mutate: func(t *testing.T, m *Mux, p *pane) { m.advancePane(p, []byte("\x1bc")) }},
		{name: "reflow", mutate: func(t *testing.T, m *Mux, p *pane) {
			if _, err := m.ResizeGrid(PixelRect{Width: 400, Height: 240}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "metrics", mutate: func(t *testing.T, m *Mux, p *pane) { m.paneMetrics[p.id] = CellMetrics{CellWidth: 9, CellHeight: 16} }},
		{name: "model pointer", mutate: func(t *testing.T, m *Mux, p *pane) { m.model = NewModel() }},
		{name: "store pointer", mutate: func(t *testing.T, m *Mux, p *pane) {
			replacement := termimage.NewStore(m.imageBudget, m.imageLimits)
			if replacement == nil {
				t.Fatal("replacement store")
			}
			t.Cleanup(replacement.Close)
			p.imageStore = replacement
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, session, wakes := defaultITermRuntimeMux(t)
			p, _ := m.sessions.lookup(1)
			raw, _ := itermRuntimePNG(t, 2, 2)
			m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
			owner := itermOwnerForTest(t, m)
			awaitSchedulerSignals(t, wakes, 1)
			test.mutate(t, m, p)
			if events := m.Drain(16); len(events) != 0 {
				t.Fatalf("stale completion events=%#v", events)
			}
			if _, ok := owner.store.ResourceRef(owner.image); ok || owner.store.Usage() != (termimage.Usage{}) {
				t.Fatalf("stale completion published or leaked usage=%#v", owner.store.Usage())
			}
			if len(session.written()) != 0 {
				t.Fatalf("stale completion replied %q", session.written())
			}
		})
	}
}

func TestITermRuntimeAllowsHiddenAndTransferredPaneCompletion(t *testing.T) {
	raw, _ := itermRuntimePNG(t, 2, 2)
	t.Run("hidden tab", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		awaitSchedulerSignals(t, wakes, 1)
		if _, _, _, err := m.SpawnTab(SpawnSpec{}, CellMetrics{CellWidth: 8, CellHeight: 16}, "hidden source"); err != nil {
			t.Fatal(err)
		}
		m.Drain(16)
		if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
			t.Fatal("hidden pane completion was rejected")
		}
	})

	t.Run("whole tab transfer", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		awaitSchedulerSignals(t, wakes, 1)
		destination, _ := addRuntimeTestWindow(t, m, CellMetrics{CellWidth: 8, CellHeight: 16})
		if err := m.model.ActivateWindow(1); err != nil {
			t.Fatal(err)
		}
		if _, err := m.TransferTabBetweenWindows(TabTransferRequest{
			SourceWindow: 1, DestinationWindow: destination.ID, Tab: 1, Position: 1,
			SourceBounds: m.bounds, DestinationBounds: m.bounds, Resolve: m.resolveMetrics,
		}); err != nil {
			t.Fatal(err)
		}
		m.Drain(16)
		if after, ok := m.sessions.lookup(p.id); !ok || after != p || after.imageStore != owner.store {
			t.Fatal("transfer changed pane/store ownership")
		}
		if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
			t.Fatal("transferred pane completion was rejected")
		}
	})
}

func TestITermRuntimeCloseReleasesAdapterOutcomesAndBufferedCompletion(t *testing.T) {
	raw, _ := itermRuntimePNG(t, 2, 2)
	payload := []byte("File=inline=1;size=" + strconv.Itoa(len(raw)) + ":" + base64.StdEncoding.EncodeToString(raw))
	t.Run("open adapter transfer", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		outcome := p.itermAdapter.Advance(time.Now(), itermimage.OSCEvent{Data: payload})
		if outcome != (itermimage.Outcome{}) || p.imageStore.Usage().PendingTransfers != 1 {
			t.Fatalf("outcome=%#v usage=%#v", outcome, p.imageStore.Usage())
		}
		if _, err := m.ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
		if !p.imageStore.Closed() || m.imageBudget.Usage() != (termimage.Usage{}) || len(p.itermOutcomes) != 0 {
			t.Fatalf("closed=%v usage=%#v outcomes=%d", p.imageStore.Closed(), m.imageBudget.Usage(), len(p.itermOutcomes))
		}
	})

	t.Run("queued sealed outcome", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		outcome := p.itermAdapter.Advance(time.Now(), itermimage.OSCEvent{Data: payload, Final: true})
		if outcome.Command == nil || p.imageStore.Usage().PendingTransfers != 1 {
			t.Fatalf("outcome=%#v usage=%#v", outcome, p.imageStore.Usage())
		}
		p.itermOutcomes = append(p.itermOutcomes, outcome)
		if _, err := m.ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
		if usage := m.imageBudget.Usage(); usage != (termimage.Usage{}) {
			t.Fatalf("queued outcome leaked usage=%#v", usage)
		}
	})

	t.Run("buffered completion", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		awaitSchedulerSignals(t, wakes, 1)
		if _, err := m.ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
		if events := m.Drain(16); len(events) != 0 {
			t.Fatalf("closed completion events=%#v", events)
		}
		if usage := m.imageBudget.Usage(); usage != (termimage.Usage{}) {
			t.Fatalf("close leaked usage=%#v", usage)
		}
	})
}

func TestITermRuntimeAcceptsOnTimeBufferedResultAtDeadline(t *testing.T) {
	limits := termimage.DefaultLimits()
	var nanos atomic.Int64
	m, _, wakes := newITermRuntimeMux(t, false, false, true, &limits, func() time.Time { return time.Unix(0, nanos.Load()) })
	p, _ := m.sessions.lookup(1)
	raw, _ := itermRuntimePNG(t, 2, 2)
	m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
	owner := itermOwnerForTest(t, m)
	awaitSchedulerSignals(t, wakes, 1)
	nanos.Store(owner.acceptUntil.UnixNano())
	m.Drain(16)
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("on-time buffered result was expired before dispatch")
	}
}

type blockingITermRuntimeJob struct {
	started chan<- struct{}
	release <-chan struct{}
	closes  atomic.Int32
}

func (j *blockingITermRuntimeJob) Run(ctx context.Context) *itermimage.DecodeResult {
	if j.started != nil {
		j.started <- struct{}{}
	}
	select {
	case <-j.release:
	case <-ctx.Done():
	}
	return &itermimage.DecodeResult{Failure: itermimage.FailureCancelled}
}

func (j *blockingITermRuntimeJob) Close() { j.closes.Add(1) }

func TestITermExpiryDoesNotFinishPaneSlotBeforeWorkerCleanup(t *testing.T) {
	limits := termimage.DefaultLimits()
	var nanos atomic.Int64
	m, session, _ := newITermRuntimeMux(t, false, false, true, &limits, func() time.Time { return time.Unix(0, nanos.Load()) })
	p, _ := m.sessions.lookup(1)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	owner := itermDecodeOwner{paneID: p.id, pane: p, token: 1, acceptUntil: time.Unix(0, int64(termimage.HardAcceptanceDeadline))}
	job := &blockingITermRuntimeJob{started: started, release: release}
	if err := m.imageScheduler.submitITerm(itermDecodeWork{owner: owner, job: job}); err != nil {
		t.Fatal(err)
	}
	m.itermPending[owner.token] = owner
	awaitSchedulerSignals(t, started, 1)
	nanos.Store(owner.acceptUntil.UnixNano())
	m.expireImages(time.Unix(0, nanos.Load()))
	if len(m.itermPending) != 0 {
		t.Fatal("expired owner remained pending")
	}
	duplicate := &blockingITermRuntimeJob{release: make(chan struct{})}
	if err := m.imageScheduler.submitITerm(itermDecodeWork{owner: itermDecodeOwner{paneID: p.id}, job: duplicate}); err != errKittyDecodePaneActive || duplicate.closes.Load() != 1 {
		t.Fatalf("duplicate err=%v closes=%d", err, duplicate.closes.Load())
	}
	close(release)
	completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
	if events := m.applyImageCompletion(completion); len(events) != 0 {
		t.Fatalf("expired completion events=%#v", events)
	}
	accepted := &blockingITermRuntimeJob{release: closedSignal()}
	if err := m.imageScheduler.submitITerm(itermDecodeWork{owner: itermDecodeOwner{paneID: p.id}, job: accepted}); err != nil {
		t.Fatalf("pane key not released after completion cleanup: %v", err)
	}
	completion = awaitImageSchedulerCompletion(t, m.imageScheduler)
	completion.Close()
	m.imageScheduler.finish(completion.Key)
	if len(session.written()) != 0 {
		t.Fatalf("expiry emitted reply %q", session.written())
	}
}

func closedSignal() <-chan struct{} {
	ready := make(chan struct{})
	close(ready)
	return ready
}

func TestITermCompletionRejectsForgedIDsCandidateMetadataSpanAndDimensions(t *testing.T) {
	for _, name := range []string{"image id", "placement id", "candidate", "metadata", "span", "dimensions"} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := defaultITermRuntimeMux(t)
			p, _ := m.sessions.lookup(1)
			raw, _ := itermRuntimePNG(t, 2, 2)
			m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
			completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
			typed, ok := decodeITermCompletion(completion)
			if !ok || typed.Result == nil || typed.Result.Candidate == nil {
				t.Fatal("missing iTerm completion")
			}
			owner := typed.Owner
			switch name {
			case "image id":
				owner.image = 1
			case "placement id":
				owner.placement = 1
			case "candidate":
				typed.Result.Candidate.Close()
				candidate, err := p.imageStore.NewDecodedCandidate(owner.image+1, 2, 2)
				if err != nil {
					t.Fatal(err)
				}
				if err = candidate.SealWrites(); err != nil {
					t.Fatal(err)
				}
				typed.Result.Candidate = candidate
			case "metadata":
				owner.metadata.PreserveAspectRatio = false
			case "span":
				typed.Result.Span.Cols++
			case "dimensions":
				typed.Result.Candidate.Close()
				candidate, err := p.imageStore.NewDecodedCandidate(owner.image, 9, 1)
				if err != nil {
					t.Fatal(err)
				}
				if err = candidate.SealWrites(); err != nil {
					t.Fatal(err)
				}
				typed.Result.Candidate = candidate
			}
			m.itermPending[owner.token] = owner
			completion.Owner.value = owner
			completion.Result = typed.Result
			if events := m.applyImageCompletion(completion); len(events) != 0 {
				t.Fatalf("forged completion events=%#v", events)
			}
			if _, ok := p.imageStore.ResourceRef(typed.Owner.image); ok || p.imageStore.Usage() != (termimage.Usage{}) {
				t.Fatalf("forged completion published or leaked usage=%#v", p.imageStore.Usage())
			}
		})
	}
}

func TestITermAtomicRollbackAndEphemeralLifecycle(t *testing.T) {
	raw, _ := itermRuntimePNG(t, 2, 2)
	t.Run("placement reservation rollback", func(t *testing.T) {
		limits := termimage.DefaultLimits()
		limits.Placements = 1
		m, session, wakes := newITermRuntimeMux(t, false, false, true, &limits, nil)
		p, _ := m.sessions.lookup(1)
		candidate, err := p.imageStore.NewDecodedCandidate(1, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err = candidate.SealWrites(); err != nil {
			t.Fatal(err)
		}
		if _, err = p.terminal.CommitImage(core.ImageCommit{Candidate: candidate, Placement: &termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Col: 10}, Cols: 1, Rows: 1}}); err != nil {
			t.Fatal(err)
		}
		p.capture()
		beforeUsage := p.imageStore.Usage()
		beforeGeneration := p.terminal.ImageGeneration()
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		if events := drainITermCompletion(t, m, wakes); len(events) != 0 {
			t.Fatalf("failed commit events=%#v", events)
		}
		if _, ok := p.imageStore.ResourceRef(owner.image); ok || p.imageStore.Usage() != beforeUsage || p.terminal.ImageGeneration() != beforeGeneration {
			t.Fatalf("rollback resource=%v usage=%#v generation=%d", ok, p.imageStore.Usage(), p.terminal.ImageGeneration())
		}
		if projection := p.terminal.ImageProjection(0, p.terminal.Rows()); len(projection.Placements) != 1 || projection.Placements[0].ID != 1 {
			t.Fatalf("rollback projection=%#v", projection)
		}
		if len(session.written()) != 0 {
			t.Fatalf("failed commit replied %q", session.written())
		}
	})

	t.Run("final placement retires resource", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		drainITermCompletion(t, m, wakes)
		if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
			t.Fatal("ephemeral resource missing before retirement")
		}
		m.advancePane(p, []byte("X"))
		if _, ok := p.imageStore.ResourceRef(owner.image); ok || p.imageStore.Usage() != (termimage.Usage{}) || len(p.terminal.ImageProjection(0, p.terminal.Rows()).Placements) != 0 {
			t.Fatalf("ephemeral resource survived final placement usage=%#v", p.imageStore.Usage())
		}
	})
}

func TestITermRuntimeRestorePublishesFreshAdapterAndStore(t *testing.T) {
	limits := termimage.DefaultLimits()
	factory := &restoreTestFactory{}
	wakes := make(chan struct{}, 64)
	m := New(factory, Options{IngressCapacity: 64, ImageLimits: &limits, ITermEnabled: true, Wake: func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	}})
	t.Cleanup(func() { _ = m.Shutdown() })
	candidate, err := m.PrepareRestore(blueprintFromSnapshot(t, restoreSnapshot()), restoreGeometries())
	if err != nil {
		t.Fatal(err)
	}
	for _, pane := range candidate.panes {
		if pane.imageStore == nil || pane.itermAdapter == nil {
			t.Fatalf("restored pane %d missing iTerm state", pane.id)
		}
	}
	if _, err = m.CommitRestore(candidate); err != nil {
		t.Fatal(err)
	}
	pane := candidate.panes[0]
	raw, _ := itermRuntimePNG(t, 2, 2)
	m.advancePane(pane, []byte(itermRuntimeFrame(raw, "", 0x07)))
	owner := itermOwnerForTest(t, m)
	drainITermCompletion(t, m, wakes)
	if _, ok := pane.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("restored pane did not publish iTerm resource")
	}
}

func TestITermAndSixelDoNotReserveReplySlotsOrPerturbKittyDSROSCOrder(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, session, wakes := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)
	p.terminal.SetPaletteFG(core.RGB{R: 0x12, G: 0x34, B: 0x56})
	raw, _ := itermRuntimePNG(t, 1, 1)
	input := "\x1b_Ga=t,i=1,s=1,v=1;AQIDBA==\x1b\\" + sixelRuntimeFrame + itermRuntimeFrame(raw, "", 0x07) + "\x1b[5n\x1b]10;?\x1b\\"
	m.advancePane(p, []byte(input))
	if got := session.written(); len(got) != 0 {
		t.Fatalf("later replies overtook Kitty: %q", got)
	}
	if p.replies.stats.ReservedSlots != 1 {
		t.Fatalf("Phase 14 reserved Kitty reply slots: %#v", p.replies.stats)
	}
	if len(m.sixelPending) != 0 || len(m.itermPending) != 0 || len(m.kittyPending) != 1 {
		t.Fatalf("pending kitty=%d sixel=%d iterm=%d", len(m.kittyPending), len(m.sixelPending), len(m.itermPending))
	}
	awaitSchedulerSignals(t, wakes, 1)
	m.Drain(16)
	want := []byte("\x1b_Ga=t;OK\x1b\\\x1b[0n\x1b]10;rgb:1212/3434/5656\x1b\\")
	if got := session.written(); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
}

func TestMixedImageAdaptersShareExactPaneAndProcessPendingBudgets(t *testing.T) {
	process := termimage.NewProcessBudget()
	var closeAll []func()
	for paneIndex := 0; paneIndex < 4; paneIndex++ {
		store := termimage.NewStore(process, termimage.DefaultLimits())
		kittyAdapter := kitty.NewAdapter(store)
		closeAll = append(closeAll, kittyAdapter.Close)
		for slot := 0; slot < int(termimage.HardPendingTransfersPerPane); slot++ {
			switch slot % 3 {
			case 0:
				adapter := kittyAdapter
				id := paneIndex*16 + slot + 1
				out := adapter.Advance(time.Now(), kitty.APCEvent{Data: []byte(fmt.Sprintf("Ga=T,i=%d,p=%d,c=1,r=1,s=1,v=1;AQIDBA==", id, id)), Final: true})
				if out.Command == nil || out.Failure != kitty.ReplyNone {
					t.Fatalf("kitty pane=%d slot=%d outcome=%#v", paneIndex, slot, out)
				}
				command := out.Command
				closeAll = append(closeAll, command.Transfer.Close)
			case 1:
				adapter := sixel.NewAdapter(store)
				out := adapter.Advance(time.Now(), sixel.DCSEvent{Data: []byte("\"1;1;2;6#1~?"), Final: true})
				if out.Command == nil || out.Failure != sixel.FailureNone {
					t.Fatalf("sixel pane=%d slot=%d outcome=%#v", paneIndex, slot, out)
				}
				command := out.Command
				closeAll = append(closeAll, func() { command.Close(); adapter.Close() })
			case 2:
				adapter := itermimage.NewAdapter(store)
				out := adapter.Advance(time.Now(), itermimage.OSCEvent{Data: []byte("File=inline=1;size=3:AAAA"), Final: true})
				if out.Command == nil || out.Failure != itermimage.FailureNone {
					t.Fatalf("iterm pane=%d slot=%d outcome=%#v", paneIndex, slot, out)
				}
				command := out.Command
				closeAll = append(closeAll, func() { command.Close(); adapter.Close() })
			}
		}
		if got := store.Usage().PendingTransfers; got != termimage.HardPendingTransfersPerPane {
			t.Fatalf("pane %d pending=%d", paneIndex, got)
		}
		extra := itermimage.NewAdapter(store)
		if out := extra.Advance(time.Now(), itermimage.OSCEvent{Data: []byte("File=inline=1;size=3:AAAA")}); out.Failure != itermimage.FailureLimit {
			t.Fatalf("pane %d overflow=%#v", paneIndex, out)
		}
		extra.Close()
	}
	if got := process.Usage().PendingTransfers; got != termimage.HardPendingTransfersProcess {
		t.Fatalf("process pending=%d want=%d", got, termimage.HardPendingTransfersProcess)
	}
	extraStore := termimage.NewStore(process, termimage.DefaultLimits())
	extra := itermimage.NewAdapter(extraStore)
	if out := extra.Advance(time.Now(), itermimage.OSCEvent{Data: []byte("File=inline=1;size=3:AAAA")}); out.Failure != itermimage.FailureLimit {
		t.Fatalf("process overflow=%#v", out)
	}
	extra.Close()
	for index := len(closeAll) - 1; index >= 0; index-- {
		closeAll[index]()
	}
	if process.Usage() != (termimage.Usage{}) {
		t.Fatalf("mixed pending ownership leaked: %#v", process.Usage())
	}
}

func TestITermRuntimeShutdownClearsAllProtocolPendingOwners(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, _, wakes := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)
	raw, _ := itermRuntimePNG(t, 2, 2)
	m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
	awaitSchedulerSignals(t, wakes, 1)
	if len(m.itermPending) != 1 {
		t.Fatalf("pending iterm=%d", len(m.itermPending))
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if len(m.kittyPending) != 0 || len(m.sixelPending) != 0 || len(m.itermPending) != 0 {
		t.Fatalf("pending survived shutdown kitty=%d sixel=%d iterm=%d", len(m.kittyPending), len(m.sixelPending), len(m.itermPending))
	}
	if usage := m.imageBudget.Usage(); usage != (termimage.Usage{}) {
		t.Fatalf("shutdown usage=%#v", usage)
	}
}
