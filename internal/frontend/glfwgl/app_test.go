//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/config"
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
	a := &App{cfg: cfg, cellW: 10, cellH: 20, paddingX: 5, paddingY: 5, uiScale: 1}
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
	bounds := a.muxContentBounds(1000, 500)
	wantWidth := 1000 - cfg.Scrollbar.ReservedWidthPX
	if bounds.Width != wantWidth || bounds.Height != 500 {
		t.Fatalf("mux bounds = %#v, want width=%d height=500", bounds, wantWidth)
	}
}
