//go:build glfw

package glfwgl

import "time"

// renderControllerPortBudget pins the complete temporary delegation surface for
// the additive seam. The controller owns ordering only; App remains the
// authoritative state and resource owner until the later movement/wiring work.
const renderControllerPortBudget = 8

type renderTickPort interface {
	tickRenderProjection()
}

type renderClockPort interface {
	renderNow() time.Time
}

type renderPresentationPort interface {
	renderReady(time.Time) bool
	throttleRender(time.Time)
}

type renderFramePort interface {
	beginRenderFrame()
	drawRenderFrameBody()
	finishRenderFrame()
	endRenderFrame()
}

// renderController captures the existing per-projection presentation order
// without owning damage, redraw demand, frame accounting, or native resources.
// TODO(L1-06; expires Slice 6.1a): replace App scratch swapping with pane render contexts.
// TODO(L1-01; expires Slice 6.3d): remove the preparatory facade adapter.
type renderController struct {
	ticks        renderTickPort
	clock        renderClockPort
	presentation renderPresentationPort
	frames       renderFramePort
}

func newRenderController(ticks renderTickPort, clock renderClockPort, presentation renderPresentationPort, frames renderFramePort) *renderController {
	return &renderController{ticks: ticks, clock: clock, presentation: presentation, frames: frames}
}

// renderProjection reports only whether a frame reached EndFrame. External
// damage clearing, presentation recording, metering, and redraw acknowledgement
// deliberately remain outside this preparatory controller.
func (c *renderController) renderProjection(continuous bool) bool {
	c.ticks.tickRenderProjection()
	now := c.clock.renderNow()
	if continuous {
		c.presentation.throttleRender(now)
	} else if !c.presentation.renderReady(now) {
		return false
	}
	c.drawFrame()
	c.frames.endRenderFrame()
	return true
}

func (c *renderController) drawFrame() {
	c.frames.beginRenderFrame()
	defer c.frames.finishRenderFrame()
	c.frames.drawRenderFrameBody()
}
