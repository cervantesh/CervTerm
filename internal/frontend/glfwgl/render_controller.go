//go:build glfw

package glfwgl

import "time"

// renderControllerPortBudget pins the complete temporary delegation surface for
// the additive seam. The controller owns ordering only; App remains the
// authoritative state and resource owner until the later movement/wiring work.
const renderControllerPortBudget = 7

type renderTickPort interface {
	tickRenderProjection()
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
type renderController struct {
	ticks        renderTickPort
	presentation renderPresentationPort
	frames       renderFramePort
}

func newRenderController(ticks renderTickPort, presentation renderPresentationPort, frames renderFramePort) *renderController {
	return &renderController{ticks: ticks, presentation: presentation, frames: frames}
}

// renderProjection reports only whether a frame reached EndFrame. External
// damage clearing, presentation recording, metering, and redraw acknowledgement
// deliberately remain outside this preparatory controller.
func (c *renderController) renderProjection(now time.Time, continuous bool) bool {
	c.ticks.tickRenderProjection()
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
