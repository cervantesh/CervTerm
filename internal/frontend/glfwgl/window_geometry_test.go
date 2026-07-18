package glfwgl

import (
	"testing"

	"cervterm/internal/frontend/gpu"
)

func TestProjectOuterInsetsRoundsEachSideAtEffectiveScale(t *testing.T) {
	got := projectOuterInsets(OuterInsets{Left: 1, Right: 2, Top: 3, Bottom: 4}, 1.5)
	want := FramebufferInsets{Left: 2, Right: 3, Top: 5, Bottom: 6}
	if got != want {
		t.Fatalf("projected insets = %#v, want %#v", got, want)
	}
	if got := projectOuterInsets(OuterInsets{Left: -1, Right: -2}, 0); got != (FramebufferInsets{}) {
		t.Fatalf("invalid scale/negative insets = %#v, want clamped zero", got)
	}
}

func TestResolveWindowGeometryUsesEverySideAndStableGutter(t *testing.T) {
	got := resolveWindowGeometry(100, 80, FramebufferInsets{Left: 2, Right: 3, Top: 5, Bottom: 6}, 7.2)
	want := WindowGeometry{
		Framebuffer:     gpu.ClipRect{Width: 100, Height: 80},
		Insets:          FramebufferInsets{Left: 2, Right: 3, Top: 5, Bottom: 6},
		ScrollbarGutter: 8,
		Content:         gpu.ClipRect{X: 2, Y: 5, Width: 87, Height: 69},
		ScrollbarTrack:  gpu.ClipRect{X: 92, Y: 5, Width: 8, Height: 69},
		TabBar:          gpu.ClipRect{X: 2, Y: 5, Width: 87},
	}
	if got != want {
		t.Fatalf("geometry = %#v, want %#v", got, want)
	}
}

func TestResolveWindowGeometryClampsCompressedContent(t *testing.T) {
	got := resolveWindowGeometry(10, 8, FramebufferInsets{Left: 8, Right: 8, Top: 5, Bottom: 5}, 12)
	if got.Content != (gpu.ClipRect{X: 8, Y: 5}) {
		t.Fatalf("compressed content = %#v, want zero-sized rect at projected origin", got.Content)
	}
	if got.ScrollbarTrack != (gpu.ClipRect{X: 0, Y: 5, Width: 10}) {
		t.Fatalf("compressed scrollbar track = %#v", got.ScrollbarTrack)
	}
}
