package mux

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/pty"
)

type fakeSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter

	mu         sync.Mutex
	writes     [][]byte
	resizes    []pty.Size
	resizeErr  error
	closeErr   error
	closeCount int
	onResize   func(pty.Size)
}

func newFakeSession() *fakeSession {
	r, w := io.Pipe()
	return &fakeSession{reader: r, writer: w}
}

func (s *fakeSession) Reader() io.Reader { return s.reader }
func (s *fakeSession) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, append([]byte(nil), data...))
	return len(data), nil
}
func (s *fakeSession) Resize(size pty.Size) error {
	s.mu.Lock()
	s.resizes = append(s.resizes, size)
	onResize, err := s.onResize, s.resizeErr
	s.mu.Unlock()
	if onResize != nil {
		onResize(size)
	}
	return err
}
func (s *fakeSession) Close() error {
	s.mu.Lock()
	s.closeCount++
	err := s.closeErr
	s.mu.Unlock()
	_ = s.writer.Close()
	_ = s.reader.Close()
	return err
}
func (s *fakeSession) feed(data []byte) error {
	_, err := s.writer.Write(data)
	return err
}
func (s *fakeSession) eof() error { return s.writer.Close() }
func (s *fakeSession) written() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Join(s.writes, nil)
}
func (s *fakeSession) closes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCount
}

type fakeFactory struct {
	mu             sync.Mutex
	sessions       []*fakeSession
	err            error
	sessionOnError *fakeSession
	calls          []pty.Size
}

func (f *fakeFactory) Spawn(rows, cols uint16, _ pty.Options) (pty.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pty.Size{Rows: rows, Cols: cols})
	if f.err != nil {
		if f.sessionOnError == nil {
			return nil, f.err
		}
		return f.sessionOnError, f.err
	}
	s := newFakeSession()
	f.sessions = append(f.sessions, s)
	return s, nil
}

func newTestMux(t *testing.T) (*Mux, *fakeSession, chan struct{}) {
	t.Helper()
	factory := &fakeFactory{}
	wakes := make(chan struct{}, 16)
	m := New(factory, Options{IngressCapacity: 8, Wake: func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	}})
	_, id, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil || id != 1 {
		t.Fatalf("bootstrap = pane %d, %v", id, err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, factory.sessions[0], wakes
}

func awaitWake(t *testing.T, wakes <-chan struct{}) {
	t.Helper()
	select {
	case <-wakes:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pane ingress")
	}
}

func TestMuxBootstrapDrainSnapshotAndParserReply(t *testing.T) {
	m, session, wakes := newTestMux(t)
	if err := session.feed([]byte("A\x1b[6n")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	events := m.Drain(16)
	if len(events) == 0 || events[0].Pane != 1 {
		t.Fatalf("events = %#v", events)
	}
	view, ok := m.PaneView(1)
	if !ok || len(view.Snapshot.Cells) == 0 || view.Snapshot.Cells[0].Rune != 'A' {
		t.Fatalf("snapshot did not capture pane output: %#v", view)
	}
	if got := string(session.written()); got != "\x1b[1;2R" {
		t.Fatalf("parser reply = %q, want cursor report", got)
	}
	view.Snapshot.Cells[0].Rune = 'Z'
	again, _ := m.PaneView(1)
	if again.Snapshot.Cells[0].Rune != 'A' {
		t.Fatal("PaneView exposed mutable snapshot storage")
	}
}

func TestMuxWriteResizeOrderingAndRetryState(t *testing.T) {
	m, session, _ := newTestMux(t)
	if _, err := m.Write(1, []byte("input")); err != nil {
		t.Fatal(err)
	}
	if got := string(session.written()); got != "input" {
		t.Fatalf("input write = %q", got)
	}

	session.resizeErr = errors.New("resize failed")
	session.onResize = func(size pty.Size) {
		pane := m.panes[1]
		if pane.terminal.Cols() != int(size.Cols) || pane.terminal.Rows() != int(size.Rows) {
			t.Errorf("PTY resize ran before terminal resize: terminal=%dx%d pty=%dx%d", pane.terminal.Cols(), pane.terminal.Rows(), size.Cols, size.Rows)
		}
	}
	events, err := m.Resize(PixelRect{Width: 640, Height: 320}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err == nil || len(events) == 0 {
		t.Fatalf("failed resize = events %#v, err %v", events, err)
	}
	view, _ := m.PaneView(1)
	if view.ResizeErr == nil || view.DesiredSize == view.AppliedSize {
		t.Fatalf("resize retry state = %#v", view)
	}

	session.resizeErr = nil
	if _, err := m.Resize(PixelRect{Width: 640, Height: 320}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	view, _ = m.PaneView(1)
	if view.ResizeErr != nil || view.DesiredSize != view.AppliedSize {
		t.Fatalf("successful retry state = %#v", view)
	}
}

func TestMuxEOFRetainsPaneAndCloseIsIdempotent(t *testing.T) {
	m, session, wakes := newTestMux(t)
	if err := session.eof(); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	events := m.Drain(8)
	if len(events) != 1 || events[0].Kind != PaneExited || events[0].Err != nil {
		t.Fatalf("EOF events = %#v", events)
	}
	view, ok := m.PaneView(1)
	if !ok || view.State != PaneStateExited || len(m.PaneIDs()) != 1 {
		t.Fatalf("exited pane was not retained: %#v panes=%v", view, m.PaneIDs())
	}
	if _, err := m.Write(1, []byte("x")); !errors.Is(err, ErrPaneNotRunning) {
		t.Fatalf("write to exited pane = %v", err)
	}
	if _, err := m.ClosePane(1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ClosePane(1); err != nil {
		t.Fatal(err)
	}
	if session.closes() != 1 {
		t.Fatalf("session close count = %d, want 1", session.closes())
	}
}

func TestMuxSpawnFailurePreservesDiagnosticLeaf(t *testing.T) {
	factory := &fakeFactory{err: errors.New("spawn unavailable")}
	m := New(factory, Options{})
	defer m.Shutdown()
	_, id, events, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err == nil || id != 1 || len(events) != 4 {
		t.Fatalf("bootstrap failure = pane %d events %#v err %v", id, events, err)
	}
	view, ok := m.PaneView(1)
	if !ok || view.State != PaneStateFailed || len(view.Snapshot.Cells) == 0 {
		t.Fatalf("failed pane = %#v", view)
	}
	if got := stringCells(view.Snapshot.Cells); !bytes.Contains([]byte(got), []byte("Local PTY unavailable")) {
		t.Fatalf("fallback banner missing from %q", got)
	}
	if _, err := m.FeedFallback(1, []byte("\x1b[6n")); err != nil {
		t.Fatalf("fallback parser reply handling: %v", err)
	}
}

func stringCells(cells []core.Cell) string {
	var b bytes.Buffer
	for _, cell := range cells {
		if cell.Rune != 0 {
			b.WriteRune(cell.Rune)
		}
	}
	return b.String()
}

func TestMuxSplitRoutesIndependentSessionsAndCollapses(t *testing.T) {
	factory := &fakeFactory{}
	wakes := make(chan struct{}, 16)
	m := New(factory, Options{Wake: func() { wakes <- struct{}{} }})
	defer m.Shutdown()
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	second, events, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil || second != 2 || len(events) == 0 {
		t.Fatalf("split = pane %d events=%#v err=%v", second, events, err)
	}
	if len(factory.sessions) != 2 {
		t.Fatalf("spawned sessions = %d, want 2", len(factory.sessions))
	}
	if err := factory.sessions[0].feed([]byte("A")); err != nil {
		t.Fatal(err)
	}
	if err := factory.sessions[1].feed([]byte("B")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	awaitWake(t, wakes)
	m.Drain(16)
	firstView, _ := m.PaneView(1)
	secondView, _ := m.PaneView(2)
	if firstView.Snapshot.Cells[0].Rune != 'A' || secondView.Snapshot.Cells[0].Rune != 'B' {
		t.Fatalf("pane output crossed: first=%q second=%q", firstView.Snapshot.Cells[0].Rune, secondView.Snapshot.Cells[0].Rune)
	}
	if _, err := m.Write(2, []byte("only-second")); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(factory.sessions[0].written(), []byte("only-second")) || !bytes.Contains(factory.sessions[1].written(), []byte("only-second")) {
		t.Fatal("pane-addressed input reached the wrong session")
	}
	if _, err := m.ClosePane(2); err != nil {
		t.Fatal(err)
	}
	if got := m.PaneIDs(); len(got) != 1 || got[0] != 1 || factory.sessions[0].closes() != 0 || factory.sessions[1].closes() != 1 {
		t.Fatalf("close/collapse panes=%v closes=(%d,%d)", got, factory.sessions[0].closes(), factory.sessions[1].closes())
	}
	firstView, ok := m.PaneView(1)
	if !ok || firstView.Geometry.Cols != 100 || firstView.Geometry.Rows != 30 || firstView.Snapshot.Cols != 100 || firstView.Snapshot.Rows != 30 {
		t.Fatalf("survivor was not resized after collapse: %#v", firstView)
	}
}

func TestMuxSplitSpawnFailureIsAtomic(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	factory.mu.Lock()
	factory.err = errors.New("second spawn failed")
	factory.mu.Unlock()
	before := m.PaneIDs()
	if _, _, err := m.Split(1, SplitColumns, SpawnSpec{}); err == nil {
		t.Fatal("split spawn failure returned nil")
	}
	after := m.PaneIDs()
	if !reflect.DeepEqual(before, after) || m.model.nextPaneID != 2 || m.model.FocusedPane() != 1 {
		t.Fatalf("failed split mutated model: before=%v after=%v next=%d focus=%d", before, after, m.model.nextPaneID, m.model.FocusedPane())
	}
}

func TestMuxBootstrapClosesPartialSessionOnSpawnError(t *testing.T) {
	partial := newFakeSession()
	factory := &fakeFactory{err: errors.New("spawn failed"), sessionOnError: partial}
	m := New(factory, Options{})
	defer m.Shutdown()
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err == nil {
		t.Fatal("bootstrap returned nil error")
	}
	if partial.closes() != 1 {
		t.Fatalf("partial session close count = %d, want 1", partial.closes())
	}
}

func TestMuxCompressedGeometryMatchesSnapshotMinimum(t *testing.T) {
	m, _, _ := newTestMux(t)
	if _, _, err := m.Split(1, SplitColumns, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResizeGrid(PixelRect{Width: 1, Height: 1}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	for _, id := range m.PaneIDs() {
		view, ok := m.PaneView(id)
		if !ok {
			t.Fatalf("missing pane %d", id)
		}
		if view.Geometry.Cols != view.Snapshot.Cols || view.Geometry.Rows != view.Snapshot.Rows || view.Geometry.Cols < 2 || view.Geometry.Rows < 1 {
			t.Fatalf("pane %d inconsistent compressed geometry: geometry=%dx%d snapshot=%dx%d", id, view.Geometry.Cols, view.Geometry.Rows, view.Snapshot.Cols, view.Snapshot.Rows)
		}
	}
}
