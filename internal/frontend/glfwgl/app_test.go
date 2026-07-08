//go:build glfw

package glfwgl

import "testing"

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
			if got := scrollRowsFromWheelDelta(tt.yoff); got != tt.want {
				t.Fatalf("want %d got %d", tt.want, got)
			}
		})
	}
}
