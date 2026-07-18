//go:build glfw

package glfwgl

import (
	"math"
	"testing"
)

func TestCheckedStartupWindowPlan(t *testing.T) {
	plan, err := checkedStartupWindowPlan(startupWindowPlanInput{Rows: 24, Cols: 80, CellWidth: 9, CellHeight: 16, InsetLeft: 6, InsetRight: 6, InsetTop: 6, InsetBottom: 6, Gutter: 12, ScaleX: 1.25, ScaleY: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if plan.FramebufferWidth != 744 || plan.FramebufferHeight != 396 || plan.WindowWidth != 596 || plan.WindowHeight != 264 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestCheckedStartupWindowPlanRejectsInvalidAndOverflow(t *testing.T) {
	base := startupWindowPlanInput{Rows: 24, Cols: 80, CellWidth: 9, CellHeight: 16, ScaleX: 1, ScaleY: 1}
	cases := []startupWindowPlanInput{
		{Rows: 9, Cols: 80, CellWidth: 9, CellHeight: 16, ScaleX: 1, ScaleY: 1},
		{Rows: 24, Cols: 80, CellWidth: 0, CellHeight: 16, ScaleX: 1, ScaleY: 1},
		{Rows: 24, Cols: 1000, CellWidth: math.MaxInt, CellHeight: 16, ScaleX: 1, ScaleY: 1},
	}
	for _, in := range cases {
		if _, err := checkedStartupWindowPlan(in); err == nil {
			t.Fatalf("expected rejection for %+v", in)
		}
	}
	base.MaxWindowWidth = 100
	if _, err := checkedStartupWindowPlan(base); err == nil {
		t.Fatal("expected oversized-window rejection")
	}
}

func TestCheckedStartupWindowPlanIncludesTabBarHeight(t *testing.T) {
	plan, err := checkedStartupWindowPlan(startupWindowPlanInput{Rows: 24, Cols: 80, CellWidth: 9, CellHeight: 16, TabBarHeight: 28, ScaleX: 1, ScaleY: 1})
	if err != nil {
		t.Fatal(err)
	}
	if plan.FramebufferHeight != 412 {
		t.Fatalf("height=%d", plan.FramebufferHeight)
	}
}
