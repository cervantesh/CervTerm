//go:build glfw

package glfwgl

import (
	"errors"
	"testing"
	"time"

	"cervterm/internal/fontglyph"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func splitZoomTestPane(t *testing.T, a *App) (termmux.PaneID, termmux.PaneID) {
	t.Helper()
	first := a.focusedPane
	if !a.handleMuxKey(glfw.KeyEqual, glfw.ModAlt|glfw.ModShift) {
		t.Fatal("column split chord was not consumed")
	}
	second := a.focusedPane
	if first == 0 || second == 0 || first == second {
		t.Fatalf("split focus first=%d second=%d", first, second)
	}
	return first, second
}

func TestPerPaneZoomStateIsIndependent(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg.Font.Size = 14
	a.zoom.base = 14
	firstState := a.ensurePaneUI(a.focusedPane)
	firstState.font = paneFontState{fontSize: 12, cellW: 7, cellH: 14, baseline: 11}
	first, second := splitZoomTestPane(t, a)

	secondState := a.ensurePaneUI(second)
	secondState.font = paneFontState{fontSize: 18, cellW: 11, cellH: 22, baseline: 17}
	secondBefore := secondState.font
	if !a.focusPane(first) {
		t.Fatalf("could not focus first pane %d", first)
	}

	a.applyFontSize(13)
	if got := a.ensurePaneUI(first).font; got.fontSize != 12 || !got.pending || got.pendingTarget != 13 {
		t.Fatalf("focused pane font state = %#v, want live 12 and pending 13", got)
	}
	if got := a.ensurePaneUI(second).font; got != secondBefore {
		t.Fatalf("sibling font state changed: got %#v, want %#v", got, secondBefore)
	}
}

func TestZoomResetTargetsConfiguredBaseForFocusedPane(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg.Font.Size = 13.5
	a.cfg.Render.ZoomResetHotkey = "ctrl+0"
	a.initZoomHotkeys()
	if !a.zoom.resetOK || a.zoom.base != 13.5 {
		t.Fatalf("reset binding/base = ok:%v base:%v", a.zoom.resetOK, a.zoom.base)
	}
	first, second := splitZoomTestPane(t, a)
	firstBefore := a.ensurePaneUI(first).font
	secondState := a.ensurePaneUI(second)
	secondState.font.fontSize = 21

	if !a.handleZoomKey(glfw.Key0, glfw.ModControl) {
		t.Fatal("configured reset chord was not consumed")
	}
	if !secondState.font.pending || secondState.font.pendingTarget != 13.5 {
		t.Fatalf("focused reset state = %#v, want pending configured base 13.5", secondState.font)
	}
	if got := a.ensurePaneUI(first).font; got != firstBefore {
		t.Fatalf("reset changed sibling state: got %#v, want %#v", got, firstBefore)
	}
}

func TestPendingZoomRemainsAttachedAfterFocusChange(t *testing.T) {
	a := newRunningMuxTestApp(t)
	first, second := splitZoomTestPane(t, a)
	if !a.focusPane(first) {
		t.Fatalf("could not focus first pane %d", first)
	}
	a.applyFontSize(17)
	firstPending := a.ensurePaneUI(first).font

	if !a.focusPane(second) {
		t.Fatalf("could not focus second pane %d", second)
	}
	if got := a.ensurePaneUI(first).font; got != firstPending || !got.pending || got.pendingTarget != 17 {
		t.Fatalf("original pane lost pending target after focus change: got %#v, want %#v", got, firstPending)
	}
	if got := a.ensurePaneUI(second).font; got.pending {
		t.Fatalf("newly focused sibling acquired original pending target: %#v", got)
	}

	a.applyFontSize(22)
	if got := a.ensurePaneUI(first).font; got != firstPending {
		t.Fatalf("zooming sibling changed original pending state: got %#v, want %#v", got, firstPending)
	}
	if got := a.ensurePaneUI(second).font; !got.pending || got.pendingTarget != 22 {
		t.Fatalf("sibling pending state = %#v, want target 22", got)
	}
}

func TestPaneGridMetricsAndFramebufferPointUseTargetPaneCells(t *testing.T) {
	const pane termmux.PaneID = 7
	a := &App{
		cellW: 8, cellH: 16, paddingX: 2, paddingY: 3,
		paneUI: map[termmux.PaneID]*paneUIState{
			pane: {font: paneFontState{fontSize: 18, cellW: 10, cellH: 20}},
		},
	}
	geometry := termmux.PaneGeometry{
		Pane:   pane,
		Pixels: termmux.PixelRect{X: 100, Y: 50, Width: 120, Height: 120},
		Cols:   10,
		Rows:   5,
	}

	metrics := a.paneGridMetrics(pane, geometry.Cols, geometry.Rows)
	if metrics.cellW != 10 || metrics.cellH != 20 || metrics.cols != 10 || metrics.rows != 5 {
		t.Fatalf("pane metrics = %#v, want target pane 10x20 cells over 10x5 grid", metrics)
	}
	point := a.pointForPaneFramebufferPosition(pane, geometry, 121, 92)
	if point.Row != 1 || point.Col != 1 {
		t.Fatalf("framebuffer point = %#v, want row 1 col 1 using target pane metrics", point)
	}
}

func TestInheritPaneFontStateCopiesOnlyLiveProjection(t *testing.T) {
	const source, target termmux.PaneID = 1, 2
	now := time.Now()
	a := &App{
		cellW: 8,
		cellH: 16,
		paneUI: map[termmux.PaneID]*paneUIState{
			source: {font: paneFontState{
				fontSize: 19, cellW: 12, cellH: 24, baseline: 18,
				pendingTarget: 23, pending: true, ptyDirty: true, deadline: now,
			}},
			target: {font: paneFontState{
				fontSize: 9, cellW: 6, cellH: 12, baseline: 9,
				pendingTarget: 10, pending: true, ptyDirty: true, deadline: now.Add(time.Second),
			}},
		},
	}

	a.inheritPaneFontState(target, source)
	got := a.ensurePaneUI(target).font
	if got.fontSize != 19 || got.cellW != 12 || got.cellH != 24 || got.baseline != 18 {
		t.Fatalf("inherited live projection = %#v", got)
	}
	if got.pendingTarget != 0 || got.pending || got.ptyDirty || !got.deadline.IsZero() {
		t.Fatalf("inherited transient state = %#v, want zero pending/debounce state", got)
	}
}

func TestNextWakeUsesEarliestPaneZoomDeadline(t *testing.T) {
	now := time.Now()
	a := &App{paneUI: map[termmux.PaneID]*paneUIState{
		1: {font: paneFontState{pending: true, deadline: now.Add(80 * time.Millisecond)}},
		2: {font: paneFontState{pending: true, deadline: now.Add(20 * time.Millisecond)}},
		3: {font: paneFontState{pending: false, deadline: now.Add(5 * time.Millisecond)}},
	}}
	if got := a.nextWakeTimeout(now); got != 20*time.Millisecond {
		t.Fatalf("next wake = %v, want earliest pending pane deadline %v", got, 20*time.Millisecond)
	}
}

func TestApplyPanePTYResizeTargetsOnlyRequestedPane(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	_, first, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{mux: m, focusedPane: first, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 8, cellH: 16}
	a.handleMuxEvents(events)
	second, events, err := m.Split(first, termmux.SplitColumns, termmux.SpawnSpec{})
	if err != nil {
		t.Fatal(err)
	}
	a.inheritPaneFontState(second, first)
	a.handleMuxEvents(events)
	if len(factory.sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(factory.sessions))
	}
	firstBefore := recordingResizeCount(factory.sessions[0])
	secondBefore := recordingResizeCount(factory.sessions[1])

	if _, err := m.ResizePaneGrid(second, termmux.CellMetrics{CellWidth: 10, CellHeight: 20}); err != nil {
		t.Fatal(err)
	}
	if !a.applyPanePTYResize(second) {
		t.Fatal("targeted PTY settlement failed")
	}
	if got := recordingResizeCount(factory.sessions[0]); got != firstBefore {
		t.Fatalf("sibling PTY resize calls = %d, want unchanged %d", got, firstBefore)
	}
	if got := recordingResizeCount(factory.sessions[1]); got != secondBefore+1 {
		t.Fatalf("target PTY resize calls = %d, want %d", got, secondBefore+1)
	}
}

func recordingResizeCount(session *recordingPaneSession) int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.resizeCalls
}

func TestPaneHostFontSizeStaysBoundToOriginPane(t *testing.T) {
	a := newRunningMuxTestApp(t)
	a.cfg.Font.Family = "Go Mono"
	a.cfg.Font.Size = 14
	a.cfg.Render.TextGamma = 1
	renderer := &atlasTestRenderer{}
	atlas, err := newGlyphAtlasWithBackendFactory(renderer, a.fontSpec(14, 1, 1), 1, 0, func(spec fontglyph.Spec) (fontglyph.Backend, error) {
		size := max(1, int(spec.Size))
		return &atlasTestBackend{cellW: size, cellH: size + 4, baseline: size}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.atlas = atlas
	t.Cleanup(atlas.close)
	first, second := splitZoomTestPane(t, a)
	a.ensurePaneUI(first).font.fontSize = 12
	a.ensurePaneUI(second).font.fontSize = 18
	if !a.focusPane(second) {
		t.Fatal("could not focus second pane")
	}

	host := paneHost{app: a, pane: first}
	if got := host.FontSize(); got != 12 {
		t.Fatalf("background host FontSize = %v, want 12", got)
	}
	host.SetFontSize(16)
	if got := a.ensurePaneUI(first).font.fontSize; got != 16 {
		t.Fatalf("background host changed pane to %v, want 16", got)
	}
	if got := a.ensurePaneUI(second).font.fontSize; got != 18 {
		t.Fatalf("background host changed focused sibling to %v", got)
	}
}

func TestSplitInheritsFontAfterCommittedResizeError(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	_, first, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{mux: m, focusedPane: first, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 8, cellH: 16}
	a.handleMuxEvents(events)
	a.ensurePaneUI(first).font = paneFontState{fontSize: 19, cellW: 12, cellH: 24, baseline: 18}
	factory.sessions[0].setResizeError(errors.New("persistent resize failure"))
	if !a.handleMuxKey(glfw.KeyEqual, glfw.ModAlt|glfw.ModShift) {
		t.Fatal("split chord was not consumed")
	}
	second := a.focusedPane
	if second == first || second == 0 {
		t.Fatalf("split did not commit a new pane: %d", second)
	}
	got := a.ensurePaneUI(second).font
	if got.fontSize != 19 || got.cellW != 12 || got.cellH != 24 {
		t.Fatalf("committed child font = %#v, want inherited source projection", got)
	}
}

func TestPendingZoomResizeFailureUsesBoundedRetries(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	t.Cleanup(func() { _ = m.Shutdown() })
	_, pane, events, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{mux: m, focusedPane: pane, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int), cellW: 8, cellH: 16}
	a.handleMuxEvents(events)
	if _, err := m.ResizePaneGrid(pane, termmux.CellMetrics{CellWidth: 10, CellHeight: 20}); err != nil {
		t.Fatal(err)
	}
	factory.sessions[0].setResizeError(errors.New("persistent resize failure"))
	state := a.ensurePaneUI(pane)
	state.font = paneFontState{fontSize: 14, cellW: 10, cellH: 20, pendingTarget: 14, pending: true, ptyDirty: true, deadline: time.Now().Add(-time.Second)}

	for attempt := 1; attempt <= paneZoomResizeMaxAttempts; attempt++ {
		a.applyPendingZoom()
		if attempt < paneZoomResizeMaxAttempts {
			if !state.font.pending || !state.font.ptyDirty {
				t.Fatalf("retry %d disarmed early: %#v", attempt, state.font)
			}
			state.font.deadline = time.Now().Add(-time.Second)
		}
	}
	if state.font.pending || state.font.ptyDirty {
		t.Fatalf("persistent failure was not disarmed after bounded retries: %#v", state.font)
	}
}
