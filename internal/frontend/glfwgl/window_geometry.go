package glfwgl

import (
	"math"

	"cervterm/internal/frontend/gpu"
)

// OuterInsets are logical window-edge distances before content-scale projection.
type OuterInsets struct {
	Left   float64
	Right  float64
	Top    float64
	Bottom float64
}

// FramebufferInsets are pixel-aligned window-edge distances in framebuffer pixels.
type FramebufferInsets struct {
	Left   int
	Right  int
	Top    int
	Bottom int
}

// WindowGeometry is the authoritative outer-window projection. The stable gutter
// remains at the trailing window edge for compatibility; the right inset separates
// terminal content from that gutter.
type WindowGeometry struct {
	Framebuffer     gpu.ClipRect
	Insets          FramebufferInsets
	ScrollbarGutter int
	Content         gpu.ClipRect
	ScrollbarTrack  gpu.ClipRect
	TabBar          gpu.ClipRect
}

func projectOuterInsets(insets OuterInsets, scale float32) FramebufferInsets {
	if scale <= 0 {
		scale = 1
	}
	project := func(value float64) int {
		return max(0, int(math.Round(value*float64(scale))))
	}
	return FramebufferInsets{
		Left: project(insets.Left), Right: project(insets.Right),
		Top: project(insets.Top), Bottom: project(insets.Bottom),
	}
}

func resolveWindowGeometry(frameWidth, frameHeight int, insets FramebufferInsets, reservedScrollbarGutter float32) WindowGeometry {
	return resolveWindowGeometryWithTabBar(frameWidth, frameHeight, insets, reservedScrollbarGutter, 0, "top")
}

func resolveWindowGeometryWithTabBar(frameWidth, frameHeight int, insets FramebufferInsets, reservedScrollbarGutter float32, tabBarHeight int, position string) WindowGeometry {
	frameWidth, frameHeight = max(0, frameWidth), max(0, frameHeight)
	gutter := min(frameWidth, max(0, int(math.Ceil(float64(reservedScrollbarGutter)))))
	contentX, outerY := min(insets.Left, frameWidth), min(insets.Top, frameHeight)
	outerHeight := max(0, frameHeight-insets.Top-insets.Bottom)
	barHeight := min(outerHeight, max(0, tabBarHeight))
	contentY := outerY
	barY := outerY
	if position == "top" {
		contentY += barHeight
	} else {
		barY += outerHeight - barHeight
	}
	contentHeight := outerHeight - barHeight
	contentWidth := max(0, frameWidth-insets.Left-insets.Right-gutter)
	return WindowGeometry{
		Framebuffer: gpu.ClipRect{Width: frameWidth, Height: frameHeight}, Insets: insets, ScrollbarGutter: gutter,
		Content:        gpu.ClipRect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight},
		ScrollbarTrack: gpu.ClipRect{X: frameWidth - gutter, Y: contentY, Width: gutter, Height: contentHeight},
		TabBar:         gpu.ClipRect{X: contentX, Y: barY, Width: contentWidth, Height: barHeight},
	}
}
