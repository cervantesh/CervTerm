package glfwgl

import "testing"

func TestEffectiveDPI(t *testing.T) {
	tests := []struct {
		name string
		x, y float32
		want float64
	}{
		{"one", 1, 1, 96},
		{"one and quarter", 1.25, 1.25, 120},
		{"one and half", 1.5, 1.5, 144},
		{"two", 2, 2, 192},
		{"lower clamp", 0.5, 0.75, 96},
		{"upper clamp", 5, 5, 384},
		{"asymmetric", 1.25, 2, 192},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveDPI(tt.x, tt.y); got != tt.want {
				t.Fatalf("effectiveDPI(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}
