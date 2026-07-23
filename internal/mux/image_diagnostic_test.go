package mux

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cervterm/internal/itermimage"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

func TestImageDiagnosticExactPrivacySurface(t *testing.T) {
	typeOf := reflect.TypeOf(ImageDiagnostic{})
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "Protocol", typeOf: reflect.TypeOf(ImageDiagnosticProtocol(""))},
		{name: "Reason", typeOf: reflect.TypeOf(ImageDiagnosticReason(""))},
		{name: "Count", typeOf: reflect.TypeOf(uint64(0))},
		{name: "Duration", typeOf: reflect.TypeOf(time.Duration(0))},
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("ImageDiagnostic fields=%d want=%d", typeOf.NumField(), len(want))
	}
	for index, expected := range want {
		field := typeOf.Field(index)
		if field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("field[%d]=%s %v want=%s %v", index, field.Name, field.Type, expected.name, expected.typeOf)
		}
		lower := strings.ToLower(field.Name)
		for _, forbidden := range []string{"id", "payload", "pixel", "name", "base64"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("diagnostic field %q exposes forbidden surface %q", field.Name, forbidden)
			}
		}
	}
	if ImageDiagnosticProtocolSixel != "sixel" || ImageDiagnosticProtocolITerm != "iterm" {
		t.Fatalf("protocols=(%q,%q)", ImageDiagnosticProtocolSixel, ImageDiagnosticProtocolITerm)
	}
	gotReasons := []ImageDiagnosticReason{
		ImageDiagnosticReasonInvalid,
		ImageDiagnosticReasonUnsupported,
		ImageDiagnosticReasonLimit,
		ImageDiagnosticReasonTimeout,
		ImageDiagnosticReasonCancelled,
		ImageDiagnosticReasonFailed,
		ImageDiagnosticReasonStale,
		ImageDiagnosticReasonBusy,
	}
	wantReasons := []ImageDiagnosticReason{"invalid", "unsupported", "limit", "timeout", "cancelled", "failed", "stale", "busy"}
	if !reflect.DeepEqual(gotReasons, wantReasons) {
		t.Fatalf("reasons=%q want=%q", gotReasons, wantReasons)
	}
}

func TestImageDiagnosticFailureMappings(t *testing.T) {
	want := []ImageDiagnosticReason{
		ImageDiagnosticReasonInvalid,
		ImageDiagnosticReasonUnsupported,
		ImageDiagnosticReasonLimit,
		ImageDiagnosticReasonTimeout,
		ImageDiagnosticReasonCancelled,
		ImageDiagnosticReasonFailed,
	}
	for index, failure := range []sixel.Failure{
		sixel.FailureInvalid,
		sixel.FailureUnsupported,
		sixel.FailureLimit,
		sixel.FailureTimeout,
		sixel.FailureCancelled,
		sixel.FailureFailed,
	} {
		if got := sixelDiagnosticReason(failure); got != want[index] {
			t.Fatalf("Sixel failure %d mapped to %q want=%q", failure, got, want[index])
		}
	}
	for index, failure := range []itermimage.Failure{
		itermimage.FailureInvalid,
		itermimage.FailureUnsupported,
		itermimage.FailureLimit,
		itermimage.FailureTimeout,
		itermimage.FailureCancelled,
		itermimage.FailureFailed,
	} {
		if got := itermDiagnosticReason(failure); got != want[index] {
			t.Fatalf("iTerm failure %d mapped to %q want=%q", failure, got, want[index])
		}
	}
}

func TestImageDiagnosticCallbackPanicCannotAlterSixelRuntime(t *testing.T) {
	m, _, wakes := defaultSixelRuntimeMux(t)
	p, _ := m.sessions.lookup(1)
	calls := 0
	m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) {
		calls++
		panic("injected diagnostic panic")
	}
	p.sixelOutcomes = append(p.sixelOutcomes, sixel.Outcome{Failure: sixel.FailureInvalid})
	m.processSixelOutcomes(p)
	if calls != 1 || len(p.sixelOutcomes) != 0 {
		t.Fatalf("calls=%d outcomes=%d", calls, len(p.sixelOutcomes))
	}

	m.advancePane(p, []byte(sixelRuntimeFrame))
	owner := sixelOwnerForTest(t, m)
	if events := drainSixelCompletion(t, m, wakes); len(events) != 1 || events[0].Kind != PaneDirty {
		t.Fatalf("success events=%#v", events)
	}
	if _, ok := p.imageStore.ResourceRef(owner.image); !ok {
		t.Fatal("diagnostic panic changed successful Sixel runtime")
	}
	if calls != 1 {
		t.Fatalf("successful runtime emitted diagnostic calls=%d", calls)
	}
}

func TestImageDiagnosticSuccessEmitsNothing(t *testing.T) {
	t.Run("sixel", func(t *testing.T) {
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		m.advancePane(p, []byte(sixelRuntimeFrame))
		drainSixelCompletion(t, m, wakes)
		if len(diagnostics) != 0 {
			t.Fatalf("success diagnostics=%#v", diagnostics)
		}
	})

	t.Run("iterm", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		raw, _ := itermRuntimePNG(t, 2, 2)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		drainITermCompletion(t, m, wakes)
		if len(diagnostics) != 0 {
			t.Fatalf("success diagnostics=%#v", diagnostics)
		}
	})
}

func TestImageDiagnosticStaleCompletion(t *testing.T) {
	t.Run("sixel", func(t *testing.T) {
		m, _, wakes := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
		awaitSchedulerSignals(t, wakes, 1)
		p.terminal.PutRune('x')
		if events := m.Drain(16); len(events) != 0 {
			t.Fatalf("stale events=%#v", events)
		}
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonStale)
		if _, ok := owner.store.ResourceRef(owner.image); ok {
			t.Fatal("stale Sixel completion published")
		}
	})

	t.Run("iterm", func(t *testing.T) {
		m, _, wakes := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		raw, _ := itermRuntimePNG(t, 2, 2)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		awaitSchedulerSignals(t, wakes, 1)
		p.terminal.PutRune('x')
		if events := m.Drain(16); len(events) != 0 {
			t.Fatalf("stale events=%#v", events)
		}
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolITerm, ImageDiagnosticReasonStale)
		if _, ok := owner.store.ResourceRef(owner.image); ok {
			t.Fatal("stale iTerm completion published")
		}
	})
}

func TestSuccessfulImageCandidateAtOrAfterDeadlineIsRejectedWithTimeoutDiagnostic(t *testing.T) {
	t.Run("sixel equal", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		m.advancePane(p, []byte(sixelRuntimeFrame))
		owner := sixelOwnerForTest(t, m)
		completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
		typed, ok := decodeSixelCompletion(completion)
		if !ok || typed.Result == nil || typed.Result.Failure != sixel.FailureNone || typed.Result.Candidate == nil {
			t.Fatal("test requires a successful Sixel candidate")
		}
		completion.FinishedAt = owner.acceptUntil
		if events := m.applyImageCompletion(completion); len(events) != 0 {
			t.Fatalf("deadline events=%#v", events)
		}
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonTimeout)
		if diagnostics[0].Duration != termimage.HardAcceptanceDeadline {
			t.Fatalf("duration=%v want=%v", diagnostics[0].Duration, termimage.HardAcceptanceDeadline)
		}
		if _, ok := owner.store.ResourceRef(owner.image); ok {
			t.Fatal("candidate completed at deadline was published")
		}
	})

	t.Run("iterm after", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		raw, _ := itermRuntimePNG(t, 2, 2)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		owner := itermOwnerForTest(t, m)
		completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
		typed, ok := decodeITermCompletion(completion)
		if !ok || typed.Result == nil || typed.Result.Failure != itermimage.FailureNone || typed.Result.Candidate == nil {
			t.Fatal("test requires a successful iTerm candidate")
		}
		completion.FinishedAt = owner.acceptUntil.Add(time.Nanosecond)
		if events := m.applyImageCompletion(completion); len(events) != 0 {
			t.Fatalf("deadline events=%#v", events)
		}
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolITerm, ImageDiagnosticReasonTimeout)
		if diagnostics[0].Duration != termimage.HardAcceptanceDeadline+time.Nanosecond {
			t.Fatalf("duration=%v want=%v", diagnostics[0].Duration, termimage.HardAcceptanceDeadline+time.Nanosecond)
		}
		if _, ok := owner.store.ResourceRef(owner.image); ok {
			t.Fatal("candidate completed after deadline was published")
		}
	})
}

func assertImageDiagnostic(t *testing.T, diagnostics []ImageDiagnostic, protocol ImageDiagnosticProtocol, reason ImageDiagnosticReason) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%#v want exactly one", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Protocol != protocol || diagnostic.Reason != reason || diagnostic.Count != 1 || diagnostic.Duration < 0 {
		t.Fatalf("diagnostic=%#v want protocol=%q reason=%q count=1 nonnegative duration", diagnostic, protocol, reason)
	}
}

func TestImageDiagnosticSubmissionFailureSites(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		outcome := diagnosticSixelOutcome(t, m, p)
		scheduler := m.imageScheduler
		m.imageScheduler = nil
		m.submitSixelDecode(p, outcome)
		m.imageScheduler = scheduler
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonFailed)
	})

	t.Run("bad metrics", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		m.paneMetrics[p.id] = CellMetrics{}
		m.submitSixelDecode(p, diagnosticSixelOutcome(t, m, p))
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonInvalid)
	})

	t.Run("job validation", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		outcome := diagnosticSixelOutcome(t, m, p)
		outcome.Command.Image = 1
		m.submitSixelDecode(p, outcome)
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonInvalid)
	})

	t.Run("scheduler busy", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		blocking := &blockingSixelRuntimeJob{started: started, release: release}
		if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: sixelDecodeOwner{paneID: p.id}, job: blocking}); err != nil {
			t.Fatal(err)
		}
		awaitSchedulerSignals(t, started, 1)
		m.submitSixelDecode(p, diagnosticSixelOutcome(t, m, p))
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonBusy)
		close(release)
		completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
		completion.Close()
		m.imageScheduler.finish(completion.Key)
	})
}

func TestImageDiagnosticAdapterFailureMappingsEmitOnce(t *testing.T) {
	t.Run("sixel", func(t *testing.T) {
		m, _, _ := defaultSixelRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		failures := []sixel.Failure{sixel.FailureInvalid, sixel.FailureUnsupported, sixel.FailureLimit, sixel.FailureTimeout, sixel.FailureCancelled, sixel.FailureFailed}
		for _, failure := range failures {
			var diagnostics []ImageDiagnostic
			m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
			p.sixelOutcomes = append(p.sixelOutcomes, sixel.Outcome{Failure: failure})
			m.processSixelOutcomes(p)
			assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, sixelDiagnosticReason(failure))
		}
	})

	t.Run("iterm", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		failures := []itermimage.Failure{itermimage.FailureInvalid, itermimage.FailureUnsupported, itermimage.FailureLimit, itermimage.FailureTimeout, itermimage.FailureCancelled, itermimage.FailureFailed}
		for _, failure := range failures {
			var diagnostics []ImageDiagnostic
			m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
			p.itermOutcomes = append(p.itermOutcomes, itermimage.Outcome{Failure: failure})
			m.processITermOutcomes(p)
			assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolITerm, itermDiagnosticReason(failure))
		}
	})
}

func TestImageDiagnosticResultDecodeFailureAndExpiryDoNotDuplicate(t *testing.T) {
	t.Run("result decode failure", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		raw, _ := itermRuntimePNG(t, 2, 2)
		m.advancePane(p, []byte(itermRuntimeFrame(raw, "", 0x07)))
		completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
		typed, ok := decodeITermCompletion(completion)
		if !ok || typed.Result == nil {
			t.Fatal("missing typed iTerm result")
		}
		typed.Result.Close()
		mismatched := &mismatchedImageResult{}
		completion.Result = mismatched
		if events := m.applyImageCompletion(completion); len(events) != 0 {
			t.Fatalf("decode failure events=%#v", events)
		}
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolITerm, ImageDiagnosticReasonFailed)
		if mismatched.closes.Load() != 1 {
			t.Fatalf("mismatched result closes=%d", mismatched.closes.Load())
		}
	})

	t.Run("pending expiry", func(t *testing.T) {
		limits := termimage.DefaultLimits()
		var now time.Time
		m, _, _ := newSixelRuntimeMux(t, false, true, &limits, func() time.Time { return now })
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		startedAt := time.Unix(1, 0)
		now = startedAt
		owner := sixelDecodeOwner{paneID: p.id, pane: p, token: 1, startedAt: startedAt, acceptUntil: startedAt.Add(termimage.HardAcceptanceDeadline)}
		if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: owner, job: &blockingSixelRuntimeJob{started: started, release: release}}); err != nil {
			t.Fatal(err)
		}
		m.sixelPending[owner.token] = owner
		awaitSchedulerSignals(t, started, 1)
		now = owner.acceptUntil
		m.expireImages(now)
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonTimeout)
		close(release)
		completion := awaitImageSchedulerCompletion(t, m.imageScheduler)
		m.applyImageCompletion(completion)
		if len(diagnostics) != 1 {
			t.Fatalf("late completion duplicated expiry diagnostic: %#v", diagnostics)
		}
	})

	t.Run("adapter expiry", func(t *testing.T) {
		m, _, _ := defaultITermRuntimeMux(t)
		p, _ := m.sessions.lookup(1)
		var diagnostics []ImageDiagnostic
		m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) { diagnostics = append(diagnostics, diagnostic) }
		if outcome := p.itermAdapter.Advance(m.options.Now(), itermimage.OSCEvent{Data: []byte("File=inline=1;size=3:AAAA")}); outcome != (itermimage.Outcome{}) {
			t.Fatalf("partial outcome=%#v", outcome)
		}
		deadline, ok := p.itermAdapter.NextExpiry()
		if !ok {
			t.Fatal("missing adapter expiry")
		}
		m.expireImages(deadline)
		assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolITerm, ImageDiagnosticReasonTimeout)
	})
}

func diagnosticSixelOutcome(t *testing.T, m *Mux, p *pane) sixel.Outcome {
	t.Helper()
	outcome := p.sixelAdapter.Advance(m.options.Now(), sixel.DCSEvent{Data: []byte("\"1;1;2;6#1~?"), Final: true})
	if outcome.Command == nil || outcome.Failure != sixel.FailureNone {
		t.Fatalf("Sixel outcome=%#v", outcome)
	}
	return outcome
}
