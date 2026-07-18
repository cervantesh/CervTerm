//go:build glfw

package glfwgl

import (
	"fmt"
	"math"
)

// startupWindowPlan is a resource-free, checked projection from a requested
// terminal grid into framebuffer and GLFW window coordinates.
type startupWindowPlan struct {
	Rows, Cols                          int
	FramebufferWidth, FramebufferHeight int
	WindowWidth, WindowHeight           int
}

type startupWindowPlanInput struct {
	Rows, Cols                      int
	CellWidth, CellHeight           int
	InsetLeft, InsetRight           int
	InsetTop, InsetBottom           int
	Gutter                          int
	ScaleX, ScaleY                  float64
	MaxWindowWidth, MaxWindowHeight int
}

func checkedStartupWindowPlan(in startupWindowPlanInput) (startupWindowPlan, error) {
	if in.Rows < 10 || in.Rows > 1000 || in.Cols < 10 || in.Cols > 1000 {
		return startupWindowPlan{}, fmt.Errorf("startup grid must be between 10 and 1000 cells")
	}
	if in.CellWidth <= 0 || in.CellHeight <= 0 || in.ScaleX <= 0 || in.ScaleY <= 0 || math.IsNaN(in.ScaleX) || math.IsNaN(in.ScaleY) || math.IsInf(in.ScaleX, 0) || math.IsInf(in.ScaleY, 0) {
		return startupWindowPlan{}, fmt.Errorf("invalid startup cell metrics or scale")
	}
	values := []int{in.InsetLeft, in.InsetRight, in.InsetTop, in.InsetBottom, in.Gutter}
	for _, value := range values {
		if value < 0 {
			return startupWindowPlan{}, fmt.Errorf("startup insets and gutter must be non-negative")
		}
	}
	fw, ok := checkedMulAdd(in.Cols, in.CellWidth, in.InsetLeft, in.InsetRight, in.Gutter)
	if !ok {
		return startupWindowPlan{}, fmt.Errorf("startup framebuffer width overflow")
	}
	fh, ok := checkedMulAdd(in.Rows, in.CellHeight, in.InsetTop, in.InsetBottom)
	if !ok {
		return startupWindowPlan{}, fmt.Errorf("startup framebuffer height overflow")
	}
	ww, err := checkedCeilDimension(float64(fw) / in.ScaleX)
	if err != nil {
		return startupWindowPlan{}, fmt.Errorf("startup window width: %w", err)
	}
	wh, err := checkedCeilDimension(float64(fh) / in.ScaleY)
	if err != nil {
		return startupWindowPlan{}, fmt.Errorf("startup window height: %w", err)
	}
	if (in.MaxWindowWidth > 0 && ww > in.MaxWindowWidth) || (in.MaxWindowHeight > 0 && wh > in.MaxWindowHeight) {
		return startupWindowPlan{}, fmt.Errorf("requested startup window exceeds monitor work area")
	}
	return startupWindowPlan{Rows: in.Rows, Cols: in.Cols, FramebufferWidth: fw, FramebufferHeight: fh, WindowWidth: ww, WindowHeight: wh}, nil
}

func checkedMulAdd(multiplier, multiplicand int, addends ...int) (int, bool) {
	if multiplier < 0 || multiplicand < 0 || multiplier != 0 && multiplicand > math.MaxInt/multiplier {
		return 0, false
	}
	value := multiplier * multiplicand
	for _, addend := range addends {
		if addend < 0 || value > math.MaxInt-addend {
			return 0, false
		}
		value += addend
	}
	return value, value > 0
}

func checkedCeilDimension(value float64) (int, error) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) || value > float64(math.MaxInt) {
		return 0, fmt.Errorf("dimension is not representable")
	}
	return int(math.Ceil(value)), nil
}
