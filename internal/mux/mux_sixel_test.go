package mux

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

const sixelRuntimeFrame = "\x1bPq\"1;1;2;6#1~?\x1b\\"

func newSixelRuntimeMux(t *testing.T, kittyEnabled, sixelEnabled bool, limits *termimage.Limits, now func() time.Time) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	factory := &fakeFactory{}
	wakes := make(chan struct{}, 64)
	options := Options{
		IngressCapacity: 8, ImageLimits: limits, KittyEnabled: kittyEnabled, SixelEnabled: sixelEnabled, Now: now,
		Wake: func() {
			select {
			case wakes <- struct{}{}:
			default:
			}
		},
	}
	m := New(factory, options)
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, factory.sessions[0], wakes
}

func defaultSixelRuntimeMux(t *testing.T) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	limits := termimage.DefaultLimits()
	return newSixelRuntimeMux(t, false, true, &limits, nil)
}

func drainSixelCompletion(t *testing.T, m *Mux, wakes <-chan struct{}) []Event {
	t.Helper()
	awaitSchedulerSignals(t, wakes, 1)
	return m.Drain(16)
}

func sixelOwnerForTest(t *testing.T, m *Mux) sixelDecodeOwner {
	t.Helper()
	if len(m.sixelPending) != 1 {
		t.Fatalf("pending=%d want=1", len(m.sixelPending))
	}
	for _, owner := range m.sixelPending {
		return owner
	}
	return sixelDecodeOwner{}
}

func TestSixelProgrammaticOptionAndAdapterIsolation(t *testing.T) {
	limits := termimage.DefaultLimits()
	for _, test := range []struct {
		name                     string
		limits                   *termimage.Limits
		kitty, sixel             bool
		wantScheduler, wantStore bool
		wantKitty, wantSixel     bool
	}{
		{name: "no limits", kitty: true, sixel: true},
		{name: "limits only", limits: &limits, wantStore: true},
		{name: "kitty only", limits: &limits, kitty: true, wantScheduler: true, wantStore: true, wantKitty: true},
		{name: "sixel only", limits: &limits, sixel: true, wantScheduler: true, wantStore: true, wantSixel: true},
		{name: "both", limits: &limits, kitty: true, sixel: true, wantScheduler: true, wantStore: true, wantKitty: true, wantSixel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, session, _ := newSixelRuntimeMux(t, test.kitty, test.sixel, test.limits, nil)
			p, _ := m.sessions.lookup(1)
			if (m.imageScheduler != nil) != test.wantScheduler || (p.imageStore != nil) != test.wantStore ||
				(p.kittyAdapter != nil) != test.wantKitty || (p.sixelAdapter != nil) != test.wantSixel {
				t.Fatalf("scheduler=%v store=%v kitty=%v sixel=%v", m.imageScheduler != nil, p.imageStore != nil, p.kittyAdapter != nil, p.sixelAdapter != nil)
			}
			if test.sixel && !test.kitty {
				m.advancePane(p, []byte("\x1b_Ga=d,d=A\x1b\\"))
				if len(session.written()) != 0 {
					t.Fatalf("isolated Sixel runtime emitted Kitty reply %q", session.written())
				}
			}
		})
	}
}

func TestImageControlSinkDispatchesKittyAPCAndSixelDCS(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, session, wakes := newSixelRuntimeMux(t, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)
	m.advancePane(p, []byte("\x1b_Ga=d,d=A\x1b\\"+sixelRuntimeFrame))
	if got, want := session.written(), []byte("\x1b_Ga=d;OK\x1b\\"); !bytes.Equal(got, want) {
		t.Fatalf("wire=%q want=%q", got, want)
	}
	owner := sixelOwnerForTest(t, m)
	drainSixelCompletion(t, m, wakes)
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("shared sink did not dispatch selected Sixel DCS")
	}
	if got, want := session.written(), []byte("\x1b_Ga=d;OK\x1b\\"); !bytes.Equal(got, want) {
		t.Fatalf("Sixel changed Kitty reply stream: %q", got)
	}
}

func TestSixelRuntimeSuccessCapturesAnchorPaletteAndNoReply(t *testing.T) {
	m, session, wakes := defaultSixelRuntimeMux(t)
	p, _ := m.sessions.lookup(1)
	seedKittyAnchorHistory(t, p)
	p.terminal.SetCursor(1, 2)
	p.terminal.SetPaletteIndex(1, core.RGB{R: 11, G: 22, B: 33})
	wantAnchor := p.terminal.ImageCursorAnchor()
	m.advancePane(p, []byte(sixelRuntimeFrame+"\x1b[1;1H"))
	owner := sixelOwnerForTest(t, m)
	if owner.pane != p || owner.model != m.model || owner.store != p.imageStore || owner.storeEpoch != p.imageStore.Epoch() ||
		owner.imageGeneration != p.terminal.ImageGeneration() || owner.anchor != wantAnchor || owner.anchorGen != p.terminal.ImageAnchorGeneration() ||
		owner.metrics != (CellMetrics{CellWidth: 8, CellHeight: 16}) || owner.image != termimage.MinInternalImageID ||
		owner.placement != termimage.MinInternalPlacementID || owner.raster != (sixel.Raster{Width: 2, Height: 6}) {
		t.Fatalf("captured owner=%#v", owner)
	}
	p.terminal.SetPaletteIndex(1, core.RGB{R: 90, G: 91, B: 92})
	events := drainSixelCompletion(t, m, wakes)
	if len(events) != 1 || events[0].Kind != PaneDirty || events[0].Pane != p.id {
		t.Fatalf("completion events=%#v", events)
	}
	ref, ok := p.imageStore.ResourceRef(owner.image)
	if !ok {
		t.Fatal("Sixel resource not committed")
	}
	resource, ok := m.AcquireImageResource(p.id, ref)
	if !ok || resource.Width != 2 || resource.Height != 6 || resource.Stride != 8 || len(resource.RGBA) != 48 {
		t.Fatalf("resource=%#v ok=%v", resource, ok)
	}
	if got := resource.RGBA[:4]; !bytes.Equal(got, []byte{11, 22, 33, 255}) {
		t.Fatalf("captured palette pixel=%v", got)
	}
	if got := resource.RGBA[4:8]; !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("unset pixel=%v", got)
	}
	viewportTop := p.terminal.ScrollbackLines()
	projection := p.terminal.ImageProjection(viewportTop, p.terminal.Rows())
	if len(projection.Placements) != 1 {
		t.Fatalf("projection=%#v", projection)
	}
	placement := projection.Placements[0]
	if placement.ID != owner.placement || placement.Anchor.Row != wantAnchor.Row-int64(viewportTop) || placement.Anchor.Col != wantAnchor.Col ||
		placement.Cols != 1 || placement.Rows != 1 || placement.Opacity != 255 {
		t.Fatalf("placement=%#v wantAnchor=%#v viewportTop=%d", placement, wantAnchor, viewportTop)
	}
	if p.terminal.CursorRow() != 0 || p.terminal.CursorCol() != 0 {
		t.Fatalf("completion moved cursor to (%d,%d)", p.terminal.CursorRow(), p.terminal.CursorCol())
	}
	if got := session.written(); len(got) != 0 {
		t.Fatalf("Sixel emitted reply %q", got)
	}
	if p.snapshot.ImageGeneration != p.terminal.ImageGeneration() {
		t.Fatal("successful completion did not capture the fresh image generation")
	}
}

func TestSixelCaptureUsesFreshCoreImageGeneration(t *testing.T) {
	m, _, wakes := defaultSixelRuntimeMux(t)
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
		t.Fatal("test requires an intentionally stale render snapshot")
	}
	m.advancePane(p, []byte(sixelRuntimeFrame))
	owner := sixelOwnerForTest(t, m)
	if owner.imageGeneration != p.terminal.ImageGeneration() {
		t.Fatalf("captured generation=%d fresh=%d snapshot=%d", owner.imageGeneration, p.terminal.ImageGeneration(), p.snapshot.ImageGeneration)
	}
	drainSixelCompletion(t, m, wakes)
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("fresh owner generation was not accepted")
	}
}

func TestSixelRuntimeRejectsStaleOwnerState(t *testing.T) {
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
			m, session, wakes := defaultSixelRuntimeMux(t)
			p, _ := m.sessions.lookup(1)
			m.advancePane(p, []byte(sixelRuntimeFrame))
			owner := sixelOwnerForTest(t, m)
			awaitSchedulerSignals(t, wakes, 1)
			test.mutate(t, m, p)
			if events := m.Drain(16); len(events) != 0 {
				t.Fatalf("stale completion events=%#v", events)
			}
			if _, ok := owner.store.ResourceRef(owner.image); ok {
				t.Fatal("stale completion published")
			}
			if usage := owner.store.Usage(); usage != (termimage.Usage{}) {
				t.Fatalf("stale ownership leaked: %#v", usage)
			}
			if len(session.written()) != 0 {
				t.Fatalf("stale completion replied %q", session.written())
			}
		})
	}
}

func TestSixelRuntimeAllowsHiddenAndTransferredPaneCompletion(t *testing.T) {
	t.Run("hidden tab", func(t *testing.T) {
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
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
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
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

func TestSixelRuntimeCloseReleasesAdapterAndBufferedCompletion(t *testing.T) {
	t.Run("open adapter transfer", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		outcome := p.sixelAdapter.Advance(time.Now(), sixel.DCSEvent{Data: []byte("\"1;1;2;6#1~?")})
		if outcome != (sixel.Outcome{}) || p.imageStore.Usage().PendingTransfers != 1 {
			t.Fatalf("outcome=%#v usage=%#v", outcome, p.imageStore.Usage())
		}
		if _, err := m.ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
		if !p.imageStore.Closed() || m.imageBudget.Usage() != (termimage.Usage{}) || len(p.sixelOutcomes) != 0 {
			t.Fatalf("closed=%v usage=%#v outcomes=%d", p.imageStore.Closed(), m.imageBudget.Usage(), len(p.sixelOutcomes))
		}
	})

	t.Run("queued sealed outcome", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		outcome := p.sixelAdapter.Advance(time.Now(), sixel.DCSEvent{Data: []byte("\"1;1;2;6#1~?"), Final: true})
		if outcome.Command == nil || p.imageStore.Usage().PendingTransfers != 1 {
			t.Fatalf("outcome=%#v usage=%#v", outcome, p.imageStore.Usage())
		}
		p.sixelOutcomes = append(p.sixelOutcomes, outcome)
		if _, err := m.ClosePane(p.id); err != nil {
			t.Fatal(err)
		}
		if usage := m.imageBudget.Usage(); usage != (termimage.Usage{}) {
			t.Fatalf("queued outcome leaked usage=%#v", usage)
		}
	})

	t.Run("buffered completion", func(t *testing.T) {
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(sixelRuntimeFrame))
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

func TestSixelRuntimeAcceptsOnTimeBufferedResultAtDeadline(t *testing.T) {
	limits := termimage.DefaultLimits()
	var nanos atomic.Int64
	m, _, wakes := newSixelRuntimeMux(t, false, true, &limits, func() time.Time { return time.Unix(0, nanos.Load()) })
	p, _ := m.sessions.lookup(1)
	m.advancePane(p, []byte(sixelRuntimeFrame))
	owner := sixelOwnerForTest(t, m)
	awaitSchedulerSignals(t, wakes, 1)
	nanos.Store(owner.acceptUntil.UnixNano())
	m.Drain(16)
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("on-time buffered result was expired before dispatch")
	}
}

type blockingSixelRuntimeJob struct {
	started chan<- struct{}
	release <-chan struct{}
	closes  atomic.Int32
}

func (j *blockingSixelRuntimeJob) Run(ctx context.Context) *sixel.DecodeResult {
	j.started <- struct{}{}
	select {
	case <-j.release:
	case <-ctx.Done():
	}
	return &sixel.DecodeResult{Failure: sixel.FailureCancelled}
}

func (j *blockingSixelRuntimeJob) Close() { j.closes.Add(1) }

func TestSixelExpiryDoesNotFinishSchedulerBeforeWorkerCleanup(t *testing.T) {
	limits := termimage.DefaultLimits()
	var nanos atomic.Int64
	m, session, _ := newSixelRuntimeMux(t, false, true, &limits, func() time.Time { return time.Unix(0, nanos.Load()) })
	p, _ := m.sessions.lookup(1)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	owner := sixelDecodeOwner{paneID: p.id, pane: p, token: 1, acceptUntil: time.Unix(0, int64(termimage.HardAcceptanceDeadline))}
	job := &blockingSixelRuntimeJob{started: started, release: release}
	if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: owner, job: job}); err != nil {
		t.Fatal(err)
	}
	m.sixelPending[owner.token] = owner
	awaitSchedulerSignals(t, started, 1)
	nanos.Store(owner.acceptUntil.UnixNano())
	m.expireImages(time.Unix(0, nanos.Load()))
	if len(m.sixelPending) != 0 {
		t.Fatal("expired owner remained pending")
	}
	duplicate := &sixelSchedulerTestJob{}
	if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: p.id}, job: duplicate}); !errors.Is(err, errKittyDecodePaneActive) || duplicate.closes.Load() != 1 {
		t.Fatalf("duplicate err=%v closes=%d", err, duplicate.closes.Load())
	}
	close(release)
	completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
	if events := m.applyImageCompletion(completion); len(events) != 0 {
		t.Fatalf("expired completion events=%#v", events)
	}
	accepted := &sixelSchedulerTestJob{}
	if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: p.id}, job: accepted}); err != nil {
		t.Fatalf("pane key not released after completion cleanup: %v", err)
	}
	completion = awaitImageSchedulerCompletion(t, m.imageScheduler)
	completion.Close()
	m.imageScheduler.finish(completion.Key)
	if len(session.written()) != 0 {
		t.Fatalf("expiry emitted reply %q", session.written())
	}
}

func TestSixelCompletionRejectsForgedIDsSpanAndDimensions(t *testing.T) {
	for _, name := range []string{"image id", "placement id", "span", "dimensions"} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := defaultSixelRuntimeMux(t)
			p, _ := m.sessions.lookup(1)
			m.advancePane(p, []byte(sixelRuntimeFrame))
			completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
			typed, ok := decodeSixelCompletion(completion)
			if !ok || typed.Result == nil || typed.Result.Candidate == nil {
				t.Fatal("missing Sixel completion")
			}
			owner := typed.Owner
			switch name {
			case "image id":
				owner.image = 1
			case "placement id":
				owner.placement = 1
			case "span":
				typed.Result.Span.Cols++
			case "dimensions":
				typed.Result.Candidate.Close()
				candidate, err := p.imageStore.NewDecodedCandidate(owner.image, 1, owner.raster.Height)
				if err != nil {
					t.Fatal(err)
				}
				if err = candidate.SealWrites(); err != nil {
					t.Fatal(err)
				}
				typed.Result.Candidate = candidate
			}
			m.sixelPending[owner.token] = owner
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

func TestSixelAtomicCommitRollbackAndEphemeralLifecycle(t *testing.T) {
	t.Run("placement reservation rollback", func(t *testing.T) {
		limits := termimage.DefaultLimits()
		limits.Placements = 1
		m, session, wakes := newSixelRuntimeMux(t, false, true, &limits, nil)
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
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
		if events := drainSixelCompletion(t, m, wakes); len(events) != 0 {
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
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
		drainSixelCompletion(t, m, wakes)
		if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
			t.Fatal("ephemeral resource missing before retirement")
		}
		m.advancePane(p, []byte("X"))
		if _, ok := p.imageStore.ResourceRef(owner.image); ok || p.imageStore.Usage() != (termimage.Usage{}) || len(p.terminal.ImageProjection(0, p.terminal.Rows()).Placements) != 0 {
			t.Fatalf("ephemeral resource survived final placement usage=%#v", p.imageStore.Usage())
		}
	})
}

func TestSixelRuntimeRestorePublishesFreshAdapterAndStore(t *testing.T) {
	limits := termimage.DefaultLimits()
	factory := &restoreTestFactory{}
	wakes := make(chan struct{}, 64)
	m := New(factory, Options{IngressCapacity: 64, ImageLimits: &limits, SixelEnabled: true, Wake: func() {
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
		if pane.imageStore == nil || pane.sixelAdapter == nil {
			t.Fatalf("restored pane %d missing Sixel state", pane.id)
		}
	}
	if _, err = m.CommitRestore(candidate); err != nil {
		t.Fatal(err)
	}
	pane := candidate.panes[0]
	m.advancePane(pane, []byte(sixelRuntimeFrame))
	owner := sixelOwnerForTest(t, m)
	drainSixelCompletion(t, m, wakes)
	if _, ok := pane.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("restored pane did not publish Sixel resource")
	}
}

func TestSixelRuntimeShutdownClearsPendingOwnerGraphs(t *testing.T) {
	limits := termimage.DefaultLimits()
	for _, test := range []struct {
		name         string
		kitty, sixel bool
		frame        string
	}{
		{name: "sixel", sixel: true, frame: sixelRuntimeFrame},
		{name: "kitty", kitty: true, frame: "\x1b_Ga=T,i=1,p=1,c=1,r=1,s=1,v=1;AQIDBA==\x1b\\"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _, wakes := newSixelRuntimeMux(t, test.kitty, test.sixel, &limits, nil)
			pane, _ := m.sessions.lookup(1)
			m.advancePane(pane, []byte(test.frame))
			awaitSchedulerSignals(t, wakes, 1)
			if len(m.sixelPending)+len(m.kittyPending) != 1 {
				t.Fatalf("pending sixel=%d kitty=%d want total=1", len(m.sixelPending), len(m.kittyPending))
			}
			if err := m.Shutdown(); err != nil {
				t.Fatal(err)
			}
			if len(m.sixelPending) != 0 || len(m.kittyPending) != 0 {
				t.Fatalf("pending survived shutdown sixel=%d kitty=%d", len(m.sixelPending), len(m.kittyPending))
			}
			if usage := m.imageBudget.Usage(); usage != (termimage.Usage{}) {
				t.Fatalf("shutdown usage=%#v", usage)
			}
		})
	}
}
