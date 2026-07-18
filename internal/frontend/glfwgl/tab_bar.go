package glfwgl

import (
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/mux"
	"cervterm/internal/unicodecluster"
	"fmt"
	"strings"
)

type tabHitKind uint8

const (
	tabHitNone tabHitKind = iota
	tabHitBody
	tabHitClose
	tabHitAdd
)

type tabHit struct {
	Kind tabHitKind
	Tab  mux.TabID
}
type tabBarItem struct {
	Tab                  mux.TabID
	Bounds, Label, Close gpu.ClipRect
	Text                 string
	Active               bool
	Revision             uint64
}
type tabBarLayout struct {
	Bounds gpu.ClipRect
	Items  []tabBarItem
	Add    gpu.ClipRect
	First  int
}

func tabBarVisible(mode string, count int) bool {
	return mode == "always" || (mode == "multiple" && count > 1)
}
func layoutTabBar(bounds gpu.ClipRect, tabs []mux.TabView, active mux.TabID, minWidth, maxWidth, padding, cellWidth int, showClose, showNew bool, previousFirst int) tabBarLayout {
	out := tabBarLayout{Bounds: bounds}
	if bounds.Width <= 0 || bounds.Height <= 0 || len(tabs) == 0 {
		return out
	}
	control := bounds.Height
	if !showNew {
		control = 0
	}
	available := max(0, bounds.Width-control)
	if available == 0 {
		return out
	}
	minWidth = max(1, minWidth)
	maxWidth = max(minWidth, maxWidth)
	capacity := max(1, available/minWidth)
	capacity = min(capacity, len(tabs))
	first := min(max(0, previousFirst), len(tabs)-capacity)
	activeIndex := 0
	for i := range tabs {
		if tabs[i].ID == active {
			activeIndex = i
			break
		}
	}
	if activeIndex < first {
		first = activeIndex
	}
	if activeIndex >= first+capacity {
		first = activeIndex - capacity + 1
	}
	first = min(max(0, first), len(tabs)-capacity)
	out.First = first
	width := min(maxWidth, available/capacity)
	if width < minWidth {
		width = max(1, available/capacity)
	}
	x := bounds.X
	for i := first; i < first+capacity; i++ {
		right := min(bounds.X+available, x+width)
		item := tabBarItem{Tab: tabs[i].ID, Bounds: gpu.ClipRect{X: x, Y: bounds.Y, Width: max(0, right-x), Height: bounds.Height}, Active: tabs[i].ID == active, Revision: tabs[i].Revision}
		closeWidth := 0
		if showClose {
			closeWidth = min(bounds.Height, item.Bounds.Width/3)
			item.Close = gpu.ClipRect{X: right - closeWidth, Y: bounds.Y, Width: closeWidth, Height: bounds.Height}
		}
		labelX := x + padding
		labelWidth := max(0, item.Bounds.Width-closeWidth-2*padding)
		item.Label = gpu.ClipRect{X: labelX, Y: bounds.Y, Width: labelWidth, Height: bounds.Height}
		title := tabs[i].Title
		if title == "" {
			title = fmt.Sprintf("Tab %d", tabs[i].ID)
		}
		item.Text = clipTabTitle(title, max(0, labelWidth/max(1, cellWidth)))
		out.Items = append(out.Items, item)
		x = right
	}
	if showNew {
		out.Add = gpu.ClipRect{X: bounds.X + bounds.Width - control, Y: bounds.Y, Width: control, Height: bounds.Height}
	}
	return out
}
func (l tabBarLayout) Hit(x, y int) (tabHit, bool) {
	if !insideClip(l.Bounds, x, y) {
		return tabHit{}, false
	}
	if insideClip(l.Add, x, y) && l.Add.Width > 0 {
		return tabHit{Kind: tabHitAdd}, true
	}
	for _, item := range l.Items {
		if insideClip(item.Close, x, y) && item.Close.Width > 0 {
			return tabHit{Kind: tabHitClose, Tab: item.Tab}, true
		}
		if insideClip(item.Bounds, x, y) {
			return tabHit{Kind: tabHitBody, Tab: item.Tab}, true
		}
	}
	return tabHit{}, false
}
func insideClip(r gpu.ClipRect, x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.Width && y < r.Y+r.Height
}
func clipTabTitle(text string, cells int) string {
	if cells <= 0 {
		return ""
	}
	clusters := unicodecluster.Segment(text)
	used := 0
	var b strings.Builder
	for i, c := range clusters {
		w := max(0, c.Width)
		if used+w > cells {
			if cells-used >= 1 && i < len(clusters) {
				b.WriteRune('…')
			}
			break
		}
		b.WriteString(c.Text)
		used += w
	}
	return b.String()
}
