//go:build glfw

package glfwgl

import (
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func (c *windowController) applyWorkspaceProjection(events []termmux.Event) {
	relevant := false
	focusRequested := false
	for _, event := range events {
		switch event.Kind {
		case termmux.WorkspaceActivated, termmux.WindowActivated, termmux.WindowWorkspaceChanged, termmux.WindowCreated, termmux.WindowClosed:
			relevant = true
		}
		if event.Kind == termmux.WorkspaceActivated || event.Kind == termmux.WindowActivated {
			focusRequested = true
		}
	}
	if !relevant || c.services.mux == nil {
		return
	}
	workspace := c.services.mux.ActiveWorkspace()
	visible := make(map[termmux.WindowID]struct{}, len(workspace.Windows))
	for _, id := range workspace.Windows {
		visible[id] = struct{}{}
	}
	for _, id := range c.order {
		projection := c.windows[id]
		if projection == nil || projection.closed {
			continue
		}
		if _, ok := visible[id]; ok {
			projection.visible = true
			projection.host.Show()
		} else {
			if projection.visible && projection.app != nil {
				_ = projection.app.cancelComposition(ime.CancelWindowHidden)
			}
			projection.visible = false
			projection.host.Hide()
		}
	}
	c.active = workspace.Focused
	if focusRequested {
		if projection := c.windows[workspace.Focused]; projection != nil && !projection.closed {
			projection.host.Focus()
		}
	}
}

func (c *windowController) projectionVisible(id termmux.WindowID) bool {
	projection := c.windows[id]
	return projection != nil && !projection.closed && projection.visible
}
