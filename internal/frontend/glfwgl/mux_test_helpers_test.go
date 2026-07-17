//go:build glfw

package glfwgl

import (
	"errors"
	"io"
	"sync"
	"testing"

	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
)

type failingTestFactory struct{}

func (failingTestFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	return nil, errors.New("test session unavailable")
}

func newMuxTestApp(t *testing.T, cols, rows int) *App {
	t.Helper()
	m := termmux.New(failingTestFactory{}, termmux.Options{})
	_, pane, events, _ := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: cols, Height: rows}, termmux.CellMetrics{CellWidth: 1, CellHeight: 1})
	a := &App{
		mux:               m,
		focusedPane:       pane,
		paneUI:            make(map[termmux.PaneID]*paneUIState),
		pendingPaneScroll: make(map[termmux.PaneID]int),
		cellW:             1,
		cellH:             1,
	}
	a.handleMuxEvents(events)
	a.syncFocusedProjection()
	resetEvents, resetErr := m.FeedFallback(pane, []byte("\x1bc"))
	if resetErr != nil {
		t.Fatal(resetErr)
	}
	a.handleMuxEvents(resetEvents)
	t.Cleanup(func() { _ = m.Shutdown() })
	return a
}

func feedTestPane(t *testing.T, a *App, data []byte) {
	t.Helper()
	events, err := a.mux.FeedFallback(a.focusedPane, data)
	if err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents(events)
}

type idleTestSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	once   sync.Once
}

func newIdleTestSession() *idleTestSession {
	r, w := io.Pipe()
	return &idleTestSession{reader: r, writer: w}
}

func (s *idleTestSession) Reader() io.Reader              { return s.reader }
func (s *idleTestSession) Write(data []byte) (int, error) { return len(data), nil }
func (s *idleTestSession) Resize(pty.Size) error          { return nil }
func (s *idleTestSession) Close() error {
	s.once.Do(func() {
		_ = s.writer.Close()
		_ = s.reader.Close()
	})
	return nil
}

type idleTestFactory struct{}

func (idleTestFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	return newIdleTestSession(), nil
}

type capturingTestFactory struct {
	mu      sync.Mutex
	options []pty.Options
}

func (f *capturingTestFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	f.mu.Lock()
	f.options = append(f.options, options)
	f.mu.Unlock()
	return newIdleTestSession(), nil
}

func (f *capturingTestFactory) reset() {
	f.mu.Lock()
	f.options = nil
	f.mu.Unlock()
}

func (f *capturingTestFactory) last() (pty.Options, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.options) == 0 {
		return pty.Options{}, false
	}
	return f.options[len(f.options)-1], true
}

func newRunningMuxTestApp(t *testing.T) *App {
	t.Helper()
	m := termmux.New(idleTestFactory{}, termmux.Options{})
	_, pane, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{
		mux: m, focusedPane: pane, paneUI: make(map[termmux.PaneID]*paneUIState),
		pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 8, cellH: 16,
	}
	a.handleMuxEvents(events)
	a.syncFocusedProjection()
	t.Cleanup(func() { _ = m.Shutdown() })
	return a
}
