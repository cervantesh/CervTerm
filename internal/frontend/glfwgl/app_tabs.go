//go:build glfw

package glfwgl

import (
	termux "cervterm/internal/mux"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func (a *App) refreshTabBarLayout(w, h int) tabBarLayout {
	if a.mux == nil || !tabBarVisible(a.cfg.TabBar.Mode, len(a.mux.Tabs())) {
		a.tabBar = tabBarLayout{}
		return a.tabBar
	}
	bounds := a.windowGeometry(w, h).TabBar
	scale := max(float32(1), a.uiScale)
	a.tabBar = layoutTabBar(bounds, a.mux.Tabs(), a.mux.ActiveTab(), int(float32(a.cfg.TabBar.MinWidthPX)*scale), int(float32(a.cfg.TabBar.MaxWidthPX)*scale), int(float32(a.cfg.TabBar.PaddingX)*scale), max(1, int(a.cellW)), a.cfg.TabBar.ShowCloseButton, a.cfg.TabBar.ShowNewButton, a.tabBarFirst)
	a.tabBarFirst = a.tabBar.First
	return a.tabBar
}
func (a *App) drawTabBar(w, h int) {
	layout := a.refreshTabBarLayout(w, h)
	if layout.Bounds.Height == 0 {
		return
	}
	cmds := []drawCmd{{kind: cmdRect, x: float32(layout.Bounds.X), y: float32(layout.Bounds.Y), w: float32(layout.Bounds.Width), h: float32(layout.Bounds.Height), col: a.chrome.background}}
	for _, item := range layout.Items {
		col := a.chrome.background
		if item.Active {
			col = a.chrome.accent
		}
		cmds = append(cmds, drawCmd{kind: cmdRect, x: float32(item.Bounds.X), y: float32(item.Bounds.Y), w: float32(item.Bounds.Width), h: float32(item.Bounds.Height), col: col}, drawCmd{kind: cmdText, x: float32(item.Label.X), y: float32(item.Label.Y) + (float32(item.Label.Height)-a.cellH)/2, text: item.Text, col: a.chrome.muted})
		if item.Close.Width > 0 {
			cmds = append(cmds, drawCmd{kind: cmdText, x: float32(item.Close.X) + 4*a.uiScale, y: float32(item.Close.Y) + (float32(item.Close.Height)-a.cellH)/2, text: "×", col: a.chrome.muted})
		}
	}
	if layout.Add.Width > 0 {
		cmds = append(cmds, drawCmd{kind: cmdText, x: float32(layout.Add.X) + 6*a.uiScale, y: float32(layout.Add.Y) + (float32(layout.Add.Height)-a.cellH)/2, text: "+", col: a.chrome.accent})
	}
	a.paint(cmds)
}

func (a *App) handleTabBarButton(button glfw.MouseButton, action glfw.Action, x, y float64) bool {
	if button != glfw.MouseButtonLeft || a.window == nil {
		return false
	}
	w, h := a.window.GetFramebufferSize()
	a.refreshTabBarLayout(w, h)
	fx, fy := a.windowToFramebuffer(x, y)
	hit, ok := a.tabBar.Hit(int(fx), int(fy))
	if action == glfw.Press {
		if ok {
			a.tabBarPressed = hit
			return true
		}
		return false
	}
	if action != glfw.Release || a.tabBarPressed.Kind == tabHitNone {
		return false
	}
	pressed := a.tabBarPressed
	a.tabBarPressed = tabHit{}
	if !ok || hit != pressed {
		return true
	}
	var events []termux.Event
	var err error
	switch hit.Kind {
	case tabHitBody:
		events, err = a.mux.ActivateTab(hit.Tab)
	case tabHitClose:
		events, err = a.mux.CloseTab(hit.Tab)
	case tabHitAdd:
		_, _, events, err = a.mux.SpawnTab(a.desiredShellSpawnSpec(), termux.CellMetrics{CellWidth: max(1, int(a.cellW)), CellHeight: max(1, int(a.cellH))}, "")
	}
	if err != nil {
		a.Notify(err.Error())
		return true
	}
	a.handleMuxEvents(events)
	return true
}

func (a *App) pointerOverTabBar(x, y float64) bool {
	if a.window == nil {
		return false
	}
	w, h := a.window.GetFramebufferSize()
	a.refreshTabBarLayout(w, h)
	fx, fy := a.windowToFramebuffer(x, y)
	return insideClip(a.tabBar.Bounds, int(fx), int(fy)) && a.tabBar.Bounds.Height > 0
}
