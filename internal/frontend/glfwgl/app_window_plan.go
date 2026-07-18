//go:build glfw

package glfwgl

import (
	"fmt"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) applyInitialGridWindowPlan(w *glfw.Window, sx, sy float32) error {
	if a.cfg.Window.InitialRows == 0 {
		return nil
	}
	insets := projectOuterInsets(OuterInsets{Left: float64(a.cfg.Window.PaddingLeft), Right: float64(a.cfg.Window.PaddingRight), Top: float64(a.cfg.Window.PaddingTop), Bottom: float64(a.cfg.Window.PaddingBottom)}, max(sx, sy))
	gutter := 0
	if scrollbarEnabled(a.cfg.Scrollbar) && a.cfg.Scrollbar.StableGutter {
		gutter = int(float32(a.cfg.Scrollbar.ReservedWidthPX) * max(sx, sy))
	}
	plan, err := checkedStartupWindowPlan(startupWindowPlanInput{Rows: a.cfg.Window.InitialRows, Cols: a.cfg.Window.InitialCols, CellWidth: int(a.cellW), CellHeight: int(a.cellH), InsetLeft: insets.Left, InsetRight: insets.Right, InsetTop: insets.Top, InsetBottom: insets.Bottom, Gutter: gutter, ScaleX: float64(sx), ScaleY: float64(sy)})
	if err != nil {
		return err
	}
	w.SetSize(plan.WindowWidth, plan.WindowHeight)
	fbw, fbh := w.GetFramebufferSize()
	actual := resolveWindowGeometry(fbw, fbh, insets, float32(gutter)).Content
	if actual.Width/int(a.cellW) != a.cfg.Window.InitialCols || actual.Height/int(a.cellH) != a.cfg.Window.InitialRows {
		return fmt.Errorf("startup window did not converge to requested %dx%d grid", a.cfg.Window.InitialCols, a.cfg.Window.InitialRows)
	}
	return nil
}
