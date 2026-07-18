package glfwgl

import (
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/mux"
	"reflect"
	"testing"
)

func tabViews(n int, active int) []mux.TabView {
	out := make([]mux.TabView, n)
	for i := range out {
		out[i] = mux.TabView{ID: mux.TabID(i + 1), Title: "tab", Active: i == active, Revision: uint64(i + 1)}
	}
	return out
}
func TestTabBarVisibilityPolicy(t *testing.T) {
	if tabBarVisible("multiple", 1) || !tabBarVisible("multiple", 2) || !tabBarVisible("always", 1) || tabBarVisible("hidden", 2) {
		t.Fatal("visibility policy")
	}
}
func TestTabBarLayoutKeepsActiveVisibleAndHitsDisjoint(t *testing.T) {
	tabs := tabViews(8, 6)
	layout := layoutTabBar(gpu.ClipRect{Width: 330, Height: 30}, tabs, 7, 90, 140, 6, 8, true, true, 0)
	found := false
	for _, item := range layout.Items {
		found = found || item.Tab == 7
		if hit, ok := layout.Hit(item.Close.X, item.Close.Y); !ok || hit.Kind != tabHitClose || hit.Tab != item.Tab {
			t.Fatalf("close hit=%#v %v", hit, ok)
		}
	}
	if !found {
		t.Fatalf("active absent: %#v", layout)
	}
	if hit, ok := layout.Hit(layout.Add.X, layout.Add.Y); !ok || hit.Kind != tabHitAdd {
		t.Fatalf("add hit=%#v %v", hit, ok)
	}
}
func TestTabBarLayoutIsDeterministicAndDoesNotMutateInput(t *testing.T) {
	tabs := tabViews(4, 2)
	before := append([]mux.TabView(nil), tabs...)
	a := layoutTabBar(gpu.ClipRect{X: 4, Y: 8, Width: 500, Height: 28}, tabs, 3, 80, 160, 8, 8, true, true, 1)
	b := layoutTabBar(gpu.ClipRect{X: 4, Y: 8, Width: 500, Height: 28}, tabs, 3, 80, 160, 8, 8, true, true, 1)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(tabs, before) {
		t.Fatal("non-deterministic or mutated input")
	}
}
func TestClipTabTitlePreservesUnicodeClusters(t *testing.T) {
	if got := clipTabTitle("A👩‍💻B", 3); got != "A👩‍💻" {
		t.Fatalf("got %q", got)
	}
	if got := clipTabTitle("e\u0301x", 1); got != "é" {
		t.Fatalf("combining got %q", got)
	}
}
