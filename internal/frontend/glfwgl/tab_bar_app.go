//go:build glfw

package glfwgl

func (a *App) effectiveTabBarHeight() int {
	count := 1
	if a.mux != nil {
		count = len(a.mux.Tabs())
	}
	if !tabBarVisible(a.cfg.TabBar.Mode, count) {
		return 0
	}
	return max(1, int(float32(a.cfg.TabBar.HeightPX)*max(float32(1), a.uiScale)))
}
