//go:build glfw

package glfwgl

import termmux "cervterm/internal/mux"

const maxPendingWindowEvents = 256

func (c *windowController) queuePending(id termmux.WindowID, event termmux.Event) {
	if event.Kind == termmux.PaneNotificationRequested {
		event.Fresh = false
	}
	pending := c.pending[id]
	if len(pending) == maxPendingWindowEvents {
		pending = pending[1:]
	}
	c.pending[id] = append(pending, event)
}
