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
func (s *fakeSession) resizeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.resizes)
}

type fakeFactory struct {
	mu             sync.Mutex
	sessions       []*fakeSession
	err            error
	sessionOnError *fakeSession
	calls          []pty.Size
	options        []pty.Options
}

func (f *fakeFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pty.Size{Rows: rows, Cols: cols})
	options.ShellArgs = append([]string(nil), options.ShellArgs...)
	options.Env = cloneEnvironment(options.Env)
	f.options = append(f.options, options)
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

func cloneEnvironment(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	result := make(map[string]string, len(env))
	for key, value := range env {
		result[key] = value
	}
	return result
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

func TestMuxSetScrollbackCapacityAppliesToAllPanes(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Split(1, SplitColumns, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}

	m.SetScrollbackCapacity(7)
	if _, _, err := m.Split(1, SplitRows, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range m.PaneIDs() {
		if got := m.panes[id].terminal.ScrollbackCapacity(); got != 7 {
			t.Fatalf("pane %d scrollback capacity = %d, want 7", id, got)
		}
	}
}

func TestMuxSetHideCursorWhenScrolledAppliesToActiveAndFuturePanes(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}

	m.SetHideCursorWhenScrolled(false)
	if m.panes[1].captureOptions.HideCursorWhenScrolled {
		t.Fatal("active pane retained hidden-cursor policy")
	}
	newID, _, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if m.panes[newID].captureOptions.HideCursorWhenScrolled {
		t.Fatal("future pane did not inherit live cursor policy")
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

func TestMuxSetSplitRatioReflowsBeforeSettledPTYResize(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	bounds := PixelRect{Width: 801, Height: 480}
	metrics := CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, bounds, metrics); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Split(1, SplitColumns, SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	layout, err := m.Layout()
	if err != nil || len(layout.Dividers) != 1 {
		t.Fatalf("layout=%#v err=%v", layout, err)
	}
	beforeFirst, beforeSecond := factory.sessions[0].resizeCount(), factory.sessions[1].resizeCount()
	events, err := m.SetSplitRatio(layout.Dividers[0].Split, 6_000)
	if err != nil || len(events) == 0 {
		t.Fatalf("SetSplitRatio events=%#v err=%v", events, err)
	}
	first, _ := m.PaneView(1)
	second, _ := m.PaneView(2)
	if first.Geometry.Pixels.Width <= second.Geometry.Pixels.Width || first.Snapshot.Cols != first.Geometry.Cols {
		t.Fatalf("reflowed geometry first=%#v second=%#v", first.Geometry, second.Geometry)
	}
	if got := factory.sessions[0].resizeCount(); got != beforeFirst {
		t.Fatalf("first PTY resized during live ratio update: before=%d after=%d", beforeFirst, got)
	}
	if got := factory.sessions[1].resizeCount(); got != beforeSecond {
		t.Fatalf("second PTY resized during live ratio update: before=%d after=%d", beforeSecond, got)
	}
	for _, id := range m.PaneIDs() {
		if _, err := m.ApplyResize(id); err != nil {
			t.Fatal(err)
		}
	}
	if factory.sessions[0].resizeCount() != beforeFirst+1 || factory.sessions[1].resizeCount() != beforeSecond+1 {
		t.Fatalf("settled resize counts=(%d,%d), want (%d,%d)", factory.sessions[0].resizeCount(), factory.sessions[1].resizeCount(), beforeFirst+1, beforeSecond+1)
	}
}

func TestMuxSpawnSplitFailureIsAtomicAndSilent(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	_, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	beforeIDs := m.PaneIDs()
	beforeLayout, err := m.Layout()
	if err != nil {
		t.Fatal(err)
	}
	beforeFocus := m.model.FocusedPane()
	beforeSessions := len(factory.sessions)
	partial := newFakeSession()
	factory.mu.Lock()
	factory.err = errors.New("second spawn failed")
	factory.sessionOnError = partial
	factory.mu.Unlock()
	created, events, err := m.SpawnSplit(1, SplitColumns, SpawnSpec{Options: pty.Options{ShellProgram: "program", ShellArgs: []string{"space arg", `quote"arg`, "&|;$()"}}})
	if err == nil || created != 0 || len(events) != 0 {
		t.Fatalf("failed spawn = pane %d events=%#v err=%v", created, events, err)
	}
	afterLayout, layoutErr := m.Layout()
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	if !reflect.DeepEqual(beforeIDs, m.PaneIDs()) || !reflect.DeepEqual(beforeLayout, afterLayout) || m.model.nextPaneID != 2 || m.model.FocusedPane() != beforeFocus {
		t.Fatalf("failed spawn mutated model: panes=%v layout=%#v next=%d focus=%d", m.PaneIDs(), afterLayout, m.model.nextPaneID, m.model.FocusedPane())
	}
	if len(factory.sessions) != beforeSessions || partial.closes() != 1 {
		t.Fatalf("failed spawn leaked process: sessions=%d want=%d partial closes=%d", len(factory.sessions), beforeSessions, partial.closes())
	}
}

func TestMuxSpawnSplitTransportsExactOptionsWithoutShellWrapper(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	defer m.Shutdown()
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 800, Height: 480}, CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	}
	want := pty.Options{
		ShellProgram:     "literal program",
		ShellArgs:        []string{"space arg", `quote"arg`, `'single'`, "&|;$()<>*?"},
		WorkingDirectory: "directory with spaces",
		Env:              map[string]string{"VALUE": `spaces " quotes & metacharacters`},
	}
	created, events, err := m.SpawnSplit(1, SplitRows, SpawnSpec{Options: want})
	if err != nil || created != 2 || len(events) == 0 {
		t.Fatalf("SpawnSplit = pane %d events=%#v err=%v", created, events, err)
	}
	if got := factory.options[len(factory.options)-1]; !reflect.DeepEqual(got, want) {
		t.Fatalf("spawn options = %#v, want exact %#v", got, want)
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

func TestMuxResizePaneGridIsTargetedAndResizeBoundsPreservesMetrics(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	bounds := PixelRect{Width: 401, Height: 200}
	initial := CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, bounds, initial); err != nil {
		t.Fatal(err)
	}
	second, _, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	firstBefore, _ := m.PaneView(1)
	secondBefore, _ := m.PaneView(second)

	zoomed := CellMetrics{CellWidth: 10, CellHeight: 20}
	events, err := m.ResizePaneGrid(second, zoomed)
	if err != nil {
		t.Fatal(err)
	}
	firstAfter, _ := m.PaneView(1)
	secondAfter, _ := m.PaneView(second)
	if firstAfter.Geometry != firstBefore.Geometry || firstAfter.DesiredSize != firstBefore.DesiredSize {
		t.Fatalf("first pane changed during targeted resize: before=%#v/%#v after=%#v/%#v", firstBefore.Geometry, firstBefore.DesiredSize, firstAfter.Geometry, firstAfter.DesiredSize)
	}
	if secondAfter.Geometry.Pixels != secondBefore.Geometry.Pixels || secondAfter.Geometry.Cols != 20 || secondAfter.Geometry.Rows != 10 {
		t.Fatalf("second pane geometry = %#v, want unchanged pixels and 20x10 grid", secondAfter.Geometry)
	}
	if secondAfter.DesiredSize != (pty.Size{Rows: 10, Cols: 20}) || secondAfter.DesiredSize == secondBefore.DesiredSize {
		t.Fatalf("second desired size = %#v, before %#v", secondAfter.DesiredSize, secondBefore.DesiredSize)
	}
	for _, event := range events {
		if event.Pane != second {
			t.Fatalf("targeted resize emitted event for pane %d: %#v", event.Pane, events)
		}
	}
	if len(events) != 2 || events[0].Kind != PaneGeometryChanged || events[1].Kind != PaneDirty {
		t.Fatalf("targeted resize events = %#v", events)
	}
	if events, err := m.ResizePaneGrid(second, zoomed); err != nil || len(events) != 0 {
		t.Fatalf("unchanged pane metrics events=%#v err=%v", events, err)
	}

	resizedBounds := PixelRect{Width: 501, Height: 240}
	if _, err := m.ResizeBounds(resizedBounds); err != nil {
		t.Fatal(err)
	}
	firstAfter, _ = m.PaneView(1)
	secondAfter, _ = m.PaneView(second)
	if firstAfter.Geometry.Cols != 31 || firstAfter.Geometry.Rows != 15 {
		t.Fatalf("first grid after bounds resize = %dx%d, want 31x15", firstAfter.Geometry.Cols, firstAfter.Geometry.Rows)
	}
	if secondAfter.Geometry.Cols != 25 || secondAfter.Geometry.Rows != 12 {
		t.Fatalf("zoomed grid after bounds resize = %dx%d, want 25x12", secondAfter.Geometry.Cols, secondAfter.Geometry.Rows)
	}
	if got := m.paneMetrics[second]; got != zoomed {
		t.Fatalf("ResizeBounds reset pane metric to %#v, want %#v", got, zoomed)
	}

	uniform := CellMetrics{CellWidth: 5, CellHeight: 10}
	if _, err := m.ResizeGrid(resizedBounds, uniform); err != nil {
		t.Fatal(err)
	}
	firstAfter, _ = m.PaneView(1)
	secondAfter, _ = m.PaneView(second)
	if firstAfter.Geometry.Cols != secondAfter.Geometry.Cols || firstAfter.Geometry.Rows != secondAfter.Geometry.Rows {
		t.Fatalf("uniform ResizeGrid produced different grids: first=%#v second=%#v", firstAfter.Geometry, secondAfter.Geometry)
	}
	if m.paneMetrics[1] != uniform || m.paneMetrics[second] != uniform {
		t.Fatalf("uniform ResizeGrid metrics = %#v", m.paneMetrics)
	}
}

func TestMuxSplitInheritsAndCloseRemovesPaneMetrics(t *testing.T) {
	factory := &fakeFactory{}
	m := New(factory, Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	initial := CellMetrics{CellWidth: 9, CellHeight: 18, PaddingX: 1, PaddingY: 2}
	if _, _, _, err := m.Bootstrap(SpawnSpec{}, PixelRect{Width: 501, Height: 240}, initial); err != nil {
		t.Fatal(err)
	}
	second, _, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.paneMetrics[second]; got != initial {
		t.Fatalf("split metric = %#v, want inherited %#v", got, initial)
	}
	if _, err := m.ClosePane(second); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.paneMetrics[second]; ok {
		t.Fatalf("closed pane %d retained metrics", second)
	}
}

func TestMuxResizePaneGridValidationIsAtomic(t *testing.T) {
	m, _, _ := newTestMux(t)
	before, _ := m.PaneView(1)
	beforeMetrics := m.paneMetrics[1]

	if _, err := m.ResizePaneGrid(999, testMetrics); !errors.Is(err, ErrPaneNotFound) {
		t.Fatalf("missing pane error = %v, want %v", err, ErrPaneNotFound)
	}
	if _, err := m.ResizePaneGrid(1, CellMetrics{}); !errors.Is(err, ErrInvalidGeometry) {
		t.Fatalf("invalid metrics error = %v, want %v", err, ErrInvalidGeometry)
	}
	after, _ := m.PaneView(1)
	if after.Geometry != before.Geometry || after.DesiredSize != before.DesiredSize || m.paneMetrics[1] != beforeMetrics {
		t.Fatalf("rejected resize mutated pane: before=%#v after=%#v metrics=%#v", before, after, m.paneMetrics[1])
	}
}

func TestMuxPaletteBaseQueriesAndPaneLocalOverrides(t *testing.T) {
	m, session, wakes := newTestMux(t)
	base := core.DefaultPaletteBase()
	base.FG = core.RGB{R: 0x12, G: 0x34, B: 0x56}
	base.Indexed[7] = core.RGB{R: 0x65, G: 0x43, B: 0x21}
	m.SetPaletteBase(base)
	if got := m.panes[1].terminal.PaletteBase(); got != base {
		t.Fatalf("existing pane base = %#v, want %#v", got, base)
	}
	if err := session.feed([]byte("\x1b]4;7;?\a\x1b]10;?\x1b\\")); err != nil {
		t.Fatal(err)
	}
	awaitWake(t, wakes)
	m.Drain(16)
	if got, want := string(session.written()), "\x1b]4;7;rgb:6565/4343/2121\x1b\\\x1b]10;rgb:1212/3434/5656\x1b\\"; got != want {
		t.Fatalf("palette replies = %q, want %q", got, want)
	}
	second, _, err := m.Split(1, SplitColumns, SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.panes[second].terminal.PaletteBase(); got != base {
		t.Fatalf("new pane base = %#v, want %#v", got, base)
	}
	m.panes[1].terminal.SetPaletteIndex(7, core.RGB{R: 1, G: 2, B: 3})
	if m.panes[second].terminal.PaletteOverrides().HasIndexed(7) {
		t.Fatal("OSC indexed override leaked to sibling pane")
	}
	newBase := base
	newBase.Indexed[7] = core.RGB{R: 9, G: 8, B: 7}
	m.SetPaletteBase(newBase)
	if got := m.panes[1].terminal.EffectivePaletteIndex(7); got != (core.RGB{R: 1, G: 2, B: 3}) {
		t.Fatalf("base reload replaced pane override: %#v", got)
	}
	if got := m.panes[second].terminal.EffectivePaletteIndex(7); got != newBase.Indexed[7] {
		t.Fatalf("sibling did not receive reloaded base: %#v", got)
	}
}
