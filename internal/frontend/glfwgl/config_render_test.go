//go:build glfw

package glfwgl

import (
	"image/color"
	"testing"

	"cervterm/internal/config"
)

func TestConfigColor(t *testing.T) {
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	if got := configColor("#0A1B2C", fallback); got != (color.RGBA{R: 0x0A, G: 0x1B, B: 0x2C, A: 255}) {
		t.Fatalf("configColor parsed %#v", got)
	}
	if got := configColor("#0A1B2C80", fallback); got != (color.RGBA{R: 0x0A, G: 0x1B, B: 0x2C, A: 0x80}) {
		t.Fatalf("configColor RGBA parsed %#v", got)
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

func TestTransparentBackgroundDisablesSubpixelRaster(t *testing.T) {
	a := &App{cfg: config.Defaults()}
	a.cfg.Render.TextRaster = "subpixel"
	if got := a.effectiveTextRaster(); got != "go" {
		t.Fatalf("effective raster = %q, want go", got)
	}
	if got := a.fontSpec(14, 1, 1).TextRaster; got != "go" {
		t.Fatalf("transparent pane raster = %q, want go", got)
	}
	a.cfg.Colors.Background = "#080B12FF"
	if got := a.effectiveTextRaster(); got != "subpixel" {
		t.Fatalf("opaque effective raster = %q, want subpixel", got)
	}
	if got := a.fontSpec(14, 1, 1).TextRaster; got != "subpixel" {
		t.Fatalf("opaque pane raster = %q, want subpixel", got)
	}
}
