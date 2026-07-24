package mux

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"cervterm/internal/itermimage"
	"cervterm/internal/kitty"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

func newMuxProtocolSchedulingTestMux(tb testing.TB, options Options) (*Mux, *fakeSession, *pane) {
	tb.Helper()
	limits := termimage.DefaultLimits()
	options.IngressCapacity = 8
	options.ImageLimits = &limits
	options.KittyEnabled = true
	options.SixelEnabled = true
	options.ITermEnabled = true
	factory := &fakeFactory{}
	m := New(factory, options)
	_, paneID, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = m.Shutdown() })
	return m, factory.sessions[0], lookupPaneForTestTB(tb, m.sessions, paneID)
}

func lookupPaneForTestTB(tb testing.TB, registry *localSessionRegistry, id PaneID) *pane {
	tb.Helper()
	p, ok := registry.lookup(id)
	if !ok {
		tb.Fatalf("pane %d is not registry-owned", id)
	}
	return p
}

func TestMuxProtocolSchedulingDispatchOrderAroundReplyAndEvents(t *testing.T) {
	var session *fakeSession
	var trace []string
	m, createdSession, p := newMuxProtocolSchedulingTestMux(t, Options{ImageDiagnostic: func(diagnostic ImageDiagnostic) {
		entry := string(diagnostic.Protocol)
		if session != nil && len(session.written()) != 0 {
			entry += "-after-kitty"
		}
		trace = append(trace, entry)
	}})
	session = createdSession

	p.kittyEvents = append(p.kittyEvents, Event{Kind: PaneDirty, Pane: p.id})
	p.kittyOutcomes = append(p.kittyOutcomes, kitty.Outcome{Failure: kitty.ReplyFailed})
	p.sixelOutcomes = append(p.sixelOutcomes, sixel.Outcome{Failure: sixel.FailureFailed})
	p.itermOutcomes = append(p.itermOutcomes, itermimage.Outcome{Failure: itermimage.FailureFailed})

	events := m.advancePane(p, nil)
	if got, want := session.written(), (kitty.ReplyPlan{}).Encode(kitty.ReplyFailed); !bytes.Equal(got, want) {
		t.Fatalf("Kitty reply=%q want=%q", got, want)
	}
	if want := []string{"sixel-after-kitty", "iterm-after-kitty"}; !reflect.DeepEqual(trace, want) {
		t.Fatalf("protocol trace=%v want=%v", trace, want)
	}
	if got, want := sessionIngressKinds(events), []EventKind{PaneDirty, PaneOutput, PaneDirty}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event order=%v want=%v events=%#v", got, want, events)
	}
}

func TestMuxProtocolSchedulingUnknownCompletionClosesWithoutEvents(t *testing.T) {
	m, _, _ := newMuxProtocolSchedulingTestMux(t, Options{})
	result := &mismatchedImageResult{}
	completion := imageDecodeCompletion{Key: 1, Owner: imageDecodeOwner{protocol: imageDecodeProtocol(255)}, Result: result}
	if events := m.applyImageCompletion(completion); events != nil {
		t.Fatalf("unknown completion events=%#v", events)
	}
	if got := result.closes.Load(); got != 1 {
		t.Fatalf("unknown completion closes=%d want=1", got)
	}
}

func TestMuxProtocolSchedulingDeadlineMinimumAndInclusiveBoundary(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	var diagnostics []ImageDiagnostic
	m, _, p := newMuxProtocolSchedulingTestMux(t, Options{Now: func() time.Time { return base }, ImageDiagnostic: func(diagnostic ImageDiagnostic) {
		diagnostics = append(diagnostics, diagnostic)
	}})
	m.kittyPending[1] = kittyDecodeOwner{paneID: p.id, pane: p, token: 1, acceptUntil: base.Add(3 * time.Second)}
	m.sixelPending[1] = sixelDecodeOwner{paneID: p.id, pane: p, token: 1, startedAt: base, acceptUntil: base.Add(time.Second)}
	m.itermPending[1] = itermDecodeOwner{paneID: p.id, pane: p, token: 1, startedAt: base, acceptUntil: base.Add(2 * time.Second)}

	deadline, ok := m.NextImageDeadline()
	if want := base.Add(time.Second); !ok || deadline != want {
		t.Fatalf("minimum deadline=%v ok=%t want=%v", deadline, ok, want)
	}
	if events := m.expireImages(deadline.Add(-time.Nanosecond)); events != nil {
		t.Fatalf("pre-boundary events=%#v", events)
	}
	if len(m.sixelPending) != 1 || len(diagnostics) != 0 {
		t.Fatalf("pre-boundary sixel pending=%d diagnostics=%#v", len(m.sixelPending), diagnostics)
	}
	if events := m.expireImages(deadline); events != nil {
		t.Fatalf("boundary events=%#v", events)
	}
	if len(m.sixelPending) != 0 || len(m.kittyPending) != 1 || len(m.itermPending) != 1 {
		t.Fatalf("boundary pending kitty=%d sixel=%d iterm=%d", len(m.kittyPending), len(m.sixelPending), len(m.itermPending))
	}
	if len(diagnostics) != 1 || diagnostics[0].Protocol != ImageDiagnosticProtocolSixel || diagnostics[0].Reason != ImageDiagnosticReasonTimeout {
		t.Fatalf("boundary diagnostics=%#v", diagnostics)
	}
	if next, found := m.NextImageDeadline(); !found || next != base.Add(2*time.Second) {
		t.Fatalf("next deadline=%v found=%t", next, found)
	}
}

func TestMuxProtocolSchedulingAllDisabledIdle(t *testing.T) {
	m, _, _ := newTestMux(t)
	if m.imageScheduler != nil {
		t.Fatal("all-disabled mux created protocol scheduler")
	}
	if events := m.Drain(1); events != nil {
		t.Fatalf("all-disabled idle events=%#v", events)
	}
	if deadline, ok := m.NextImageDeadline(); ok || !deadline.IsZero() {
		t.Fatalf("all-disabled idle deadline=%v ok=%t", deadline, ok)
	}
}

// TestKnownDefect_L3_09_ErasedSchedulerResultRequiresRuntimeRouting expires Slice 4.8.
func TestKnownDefect_L3_09_ErasedSchedulerResultRequiresRuntimeRouting(t *testing.T) {
	m, _, p := newMuxProtocolSchedulingTestMux(t, Options{})
	result := &mismatchedImageResult{}
	completion := imageDecodeCompletion{
		Key: p.id,
		Owner: imageDecodeOwner{
			protocol: imageDecodeKitty,
			value:    kittyDecodeOwner{paneID: p.id, pane: p},
		},
		Result: result,
	}
	if events := m.applyImageCompletion(completion); events != nil {
		t.Fatalf("mismatched erased result events=%#v", events)
	}
	if got := result.closes.Load(); got != 1 {
		t.Fatalf("mismatched erased result closes=%d want=1", got)
	}
}

// TestKnownDefect_L3_09_MuxClockDoesNotReachTermimageStore expires Slice 4.8.
func TestKnownDefect_L3_09_MuxClockDoesNotReachTermimageStore(t *testing.T) {
	fakeNow := time.Unix(1, 0)
	_, _, p := newMuxProtocolSchedulingTestMux(t, Options{Now: func() time.Time { return fakeNow }})
	outcome := p.sixelAdapter.Advance(fakeNow, sixel.DCSEvent{Data: []byte("\"1;1;2;6#1~?")})
	if outcome != (sixel.Outcome{}) {
		t.Fatalf("partial Sixel outcome=%#v", outcome)
	}
	deadline, ok := p.sixelAdapter.NextExpiry()
	if !ok {
		t.Fatal("partial Sixel transfer has no deadline")
	}
	if injectedDeadline := fakeNow.Add(termimage.HardTransferLifetime); !deadline.After(injectedDeadline) {
		t.Fatalf("termimage deadline=%v want wall-clock value after injected deadline %v", deadline, injectedDeadline)
	}
}

var benchmarkMuxProtocolEvents []Event

type muxProtocolBenchmarkResult struct{ closes uint64 }

func (r *muxProtocolBenchmarkResult) Close() { r.closes++ }

func BenchmarkMuxProtocolDispatchIdle(b *testing.B) {
	m, _, _ := newMuxProtocolSchedulingTestMux(b, Options{})
	allocs := testing.AllocsPerRun(1000, func() {
		benchmarkMuxProtocolEvents = m.Drain(1)
	})
	if allocs != 0 {
		b.Fatalf("idle protocol dispatch allocations=%v want=0", allocs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMuxProtocolEvents = m.Drain(1)
	}
	b.StopTimer()
	if benchmarkMuxProtocolEvents != nil {
		b.Fatalf("idle protocol dispatch events=%#v", benchmarkMuxProtocolEvents)
	}
}

func BenchmarkMuxProtocolCompletionDiscard(b *testing.B) {
	m, _, p := newMuxProtocolSchedulingTestMux(b, Options{})
	result := &muxProtocolBenchmarkResult{}
	completion := imageDecodeCompletion{Key: p.id, Owner: imageDecodeOwner{protocol: imageDecodeProtocol(255)}, Result: result}
	allocs := testing.AllocsPerRun(1000, func() {
		benchmarkMuxProtocolEvents = m.applyImageCompletion(completion)
	})
	if allocs != 0 {
		b.Fatalf("completion discard allocations=%v want=0", allocs)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMuxProtocolEvents = m.applyImageCompletion(completion)
	}
	b.StopTimer()
	if benchmarkMuxProtocolEvents != nil {
		b.Fatalf("completion discard events=%#v", benchmarkMuxProtocolEvents)
	}
	if result.closes == 0 {
		b.Fatal("completion discard did not close result")
	}
}
