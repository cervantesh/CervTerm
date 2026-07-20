//go:build glfw

package glfwgl

import (
	"cervterm/internal/ime"
	termmux "cervterm/internal/mux"
)

func (c *windowController) focus(id termmux.WindowID) error {
	if err := c.requireLoop(); err != nil {
		return err
	}
	projection, ok := c.windows[id]
	if !ok || projection.closed {
		return errWindowProjectionMissing
	}
	if c.active != 0 && c.active != id {
		if active := c.windows[c.active]; active != nil && active.app != nil {
			_ = active.app.cancelComposition(ime.CancelFocusLost)
		}
	}
	projection.host.Focus()
	c.active = id
	return nil
}
