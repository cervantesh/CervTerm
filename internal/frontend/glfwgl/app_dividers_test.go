//go:build glfw

package glfwgl

import (
	"errors"
	"testing"
	"time"

	termmux "cervterm/internal/mux"
)

func TestHitDividerUsesExpandedNearestTarget(t *testing.T) {
	columns := termmux.Divider{Split: 1, Axis: termmux.SplitColumns, Pixels: termmux.PixelRect{X: 100, Width: 1, Height: 200}, Container: termmux.PixelRect{Width: 201, Height: 200}}
	rows := termmux.Divider{Split: 2, Axis: termmux.SplitRows, Pixels: termmux.PixelRect{X: 101, Y: 90, Width: 100, Height: 1}, Container: termmux.PixelRect{X: 101, Width: 100, Height: 200}}
	layout := termmux.Layout{Dividers: []termmux.Divider{columns, rows}}

	if got, ok := hitDivider(layout, 97, 50, 4); !ok || got.Split != columns.Split {
		t.Fatalf("column expanded hit = %#v, %v", got, ok)
	}
	if got, ok := hitDivider(layout, 150, 93, 4); !ok || got.Split != rows.Split {
		t.Fatalf("row expanded hit = %#v, %v", got, ok)
	}
	if got, ok := hitDivider(layout, 90, 50, 4); ok {
		t.Fatalf("distant point unexpectedly hit %#v", got)
	}
}

func TestDividerRatioUsesOwningContainer(t *testing.T) {
	columns := termmux.Divider{Split: 1, Axis: termmux.SplitColumns, Container: termmux.PixelRect{X: 20, Width: 401, Height: 200}}
	if got, ok := dividerRatio(columns, 120, 0); !ok || got != 2_500 {
		t.Fatalf("column ratio = %d, %v; want 2500, true", got, ok)
	}
	rows := termmux.Divider{Split: 2, Axis: termmux.SplitRows, Container: termmux.PixelRect{Y: 40, Width: 200, Height: 201}}
	if got, ok := dividerRatio(rows, 0, 190); !ok || got != 7_500 {
		t.Fatalf("row ratio = %d, %v; want 7500, true", got, ok)
	}
	if got, ok := dividerRatio(termmux.Divider{Axis: termmux.SplitColumns, Container: termmux.PixelRect{Width: 1}}, 0, 0); ok || got != 0 {
		t.Fatalf("degenerate ratio = %d, %v; want 0, false", got, ok)
	}
}

func TestDividerSettlementRetriesTransientPTYResizeFailure(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	defer m.Shutdown()
	bounds := termmux.PixelRect{Width: 800, Height: 480}
	metrics := termmux.CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, first, _, err := m.Bootstrap(termmux.SpawnSpec{}, bounds, metrics); err != nil {
		t.Fatal(err)
	} else if _, _, err := m.Split(first, termmux.SplitColumns, termmux.SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	layout, err := m.Layout()
	if err != nil || len(layout.Dividers) != 1 {
		t.Fatalf("layout=%#v err=%v", layout, err)
	}
	if _, err := m.SetSplitRatio(layout.Dividers[0].Split, 6_000); err != nil {
		t.Fatal(err)
	}
	factory.sessions[0].setResizeError(errors.New("transient resize"))
	a := &App{mux: m, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry), pendingPaneScroll: make(map[termmux.PaneID]int)}
	a.divider.settlePending = true
	a.divider.settleAt = time.Now()
	a.applyPendingDividerResize()
	if !a.divider.settlePending || !a.divider.settleAt.After(time.Now()) {
		t.Fatalf("failed resize was not retained for retry: %#v", a.divider)
	}
	factory.sessions[0].setResizeError(nil)
	a.divider.settleAt = time.Time{}
	a.applyPendingDividerResize()
	if a.divider.settlePending {
		t.Fatal("successful retry did not clear pending settlement")
	}
}

func TestWindowResizeFailureArmsBoundedPaneRetry(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	defer m.Shutdown()
	if _, pane, _, err := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}); err != nil {
		t.Fatal(err)
	} else if events, err := m.ResizeBounds(termmux.PixelRect{Width: 640, Height: 400}); err != nil {
		t.Fatal(err)
	} else {
		a := &App{mux: m, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry), pendingPaneScroll: make(map[termmux.PaneID]int)}
		a.handleMuxEvents(events)
		factory.sessions[0].setResizeError(errors.New("transient window resize"))
		if a.resizePTYToGrid() {
			t.Fatal("failed window resize reported success")
		}
		state := a.ensurePaneUI(pane)
		if !state.font.pending || !state.font.ptyDirty || state.font.resizeAttempt != 1 {
			t.Fatalf("window resize did not arm bounded retry: %#v", state.font)
		}

		state.font = paneFontState{}
		if a.resizePTYToGridReporting(true) {
			t.Fatal("failed divider-owned resize reported success")
		}
		if state.font.pending || state.font.ptyDirty {
			t.Fatalf("divider-owned retry also armed pane retry: %#v", state.font)
		}
	}
}

func TestDividerSettlementStopsAfterPersistentPTYResizeFailure(t *testing.T) {
	factory := &recordingPaneFactory{}
	m := termmux.New(factory, termmux.Options{})
	defer m.Shutdown()
	bounds := termmux.PixelRect{Width: 800, Height: 480}
	metrics := termmux.CellMetrics{CellWidth: 8, CellHeight: 16}
	if _, first, _, err := m.Bootstrap(termmux.SpawnSpec{}, bounds, metrics); err != nil {
		t.Fatal(err)
	} else if _, _, err := m.Split(first, termmux.SplitColumns, termmux.SpawnSpec{}); err != nil {
		t.Fatal(err)
	}
	layout, err := m.Layout()
	if err != nil || len(layout.Dividers) != 1 {
		t.Fatalf("layout=%#v err=%v", layout, err)
	}
	if _, err := m.SetSplitRatio(layout.Dividers[0].Split, 6_000); err != nil {
		t.Fatal(err)
	}
	factory.sessions[0].setResizeError(errors.New("persistent resize"))
	a := &App{mux: m, paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry), pendingPaneScroll: make(map[termmux.PaneID]int)}
	a.divider.settlePending = true
	for attempt := 0; attempt < dividerResizeMaxAttempts; attempt++ {
		a.divider.settleAt = time.Time{}
		a.applyPendingDividerResize()
		if attempt == 0 {
			firstNoticeUntil := a.noticeUntil
			a.divider.settleAt = time.Time{}
			a.applyPendingDividerResize()
			attempt++
			if a.noticeUntil != firstNoticeUntil {
				t.Fatal("silent retry refreshed the resize notice")
			}
		}
	}
	if a.divider.settlePending || a.divider.settleAttempts != 0 {
		t.Fatalf("persistent failure kept retry loop armed: %#v", a.divider)
	}
}

func TestClearDividerCursorResetsStateWithoutWindow(t *testing.T) {
	a := &App{}
	a.divider.cursorSet = true
	a.clearDividerCursor()
	if a.divider.cursorSet {
		t.Fatal("divider cursor state remained set")
	}
}

func TestCancelMouseCaptureClearsPaneAndProjection(t *testing.T) {
	a := &App{focusedPane: 1, mouseCapturePane: 1, paneUI: make(map[termmux.PaneID]*paneUIState)}
	a.mouseReport.down = true
	a.ensurePaneUI(1).mouseReport.down = true
	a.cancelMouseCapture()
	if a.mouseCapturePane != 0 || a.mouseReport.down || a.ensurePaneUI(1).mouseReport.down {
		t.Fatalf("mouse capture was not cleared: capture=%d projection=%#v pane=%#v", a.mouseCapturePane, a.mouseReport, a.ensurePaneUI(1).mouseReport)
	}
}
