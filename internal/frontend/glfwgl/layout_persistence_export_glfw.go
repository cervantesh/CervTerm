//go:build glfw

package glfwgl

import (
	"fmt"

	"cervterm/internal/layoutstate"
	termmux "cervterm/internal/mux"
)

func (c *windowController) currentLayoutPlan() (layoutstate.Plan, error) {
	if c == nil || c.services.mux == nil {
		return layoutstate.Plan{}, errWindowProjectionMissing
	}
	fresh, err := c.services.mux.FreshSessionSnapshot()
	if err != nil {
		return layoutstate.Plan{}, err
	}
	views := c.services.mux.Workspaces()
	if len(views) != len(fresh.Workspaces) {
		return layoutstate.Plan{}, fmt.Errorf("layout persistence workspace projection mismatch")
	}
	document := layoutstate.Document{Version: layoutstate.Version1, ActiveWorkspace: fresh.ActiveWorkspace, Workspaces: make([]layoutstate.Workspace, len(fresh.Workspaces))}
	for wi, workspace := range fresh.Workspaces {
		if len(views[wi].Windows) != len(workspace.Windows) {
			return layoutstate.Plan{}, fmt.Errorf("layout persistence window projection mismatch")
		}
		out := layoutstate.Workspace{Name: workspace.Name, ActiveWindow: workspace.ActiveWindow, Windows: make([]layoutstate.Window, len(workspace.Windows))}
		for windowIndex, window := range workspace.Windows {
			id := views[wi].Windows[windowIndex]
			projection := c.windows[id]
			if projection == nil || projection.closed || projection.host == nil || projection.app == nil {
				return layoutstate.Plan{}, fmt.Errorf("layout persistence window %d has no native projection", id)
			}
			x, y := projection.host.GetPos()
			width, height := projection.host.GetSize()
			cfg := projection.app.cfg
			backgroundOpacity, textOpacity, blur, fontSize := cfg.Window.BackgroundOpacity, cfg.Window.TextOpacity, cfg.Window.Blur, cfg.Font.Size
			outWindow := layoutstate.Window{Title: window.Title, Bounds: layoutstate.Bounds{X: x, Y: y, Width: width, Height: height}, ActiveTab: window.ActiveTab, Appearance: layoutstate.Appearance{ColorScheme: cfg.ColorScheme, BackgroundOpacity: &backgroundOpacity, TextOpacity: &textOpacity, Blur: &blur, FontSize: &fontSize}, Tabs: make([]layoutstate.Tab, len(window.Tabs))}
			for tabIndex, tab := range window.Tabs {
				outWindow.Tabs[tabIndex] = layoutstate.Tab{Title: tab.Title, FocusedLeaf: tab.FocusedLeaf, Root: freshLayoutNode(tab.Root)}
			}
			out.Windows[windowIndex] = outWindow
		}
		document.Workspaces[wi] = out
	}
	return layoutstate.NewPlan(document)
}

func freshLayoutNode(source termmux.FreshNode) layoutstate.Node {
	if source.Type == "pane" {
		launch := layoutstate.Launch{}
		if source.Launch != nil {
			launch = layoutstate.Launch{TargetID: source.Launch.TargetID, Program: source.Launch.Program, Args: append([]string(nil), source.Launch.Args...), CWD: source.Launch.CWD}
		}
		return layoutstate.Node{Type: "pane", Launch: &launch}
	}
	first, second := freshLayoutNode(*source.First), freshLayoutNode(*source.Second)
	axis := "columns"
	if source.Axis == termmux.SplitRows {
		axis = "rows"
	}
	return layoutstate.Node{Type: "split", Axis: axis, Ratio: int(source.Ratio), First: &first, Second: &second}
}
