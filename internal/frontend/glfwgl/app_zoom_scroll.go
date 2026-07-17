//go:build glfw

package glfwgl

import (
	termaction "cervterm/internal/action"

	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Font-size bounds for the built-in zoom hotkeys, matching the term:set_font_size
// Lua clamp so both entry points agree.
const (
	zoomFontMin  = 6.0
	zoomFontMax  = 72.0
	zoomFontStep = 1.0
)

// zoomBindings holds parsed zoom hotkeys and the configured base size restored
// by the reset chord. Pending zoom state is pane-local in paneFontState.
type zoomBindings struct {
	in, out, reset       script.Spec
	inOK, outOK, resetOK bool
	base                 float64
}

// initZoomHotkeys parses the configured zoom chords and records the base font
// size the reset chord restores. Empty/invalid chords leave that binding off.
func (a *App) initZoomHotkeys() {
	a.zoom.base = a.cfg.Font.Size
	if spec, ok := parseStatsHotkey(a.cfg.Render.ZoomInHotkey); ok {
		a.zoom.in, a.zoom.inOK = spec, true
	}
	if spec, ok := parseStatsHotkey(a.cfg.Render.ZoomOutHotkey); ok {
		a.zoom.out, a.zoom.outOK = spec, true
	}
	if spec, ok := parseStatsHotkey(a.cfg.Render.ZoomResetHotkey); ok {
		a.zoom.reset, a.zoom.resetOK = spec, true
	}
}

// handleZoomWheel zooms on Ctrl+wheel (up = in, down = out), the standard
// terminal gesture. Returns true when Ctrl was held so the caller skips the
// normal scroll/mouse-report path. GLFW omits modifiers from the scroll
// callback, so the live Ctrl key state is queried.
func (a *App) handleZoomWheel(yoff float64) bool {
	if a.window == nil || yoff == 0 {
		return false
	}
	if a.window.GetKey(glfw.KeyLeftControl) != glfw.Press && a.window.GetKey(glfw.KeyRightControl) != glfw.Press {
		return false
	}
	delta := -zoomFontStep
	if yoff > 0 {
		delta = zoomFontStep
	}
	envelope := actionEnvelope(termaction.Zoom{Mode: termaction.ZoomDelta, Amount: delta})
	if err := a.executeAction(envelope, a.actionContext(termaction.SourceMouse)); err != nil {
		a.notifyActionError(err)
	}
	return true
}
