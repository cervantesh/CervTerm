package glfwgl

import (
	"cervterm/internal/frontend/gpu"
	"testing"
)

func TestWindowGeometryReservesTopTabBarWithScrollbar(t *testing.T) {
	g := resolveWindowGeometryWithTabBar(800, 600, FramebufferInsets{Left: 10, Right: 20, Top: 30, Bottom: 40}, 12, 28, "top")
	if g.TabBar != (gpu.ClipRect{X: 10, Y: 30, Width: 758, Height: 28}) {
		t.Fatalf("bar=%#v", g.TabBar)
	}
	if g.Content != (gpu.ClipRect{X: 10, Y: 58, Width: 758, Height: 502}) {
		t.Fatalf("content=%#v", g.Content)
	}
	if g.ScrollbarTrack.Y != 58 || g.ScrollbarTrack.Height != 502 {
		t.Fatalf("track=%#v", g.ScrollbarTrack)
	}
}
func TestWindowGeometryReservesBottomTabBar(t *testing.T) {
	g := resolveWindowGeometryWithTabBar(400, 300, FramebufferInsets{Top: 5, Bottom: 7}, 0, 24, "bottom")
	if g.Content.Y != 5 || g.Content.Height != 264 || g.TabBar.Y != 269 || g.TabBar.Height != 24 {
		t.Fatalf("geometry=%#v", g)
	}
}
func TestWindowGeometryClampsCompressedTabBar(t *testing.T) {
	g := resolveWindowGeometryWithTabBar(20, 10, FramebufferInsets{Top: 8, Bottom: 8}, 0, 30, "top")
	if g.TabBar.Height != 0 || g.Content.Height != 0 {
		t.Fatalf("geometry=%#v", g)
	}
}
