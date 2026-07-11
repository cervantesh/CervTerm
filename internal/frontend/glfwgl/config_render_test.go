//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"
)

func TestConfigColor(t *testing.T) {
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	if got := configColor("#0A1B2C", fallback); got != (color.RGBA{R: 0x0A, G: 0x1B, B: 0x2C, A: 255}) {
		t.Fatalf("configColor parsed %#v", got)
	}
	if got := configColor("bad", fallback); got != fallback {
		t.Fatalf("configColor fallback = %#v, want %#v", got, fallback)
	}
}

func TestCursorThicknessPixels(t *testing.T) {
	if got := cursorThicknessPixels(0.25, 8, 16); got != 2 {
		t.Fatalf("relative thickness = %v, want 2", got)
	}
	if got := cursorThicknessPixels(3, 8, 16); got != 3 {
		t.Fatalf("absolute thickness = %v, want 3", got)
	}
}
