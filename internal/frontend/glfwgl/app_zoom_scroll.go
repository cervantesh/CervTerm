//go:build glfw

package glfwgl

import (
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

// zoomBindings holds the parsed zoom hotkeys and the configured font size the
// reset chord restores. Each *OK flag is false when its chord is empty/invalid.
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

// handleZoomKey applies the configured zoom-in/out/reset hotkeys, rebuilding the
// atlas and grid at the new font size. Returns true when the key was a zoom
// chord (so it is consumed and never reaches the PTY). Runs on the main loop
// thread with the GL context current, like the other key handlers.
func (a *App) handleZoomKey(key glfw.Key, mods glfw.ModifierKey) bool {
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		return false
	}
	switch {
	case a.zoom.resetOK && spec == a.zoom.reset:
		a.applyFontSize(a.zoom.base)
	case a.zoom.inOK && spec == a.zoom.in:
		a.applyFontSize(a.cfg.Font.Size + zoomFontStep)
	case a.zoom.outOK && spec == a.zoom.out:
		a.applyFontSize(a.cfg.Font.Size - zoomFontStep)
	default:
		return false
	}
	// Zoom chords are all modified (ctrl), so no Char event follows; suppress
	// defensively in case a binding is remapped to an unmodified key.
	a.suppressNextChar = scriptKeyProducesChar(key, mods)
	return true
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
	if yoff > 0 {
		a.applyFontSize(a.cfg.Font.Size + zoomFontStep)
	} else {
		a.applyFontSize(a.cfg.Font.Size - zoomFontStep)
	}
	return true
}

// applyFontSize clamps pts to the zoom bounds and rebuilds when it changes.
// SetFontSize is a no-op when the size is unchanged, so a zoom at the clamp
// limit does nothing.
func (a *App) applyFontSize(pts float64) {
	if pts < zoomFontMin {
		pts = zoomFontMin
	}
	if pts > zoomFontMax {
		pts = zoomFontMax
	}
	a.SetFontSize(pts)
}

// handleScrollKey scrolls the scrollback viewport for Shift+PageUp/PageDown and
// Shift+Home/End. Plain (unshifted) PageUp/Home/etc. fall through to the normal
// encode path so full-screen apps still receive them. Returns true when the
// chord was a scroll chord (consumed, never sent to the PTY).
func (a *App) handleScrollKey(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&glfw.ModShift == 0 || mods&(glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
		return false
	}
	a.mu.Lock()
	page := a.term.Rows() - 1
	if page < 1 {
		page = 1
	}
	history := a.term.ScrollbackLines()
	var lines int
	switch key {
	case glfw.KeyPageUp:
		lines = page
	case glfw.KeyPageDown:
		lines = -page
	case glfw.KeyHome:
		lines = history // clamps to the oldest row
	case glfw.KeyEnd:
		lines = -history // back to the live prompt
	default:
		a.mu.Unlock()
		return false
	}
	moved := a.term.ScrollViewport(lines)
	a.mu.Unlock()
	if moved {
		a.requestRedraw()
		a.markScrollEvent()
	}
	return true
}
