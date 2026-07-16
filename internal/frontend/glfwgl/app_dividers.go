//go:build glfw

package glfwgl

import (
	"errors"
	"time"

	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	dividerHitSlop     = 4
	dividerResizeRetry = 100 * time.Millisecond
)

type dividerInteraction struct {
	active        bool
	split         termmux.SplitID
	axis          termmux.SplitAxis
	dirty         bool
	settlePending bool
	settleAt      time.Time
	cursorSet     bool
	h             *glfw.Cursor
	v             *glfw.Cursor
}

func hitDivider(layout termmux.Layout, x, y, slop int) (termmux.Divider, bool) {
	bestDistance := slop + 1
	bestLength := int(^uint(0) >> 1)
	var best termmux.Divider
	for _, divider := range layout.Dividers {
		r := divider.Pixels
		distance, length, eligible := 0, 0, false
		switch divider.Axis {
		case termmux.SplitColumns:
			if y < r.Y || y >= r.Bottom() {
				continue
			}
			distance = intervalDistance(x, r.X, r.Right())
			length, eligible = r.Height, true
		case termmux.SplitRows:
			if x < r.X || x >= r.Right() {
				continue
			}
			distance = intervalDistance(y, r.Y, r.Bottom())
			length, eligible = r.Width, true
		}
		if !eligible || distance > slop {
			continue
		}
		if distance < bestDistance || distance == bestDistance && (length < bestLength || length == bestLength && divider.Split < best.Split) {
			best, bestDistance, bestLength = divider, distance, length
		}
	}
	return best, bestDistance <= slop
}

func intervalDistance(value, start, end int) int {
	if value < start {
		return start - value
	}
	if value >= end {
		return value - end + 1
	}
	return 0
}

func dividerRatio(divider termmux.Divider, x, y int) (termmux.SplitRatio, bool) {
	start, total, position := divider.Container.X, divider.Container.Width, x
	if divider.Axis == termmux.SplitRows {
		start, total, position = divider.Container.Y, divider.Container.Height, y
	}
	available := total - termmux.DividerPixels
	if available <= 1 {
		return 0, false
	}
	offset := position - start
	if offset < 1 {
		offset = 1
	}
	if offset >= available {
		offset = available - 1
	}
	ratio := termmux.SplitRatio((offset*termmux.RatioScale + available/2) / available)
	if ratio <= 0 {
		ratio = 1
	}
	if ratio >= termmux.RatioScale {
		ratio = termmux.RatioScale - 1
	}
	return ratio, true
}

func (a *App) framebufferPoint(x, y float64) (int, int, bool) {
	if a.window == nil {
		return 0, 0, false
	}
	windowW, windowH := a.window.GetSize()
	fbW, fbH := a.window.GetFramebufferSize()
	if windowW <= 0 || windowH <= 0 || fbW <= 0 || fbH <= 0 {
		return 0, 0, false
	}
	return int(x * float64(fbW) / float64(windowW)), int(y * float64(fbH) / float64(windowH)), true
}

func (a *App) dividerAtWindowPosition(x, y float64) (termmux.Divider, bool) {
	fx, fy, ok := a.framebufferPoint(x, y)
	if !ok {
		return termmux.Divider{}, false
	}
	layout, err := a.mux.Layout()
	if err != nil {
		return termmux.Divider{}, false
	}
	slop := max(1, int(float32(dividerHitSlop)*a.uiScale+0.5))
	return hitDivider(layout, fx, fy, slop)
}

func (a *App) beginDividerDrag(x, y float64) bool {
	divider, ok := a.dividerAtWindowPosition(x, y)
	if !ok {
		return false
	}
	a.divider.active = true
	a.divider.split = divider.Split
	a.divider.axis = divider.Axis
	a.divider.dirty = false
	a.clearPaneHoverForDivider()
	a.setDividerCursor(divider.Axis)
	return true
}

func (a *App) dragDivider(x, y float64) bool {
	if !a.divider.active {
		return false
	}
	fx, fy, ok := a.framebufferPoint(x, y)
	if !ok {
		return true
	}
	layout, err := a.mux.Layout()
	if err != nil {
		return true
	}
	var divider termmux.Divider
	found := false
	for _, candidate := range layout.Dividers {
		if candidate.Split == a.divider.split {
			divider, found = candidate, true
			break
		}
	}
	if !found {
		a.finishDividerDrag()
		return true
	}
	ratio, ok := dividerRatio(divider, fx, fy)
	if !ok {
		return true
	}
	events, err := a.mux.SetSplitRatio(divider.Split, ratio)
	if err != nil {
		if !errors.Is(err, termmux.ErrSplitTooSmall) {
			a.Notify("resize pane: " + err.Error())
		}
		return true
	}
	if len(events) > 0 {
		a.divider.dirty = true
		a.handleMuxEvents(events)
	}
	return true
}

func (a *App) finishDividerDrag() bool {
	if !a.divider.active {
		return false
	}
	if a.divider.dirty {
		a.divider.settlePending = true
		a.divider.settleAt = time.Now()
	}
	a.divider.active = false
	a.divider.split = 0
	a.divider.dirty = false
	a.clearDividerCursor()
	a.applyPendingDividerResize()
	return true
}

func (a *App) applyPendingDividerResize() {
	if !a.divider.settlePending || a.divider.active || time.Now().Before(a.divider.settleAt) {
		return
	}
	if a.resizePTYToGrid() {
		a.divider.settlePending = false
		return
	}
	a.divider.settleAt = time.Now().Add(dividerResizeRetry)
}

func (a *App) updateDividerCursor(x, y float64) bool {
	if a.divider.active {
		a.setDividerCursor(a.divider.axis)
		return true
	}
	divider, ok := a.dividerAtWindowPosition(x, y)
	if !ok {
		a.clearDividerCursor()
		return false
	}
	a.clearPaneHoverForDivider()
	a.setDividerCursor(divider.Axis)
	return true
}

func (a *App) clearPaneHoverForDivider() {
	changed := false
	for _, state := range a.paneUI {
		if state.link.hoverActive {
			state.link.hoverActive = false
			changed = true
		}
	}
	if a.link.hoverActive {
		a.link.hoverActive = false
		changed = true
	}
	if changed {
		a.requestRedraw()
	}
}

func (a *App) setDividerCursor(axis termmux.SplitAxis) {
	if a.window == nil {
		return
	}
	if axis == termmux.SplitColumns {
		if a.divider.h == nil {
			a.divider.h = glfw.CreateStandardCursor(glfw.HResizeCursor)
		}
		a.window.SetCursor(a.divider.h)
		a.divider.cursorSet = true
		return
	}
	if a.divider.v == nil {
		a.divider.v = glfw.CreateStandardCursor(glfw.VResizeCursor)
	}
	a.window.SetCursor(a.divider.v)
	a.divider.cursorSet = true
}

func (a *App) clearDividerCursor() {
	if !a.divider.cursorSet {
		return
	}
	if a.window != nil {
		a.window.SetCursor(nil)
	}
	a.divider.cursorSet = false
}

func (a *App) closeDividerCursors() {
	a.clearDividerCursor()
	if a.divider.h != nil {
		a.divider.h.Destroy()
		a.divider.h = nil
	}
	if a.divider.v != nil {
		a.divider.v.Destroy()
		a.divider.v = nil
	}
}
