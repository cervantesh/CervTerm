//go:build glfw

package glfwgl

import "time"

// presentationGate limits completed presentations without discarding redraw
// demand. A zero maxFPS disables the explicit cap and leaves vsync authoritative.
type presentationGate struct {
	last time.Time
}

func presentationInterval(maxFPS int) time.Duration {
	if maxFPS <= 0 {
		return 0
	}
	return time.Second / time.Duration(maxFPS)
}

func (g presentationGate) wait(now time.Time, maxFPS int) time.Duration {
	interval := presentationInterval(maxFPS)
	if interval == 0 || g.last.IsZero() {
		return 0
	}
	remaining := g.last.Add(interval).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (g presentationGate) ready(now time.Time, maxFPS int) bool {
	return g.wait(now, maxFPS) == 0
}

func (g *presentationGate) record(now time.Time) {
	g.last = now
}
