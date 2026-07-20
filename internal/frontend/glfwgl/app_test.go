//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

func TestScrollRowsFromWheelDelta(t *testing.T) {
	tests := []struct {
		name string
		yoff float64
		want int
	}{
		{name: "zero", yoff: 0, want: 0},
		{name: "fraction up", yoff: 0.25, want: 1},
		{name: "fraction down", yoff: -0.25, want: -1},
		{name: "unit up", yoff: 1, want: 3},
		{name: "unit down", yoff: -1, want: -3},
		{name: "large up", yoff: 2, want: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrollRowsFromWheelDelta(tt.yoff, 3); got != tt.want {
				t.Fatalf("want %d got %d", tt.want, got)
			}
		})
	}
}

func TestScrollRowsUsesConfiguredMultiplier(t *testing.T) {
	if got := scrollRowsFromWheelDelta(1, 7); got != 7 {
		t.Fatalf("configured multiplier = %d, want 7", got)
	}
}

func TestGridSizeReservesScrollbarSlot(t *testing.T) {
	cfg := config.Defaults()
	a := &App{cfg: cfg, cellW: 10, cellH: 20, uiScale: 1}
	a.insets = projectOuterInsets(a.outerInsets(), a.uiScale)
	cols, _ := a.gridSize(1000, 500)
	cfg.Scrollbar.Enabled = false
	a.cfg = cfg
	without, _ := a.gridSize(1000, 500)
	if cols >= without || without-cols < 1 {
		t.Fatalf("grid cols with scrollbar=%d without=%d", cols, without)
	}
}

func TestMuxContentBoundsReserveScrollbarGutter(t *testing.T) {
	cfg := config.Defaults()
	a := &App{cfg: cfg, uiScale: 1}
	a.insets = projectOuterInsets(a.outerInsets(), a.uiScale)
	bounds := a.muxContentBounds(1000, 500)
	want := termmux.PixelRect{X: 6, Y: 6, Width: 1000 - 2*6 - cfg.Scrollbar.ReservedWidthPX, Height: 500 - 2*6}
	if bounds != want {
		t.Fatalf("mux bounds = %#v, want %#v", bounds, want)
	}
}

func TestGridSizeUsesAsymmetricOuterInsets(t *testing.T) {
	cfg := config.Defaults()
	cfg.Window.PaddingLeft, cfg.Window.PaddingRight = 1, 2
	cfg.Window.PaddingTop, cfg.Window.PaddingBottom = 3, 4
	cfg.Scrollbar.Enabled = false
	a := &App{cfg: cfg, cellW: 10, cellH: 10, uiScale: 1}
	a.insets = projectOuterInsets(a.outerInsets(), a.uiScale)
	if got := a.muxContentBounds(100, 100); got != (termmux.PixelRect{X: 1, Y: 3, Width: 97, Height: 93}) {
		t.Fatalf("content bounds = %#v", got)
	}
	cols, rows := a.gridSize(100, 100)
	if cols != 9 || rows != 9 {
		t.Fatalf("grid = %dx%d, want 9x9", cols, rows)
	}
}
