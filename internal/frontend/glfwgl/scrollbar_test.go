//go:build glfw

package glfwgl

import (
	"math"
	"testing"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/render"
)

func TestScrollbarGeometryMapsHistoryExtremes(t *testing.T) {
	cfg := config.Defaults().Scrollbar
	bottom := calculateScrollbarGeometry(120, 200, 0, 20, 1, cfg, 10, 90, 0)
	if bottom.slot.x != 108 || bottom.track.x != 110 || bottom.track.w != 8 {
		t.Fatalf("unexpected slot/track: %#v %#v", bottom.slot, bottom.track)
	}
	if bottom.thumb.h != 24 {
		t.Fatalf("minimum thumb height = %v, want 24", bottom.thumb.h)
	}
	if got, want := bottom.thumb.y, bottom.track.y+bottom.track.h-bottom.thumb.h; got != want {
		t.Fatalf("live thumb y = %v, want bottom %v", got, want)
	}
	top := calculateScrollbarGeometry(120, 200, 0, 20, 1, cfg, 10, 90, 90)
	if top.thumb.y != top.track.y {
		t.Fatalf("max offset thumb y = %v, want top %v", top.thumb.y, top.track.y)
	}
	if got := scrollbarOffsetForThumb(bottom, bottom.track.y); got != 90 {
		t.Fatalf("drag top offset = %d, want 90", got)
	}
	if got := scrollbarOffsetForThumb(bottom, bottom.track.y+bottom.track.h-bottom.thumb.h); got != 0 {
		t.Fatalf("drag bottom offset = %d, want 0", got)
	}
}

func TestScrollbarGeometryNoHistoryAndHiDPI(t *testing.T) {
	cfg := config.Defaults().Scrollbar
	g := calculateScrollbarGeometry(240, 400, 12, 40, 2, cfg, 8, 0, 0)
	if g.slot.w != 24 || g.track.w != 16 || g.thumb.h != 0 {
		t.Fatalf("HiDPI/no-history geometry: %#v", g)
	}
}

func TestPaneScrollbarGeometryUsesFocusedPaneVerticalBounds(t *testing.T) {
	cfg := config.Defaults().Scrollbar
	g := paneScrollbarGeometry(240, 120, 200, 12, 20, 1, cfg, 8, 40, 20)
	if g.slot.y != 120 || g.slot.h != 200 {
		t.Fatalf("slot = %#v, want pane y=120 height=200", g.slot)
	}
	if g.track.y < 132 || g.track.y+g.track.h > 320 {
		t.Fatalf("track escaped focused pane: %#v", g.track)
	}
	if g.thumb.y < g.track.y || g.thumb.y+g.thumb.h > g.track.y+g.track.h {
		t.Fatalf("thumb escaped track: %#v within %#v", g.thumb, g.track)
	}
}

func TestScrollbarFadeReturnsToIdle(t *testing.T) {
	cfg := config.Defaults()
	t0 := time.Unix(100, 0)
	a := &App{cfg: cfg, snap: render.Snapshot{HistoryRows: 10}}
	a.scrollbar.lastActivity = t0
	if got := a.scrollbarOpacity(t0.Add(500*time.Millisecond), 10); got != 1 {
		t.Fatalf("visible-delay opacity = %v, want 1", got)
	}
	mid := a.scrollbarOpacity(t0.Add(1075*time.Millisecond), 10)
	if math.Abs(float64(mid-.5)) > .02 {
		t.Fatalf("mid-fade opacity = %v, want .5", mid)
	}
	a.scrollbar.lastPaintedOpacity = mid
	if _, ok := a.scrollbarWake(t0.Add(1075 * time.Millisecond)); !ok {
		t.Fatal("active fade must schedule a wake")
	}
	a.scrollbar.lastPaintedOpacity = 0
	if got := a.scrollbarOpacity(t0.Add(2*time.Second), 10); got != 0 {
		t.Fatalf("finished opacity = %v, want 0", got)
	}
	if _, ok := a.scrollbarWake(t0.Add(2 * time.Second)); ok {
		t.Fatal("finished fade must return to idle")
	}
}

func TestScrollbarGestureOwnerPersistsAcrossGutter(t *testing.T) {
	a := &App{cfg: config.Defaults()}
	a.scrollbar.owner = pointerOwnerTerminal
	if a.handleScrollbarMove(999, 20) {
		t.Fatal("terminal-owned move must not be captured by scrollbar")
	}
	if a.handleScrollbarWheel(1, 999, 20) {
		t.Fatal("terminal-owned wheel must not be captured by scrollbar")
	}
	a.scrollbar.owner = pointerOwnerScrollbar
	if !a.handleScrollbarMove(0, 0) {
		t.Fatal("scrollbar-owned move must remain captured through release")
	}
}
