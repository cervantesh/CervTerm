//go:build glfw

package glfwgl

import (
	"bytes"
	"io"
	"sync"
	"testing"

	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
	termsel "cervterm/internal/selection"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestMuxBindingsSplitFocusCloseAndPaneUIIsolation(t *testing.T) {
	a, factory := newRecordingActionApp(t)
	first := a.focusedPane
	a.selection = selectionState{active: true, start: termsel.Point{Row: 1, Col: 2}, end: termsel.Point{Row: 1, Col: 4}}

	a.handleKeyEvent(glfw.KeyEqual, glfw.Press, glfw.ModAlt|glfw.ModShift)
	ids := a.mux.PaneIDs()
	if len(ids) != 2 || a.focusedPane == first {
		t.Fatalf("split panes=%v focus=%d first=%d", ids, a.focusedPane, first)
	}
	second := a.focusedPane
	if a.selection.active {
		t.Fatal("new pane inherited the first pane selection")
	}
	a.selection = selectionState{active: true, start: termsel.Point{Row: 2, Col: 3}, end: termsel.Point{Row: 2, Col: 5}}

	a.handleKeyEvent(glfw.KeyLeft, glfw.Press, glfw.ModAlt)
	if a.focusedPane != first {
		t.Fatalf("left focus = %d, want %d", a.focusedPane, first)
	}
	if !a.selection.active || a.selection.start != (termsel.Point{Row: 1, Col: 2}) {
		t.Fatalf("first pane selection was not restored: %#v", a.selection)
	}
	a.handleKeyEvent(glfw.KeyRight, glfw.Press, glfw.ModAlt)
	if a.focusedPane != second {
		t.Fatalf("right focus = %d, want %d", a.focusedPane, second)
	}
	if !a.selection.active || a.selection.start != (termsel.Point{Row: 2, Col: 3}) {
		t.Fatalf("second pane selection was not restored: %#v", a.selection)
	}

	a.handleKeyEvent(glfw.KeyW, glfw.Press, glfw.ModControl|glfw.ModShift)
	if got := a.mux.PaneIDs(); len(got) != 1 || got[0] != first || a.focusedPane != first {
		t.Fatalf("close/collapse panes=%v focus=%d", got, a.focusedPane)
	}
	if _, exists := a.paneUI[second]; exists {
		t.Fatalf("closed pane %d UI state was recreated", second)
	}
	if _, exists := a.pendingPaneResize[second]; exists {
		t.Fatalf("closed pane %d retained a pending resize", second)
	}
	assertNoRecordedPaneInput(t, factory)
}

func TestMuxRowSplitCreatesIndependentGeometry(t *testing.T) {
	a, factory := newRecordingActionApp(t)
	a.handleKeyEvent(glfw.KeyMinus, glfw.Press, glfw.ModAlt|glfw.ModShift)
	layout, err := a.mux.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Panes) != 2 || len(layout.Dividers) != 1 {
		t.Fatalf("layout = %#v", layout)
	}
	if layout.Panes[0].Pixels.Bottom() > layout.Dividers[0].Pixels.Y || layout.Panes[1].Pixels.Y < layout.Dividers[0].Pixels.Bottom() {
		t.Fatalf("row split overlaps divider: %#v", layout)
	}
	assertNoRecordedPaneInput(t, factory)
}

func assertNoRecordedPaneInput(t *testing.T, factory *recordingPaneFactory) {
	t.Helper()
	for i, session := range factory.sessions {
		if got := session.text(); got != "" {
			t.Fatalf("pane session %d received binding bytes %q", i, got)
		}
	}
}

type recordingPaneSession struct {
	reader      *io.PipeReader
	writer      *io.PipeWriter
	mu          sync.Mutex
	writes      bytes.Buffer
	once        sync.Once
	resizeErr   error
	resizeCalls int
}

func newRecordingPaneSession() *recordingPaneSession {
	r, w := io.Pipe()
	return &recordingPaneSession{reader: r, writer: w}
}
func (s *recordingPaneSession) Reader() io.Reader { return s.reader }
func (s *recordingPaneSession) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes.Write(data)
}
func (s *recordingPaneSession) Resize(pty.Size) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizeCalls++
	return s.resizeErr
}
func (s *recordingPaneSession) Close() error {
	s.once.Do(func() { _ = s.writer.Close(); _ = s.reader.Close() })
	return nil
}
func (s *recordingPaneSession) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes.String()
}
func (s *recordingPaneSession) setResizeError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizeErr = err
}

type recordingPaneFactory struct{ sessions []*recordingPaneSession }

func (f *recordingPaneFactory) Spawn(rows, cols uint16, options pty.Options) (pty.Session, error) {
	s := newRecordingPaneSession()
	f.sessions = append(f.sessions, s)
	return s, nil
}

func TestPaneHostRemainsBoundToEventOrigin(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	defer m.Shutdown()
	_, first, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{mux: m, focusedPane: first, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 8, cellH: 16}
	a.handleMuxEvents(events)
	host := paneHost{app: a, pane: first}
	second, events, err := m.Split(first, termmux.SplitColumns, termmux.SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	a.handleMuxEvents(events)
	if a.focusedPane != second {
		t.Fatalf("focus=%d want=%d", a.focusedPane, second)
	}

	host.WriteInput("origin-only")
	if got := factory.sessions[0].text(); got != "origin-only" {
		t.Fatalf("origin session write=%q", got)
	}
	if got := factory.sessions[1].text(); got != "" {
		t.Fatalf("focused sibling received origin write=%q", got)
	}
	host.SetTitle("background-title")
	a.processTermEvents(false)
	firstView, _ := m.PaneView(first)
	secondView, _ := m.PaneView(second)
	if firstView.Snapshot.Title != "background-title" || secondView.Snapshot.Title == "background-title" {
		t.Fatalf("title routing first=%q second=%q", firstView.Snapshot.Title, secondView.Snapshot.Title)
	}
}
