package mux

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"runtime"
	"testing"

	"cervterm/internal/pty"
)

type sessionIngressFactoryFunc func(uint16, uint16, pty.Options) (pty.Session, error)

func (f sessionIngressFactoryFunc) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	return f(rows, cols, options)
}

type sessionIngressWriteFailSession struct {
	*fakeSession
	err error
}

func (s *sessionIngressWriteFailSession) Write([]byte) (int, error) { return 0, s.err }

func sessionIngressKinds(events []Event) []EventKind {
	kinds := make([]EventKind, len(events))
	for i := range events {
		kinds[i] = events[i].Kind
	}
	return kinds
}

func TestMuxSessionIngressDataReplyAndDetachedPublicOutput(t *testing.T) {
	m, session, _ := newTestMux(t)
	pane := lookupPaneForTest(t, m.sessions, 1)
	data := []byte("A\x1b[6n")
	wantData := append([]byte(nil), data...)
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: data}

	events := m.Drain(1)
	if got, want := sessionIngressKinds(events), []EventKind{PaneOutput, PaneDirty}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event order=%v want=%v events=%#v", got, want, events)
	}
	if got := session.written(); !bytes.Equal(got, []byte("\x1b[1;2R")) {
		t.Fatalf("parser reply=%q", got)
	}
	if events[0].BytesRead != len(data) || !bytes.Equal(events[0].Data, wantData) {
		t.Fatalf("output=%#v want bytes=%d data=%q", events[0], len(data), wantData)
	}
	data[0] = 'Z'
	if !bytes.Equal(events[0].Data, wantData) {
		t.Fatalf("PaneOutput aliases ingress data: got=%q want=%q", events[0].Data, wantData)
	}
	events[0].Data[0] = 'Y'
	view, ok := m.PaneView(pane.id)
	if !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'A' {
		t.Fatalf("PaneOutput aliases captured terminal state: %#v", view)
	}
}

func TestMuxSessionIngressParserReplyFailurePrecedesOutputAndDirty(t *testing.T) {
	writeErr := errors.New("reply write failed")
	session := &sessionIngressWriteFailSession{fakeSession: newFakeSession(), err: writeErr}
	m := New(sessionIngressFactoryFunc(func(uint16, uint16, pty.Options) (pty.Session, error) {
		return session, nil
	}), Options{IngressCapacity: 1})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	pane := lookupPaneForTest(t, m.sessions, 1)
	data := []byte("\x1b[5n")
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: data}

	events := m.Drain(1)
	wantKinds := []EventKind{PaneWriteFailed, PaneOutput, PaneDirty}
	if got := sessionIngressKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event order=%v want=%v events=%#v", got, wantKinds, events)
	}
	if !errors.Is(events[0].Err, writeErr) || events[1].BytesRead != len(data) || !bytes.Equal(events[1].Data, data) {
		t.Fatalf("reply/output events=%#v", events)
	}
}

func TestMuxSessionIngressMetadataOrdering(t *testing.T) {
	m, _, _ := newTestMux(t)
	pane := lookupPaneForTest(t, m.sessions, 1)
	data := []byte("\x1b]2;mux-title\x07\x1b]7;file:///srv/project\x07\x07")
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: data}

	events := m.Drain(1)
	wantKinds := []EventKind{PaneOutput, PaneDirty, PaneTitleChanged, PaneCWDChanged, PaneBell}
	if got := sessionIngressKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event order=%v want=%v events=%#v", got, wantKinds, events)
	}
	if events[0].BytesRead != len(data) || !bytes.Equal(events[0].Data, data) {
		t.Fatalf("output=%#v", events[0])
	}
	if events[2].Text != "mux-title" || events[3].Text != "/srv/project" {
		t.Fatalf("metadata events=%#v", events)
	}
}

func TestMuxSessionIngressDataPrecedesExitAndNormalizesEOF(t *testing.T) {
	nonEOF := errors.New("read failed")
	for _, test := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "EOF", err: io.EOF},
		{name: "non-EOF", err: nonEOF, wantErr: nonEOF},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _, _ := newTestMux(t)
			pane := lookupPaneForTest(t, m.sessions, 1)
			m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: []byte("X")}
			m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, err: test.err}

			events := m.Drain(2)
			wantKinds := []EventKind{PaneOutput, PaneDirty, PaneExited, TabRevisionChanged}
			if got := sessionIngressKinds(events); !reflect.DeepEqual(got, wantKinds) {
				t.Fatalf("event order=%v want=%v events=%#v", got, wantKinds, events)
			}
			if !bytes.Equal(events[0].Data, []byte("X")) || events[0].BytesRead != 1 {
				t.Fatalf("data event=%#v", events[0])
			}
			if !errors.Is(events[2].Err, test.wantErr) || (test.wantErr == nil && events[2].Err != nil) {
				t.Fatalf("exit error=%v want=%v", events[2].Err, test.wantErr)
			}
			view, ok := m.PaneView(pane.id)
			if !ok || view.State != PaneStateExited || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'X' {
				t.Fatalf("exited view=%#v ok=%t", view, ok)
			}
		})
	}
}

func TestMuxSessionIngressRejectedRecordConsumesDrainLimit(t *testing.T) {
	for _, test := range []struct {
		name         string
		rejected     func(*pane) ingressRecord
		beforeSecond func(*pane)
	}{
		{
			name: "stale",
			rejected: func(p *pane) ingressRecord {
				return ingressRecord{pane: 999, owner: p, data: []byte("stale")}
			},
		},
		{
			name: "replaced owner",
			rejected: func(p *pane) ingressRecord {
				return ingressRecord{pane: p.id, owner: &pane{id: p.id}, data: []byte("replaced")}
			},
		},
		{
			name: "closed owner",
			rejected: func(p *pane) ingressRecord {
				p.state = PaneStateClosed
				return ingressRecord{pane: p.id, owner: p, data: []byte("closed")}
			},
			beforeSecond: func(p *pane) { p.state = PaneStateRunning },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _, _ := newTestMux(t)
			pane := lookupPaneForTest(t, m.sessions, 1)
			m.sessions.incoming <- test.rejected(pane)
			m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: []byte("V")}

			if events := m.Drain(1); len(events) != 0 {
				t.Fatalf("rejected record emitted events=%#v", events)
			}
			if got := len(m.sessions.incoming); got != 1 {
				t.Fatalf("queued records after limited drain=%d want=1", got)
			}
			if test.beforeSecond != nil {
				test.beforeSecond(pane)
			}
			events := m.Drain(1)
			if got, want := sessionIngressKinds(events), []EventKind{PaneOutput, PaneDirty}; !reflect.DeepEqual(got, want) {
				t.Fatalf("second drain order=%v want=%v events=%#v", got, want, events)
			}
			if !bytes.Equal(events[0].Data, []byte("V")) {
				t.Fatalf("second drain output=%q", events[0].Data)
			}
		})
	}
}

func TestMuxSessionIngressPreservesCallbackEnqueue(t *testing.T) {
	factory := &fakeFactory{}
	var m *Mux
	var pane *pane
	var clipboard string
	m = New(factory, Options{
		IngressCapacity: 8,
		SetClipboard: func(id PaneID, text string) {
			clipboard = text
			m.sessions.incoming <- ingressRecord{pane: id, owner: pane, data: []byte("B")}
		},
	})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	pane = lookupPaneForTest(t, m.sessions, 1)
	first := []byte("A\x1b]52;c;Qg==\x07")
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: first}

	events := m.Drain(2)
	wantKinds := []EventKind{PaneOutput, PaneDirty, PaneOutput, PaneDirty}
	if got := sessionIngressKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event order=%v want=%v events=%#v", got, wantKinds, events)
	}
	if clipboard != "B" || !bytes.Equal(events[0].Data, first) || !bytes.Equal(events[2].Data, []byte("B")) {
		t.Fatalf("clipboard=%q events=%#v", clipboard, events)
	}
	view, ok := m.PaneView(pane.id)
	if !ok || len(view.Snapshot.Cells) < 2 || view.Snapshot.Cells[0].Rune != 'A' || view.Snapshot.Cells[1].Rune != 'B' {
		t.Fatalf("callback ingress view=%#v", view)
	}
}

func TestMuxSessionIngressCallbackArrivalIsRevalidatedAndRejectedWhenStale(t *testing.T) {
	factory := &fakeFactory{}
	var m *Mux
	var pane *pane
	var callbackCalls int
	m = New(factory, Options{
		IngressCapacity: 8,
		SetClipboard: func(id PaneID, _ string) {
			callbackCalls++
			m.sessions.incoming <- ingressRecord{pane: id, owner: pane, data: []byte("B")}
			pane.state = PaneStateClosing
		},
	})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	pane = lookupPaneForTest(t, m.sessions, 1)
	defer func() { pane.state = PaneStateRunning }()
	first := []byte("A\x1b]52;c;Qg==\x07")
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: first}

	events := m.Drain(2)
	wantKinds := []EventKind{PaneOutput, PaneDirty}
	if got := sessionIngressKinds(events); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event order=%v want=%v events=%#v", got, wantKinds, events)
	}
	if callbackCalls != 1 || len(m.sessions.incoming) != 0 {
		t.Fatalf("callback calls=%d queued records=%d want 1/0", callbackCalls, len(m.sessions.incoming))
	}
	if !bytes.Equal(events[0].Data, first) {
		t.Fatalf("first output=%q want=%q", events[0].Data, first)
	}
	view, ok := m.PaneView(pane.id)
	if !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'A' {
		t.Fatalf("callback stale rejection view=%#v ok=%t", view, ok)
	}
	for _, cell := range view.Snapshot.Cells {
		if cell.Rune == 'B' {
			t.Fatalf("stale callback ingress reached terminal: %#v", view.Snapshot.Cells)
		}
	}
}

func drainMuxSessionIngressInlineForAllocationParity(m *Mux, record ingressRecord) []Event {
	var events []Event
	accepted := m.sessions.adaptSessionIngressRecord(record)
	if !accepted.acceptSessionIngress() {
		return m.ResolveEventAddresses(events)
	}
	operation := muxSessionIngressOperationAdapter{mux: m, pane: accepted.registered}
	if len(record.data) > 0 {
		events = operation.applySessionIngressData(events, record.data)
	}
	if record.err != nil {
		events = operation.applySessionIngressEnd(events, record.err)
	}
	return m.ResolveEventAddresses(events)
}

func TestMuxSessionIngressControllerWiringPreservesAllocationParity(t *testing.T) {
	active, _, _ := newTestMux(t)
	baseline, _, _ := newTestMux(t)
	activePane := lookupPaneForTest(t, active.sessions, 1)
	baselinePane := lookupPaneForTest(t, baseline.sessions, 1)
	data := []byte("x")
	activeRecord := ingressRecord{pane: activePane.id, owner: activePane, data: data}
	baselineRecord := ingressRecord{pane: baselinePane.id, owner: baselinePane, data: data}

	active.sessions.incoming <- activeRecord
	benchmarkMuxSessionIngressEvents = active.Drain(1)
	benchmarkMuxSessionIngressEvents = drainMuxSessionIngressInlineForAllocationParity(baseline, baselineRecord)
	baselineAllocs := testing.AllocsPerRun(1000, func() {
		benchmarkMuxSessionIngressEvents = drainMuxSessionIngressInlineForAllocationParity(baseline, baselineRecord)
	})
	activeAllocs := testing.AllocsPerRun(1000, func() {
		active.sessions.incoming <- activeRecord
		benchmarkMuxSessionIngressEvents = active.Drain(1)
	})
	if activeAllocs != baselineAllocs {
		t.Fatalf("Drain controller allocations=%v inline baseline=%v delta=%v", activeAllocs, baselineAllocs, activeAllocs-baselineAllocs)
	}
}

// TestKnownDefect_L3_02_SessionIngressAcceptsDifferentOwnerThread expires Slice 3.1.
func TestKnownDefect_L3_02_SessionIngressAcceptsDifferentOwnerThread(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	m, _, _ := newTestMux(t)
	pane := lookupPaneForTest(t, m.sessions, 1)
	m.sessions.incoming <- ingressRecord{pane: pane.id, owner: pane, data: []byte("X")}
	result := make(chan []Event, 1)
	go func() { result <- m.Drain(1) }()

	events := <-result
	if got, want := sessionIngressKinds(events), []EventKind{PaneOutput, PaneDirty}; !reflect.DeepEqual(got, want) {
		t.Fatalf("different-thread drain order=%v want=%v events=%#v", got, want, events)
	}
}

// TestKnownDefect_L3_04_FailedReaderStartLeavesBootstrapPublished expires Slice 3.2.
func TestKnownDefect_L3_04_FailedReaderStartLeavesBootstrapPublished(t *testing.T) {
	var m *Mux
	factory := sessionIngressFactoryFunc(func(uint16, uint16, pty.Options) (pty.Session, error) {
		session := newFakeSession()
		m.sessions.mu.Lock()
		m.sessions.started[1] = struct{}{}
		m.sessions.mu.Unlock()
		return session, nil
	})
	m = New(factory, Options{})
	t.Cleanup(func() { _ = m.Shutdown() })

	tab, pane, events, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err == nil || tab != 0 || pane != 0 || len(events) != 0 {
		t.Fatalf("failed reader start tab=%d pane=%d events=%#v err=%v", tab, pane, events, err)
	}
	if !m.bootstrapped {
		t.Fatal("known defect changed: failed reader start no longer leaves bootstrap published")
	}
	if _, ok := m.PaneView(1); ok {
		t.Fatal("failed reader start retained registry ownership")
	}
	if _, _, _, retryErr := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); !errors.Is(retryErr, ErrAlreadyBootstrapped) {
		t.Fatalf("retry error=%v want=%v", retryErr, ErrAlreadyBootstrapped)
	}
}

// TestKnownDefect_L3_08_GenericEventEnvelopeAcceptsInvalidPayloadCombination expires Slice 5.1.
func TestKnownDefect_L3_08_GenericEventEnvelopeAcceptsInvalidPayloadCombination(t *testing.T) {
	m, _, _ := newTestMux(t)
	marker := errors.New("invalid dirty error")
	events := m.ResolveEventAddresses([]Event{{
		Kind: PaneDirty, Pane: 1, Data: []byte("output"), BytesRead: 6,
		Text: "title", Geometry: PaneGeometry{Pane: 1, Cols: 80, Rows: 24}, Err: marker,
	}})
	if len(events) != 1 || events[0].Window == 0 || events[0].Workspace == 0 ||
		!bytes.Equal(events[0].Data, []byte("output")) || events[0].BytesRead != 6 ||
		events[0].Text != "title" || events[0].Geometry.Pane != 1 || !errors.Is(events[0].Err, marker) {
		t.Fatalf("generic invalid event=%#v", events)
	}
}

var benchmarkMuxSessionIngressEvents []Event

func BenchmarkMuxSessionIngressRejectedStale(b *testing.B) {
	m := New(&fakeFactory{}, Options{IngressCapacity: 1})
	b.Cleanup(func() { _ = m.Shutdown() })
	record := ingressRecord{pane: 999, data: []byte("stale")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.sessions.incoming <- record
		benchmarkMuxSessionIngressEvents = m.Drain(1)
	}
	b.StopTimer()
	if len(benchmarkMuxSessionIngressEvents) != 0 {
		b.Fatalf("stale ingress events=%#v", benchmarkMuxSessionIngressEvents)
	}
}

func BenchmarkMuxSessionIngressASCII(b *testing.B) {
	factory := &fakeFactory{}
	m := New(factory, Options{IngressCapacity: 1})
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = m.Shutdown() })
	pane, ok := m.sessions.lookup(1)
	if !ok {
		b.Fatal("bootstrap pane is not registry-owned")
	}
	record := ingressRecord{pane: pane.id, owner: pane, data: []byte("cervterm ingress\r\n")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.sessions.incoming <- record
		benchmarkMuxSessionIngressEvents = m.Drain(1)
	}
	b.StopTimer()
	if got := sessionIngressKinds(benchmarkMuxSessionIngressEvents); !reflect.DeepEqual(got, []EventKind{PaneOutput, PaneDirty}) {
		b.Fatalf("ASCII ingress events=%#v", benchmarkMuxSessionIngressEvents)
	}
}
