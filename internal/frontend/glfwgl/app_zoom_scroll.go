//go:build glfw

package glfwgl

import (
	"time"

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
	pending              float64 // latest target size; the visual is rebuilt toward this every step
	pendingSet           bool
	ptyDirty             bool      // grid changed since ConPTY was last told; resize it at burst settle
	deadline             time.Time // resize the PTY once now passes this (debounce)
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
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	case a.zoom.outOK && spec == a.zoom.out:
		a.applyFontSize(a.zoomTarget() - zoomFontStep)
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
		a.applyFontSize(a.zoomTarget() + zoomFontStep)
	} else {
		a.applyFontSize(a.zoomTarget() - zoomFontStep)
	}
	return true
}

// zoomTarget is the size the next relative zoom step builds on: the pending
// (not-yet-applied) target when a burst is in flight, otherwise the live size.
// This lets several wheel ticks in one frame compound instead of collapsing to
// a single step.
func (a *App) zoomTarget() float64 {
	if a.zoom.pendingSet {
		return a.zoom.pending
	}
	return a.cfg.Font.Size
}

// zoomDebounce is how long the zoom target must be stable before ConPTY is
// resized to the settled grid. The atlas/grid VISUAL rebuild is not debounced —
// it runs every step so the zoom animates frame by frame. Only the PTY resize is
// coalesced: ConPTY repaints its viewport asynchronously on every resize, and
// several in flight at once interleave over the next grid → duplicated/garbled
// scrollback. Debouncing the PTY resize means one resize and one repaint per
// burst while the on-screen font still tracks the wheel.
const zoomDebounce = 70 * time.Millisecond

// applyFontSize clamps pts to the zoom bounds and records it as the latest
// target, pushing the PTY-resize deadline out so a continuing burst keeps
// coalescing that one resize. applyPendingZoom does the per-step visual rebuild
// and the settled PTY resize on the loop thread.
func (a *App) applyFontSize(pts float64) {
	if pts < zoomFontMin {
		pts = zoomFontMin
	}
	if pts > zoomFontMax {
		pts = zoomFontMax
	}
	a.zoom.pending, a.zoom.pendingSet = pts, true
	a.zoom.deadline = time.Now().Add(zoomDebounce)
	a.requestRedraw()
}

// applyPendingZoom drives zoom from the loop on the main thread with the GL
// context current. Every pass it rebuilds the atlas + local grid toward the
// latest target (no PTY resize), so a burst animates frame by frame instead of
// freezing until the user stops. Once the target has been stable for
// zoomDebounce it resizes ConPTY once to the settled grid — the async-repaint
// interleaving that garbles scrollback needs several PTY resizes in flight at
// once, which one settled resize never produces.
func (a *App) applyPendingZoom() {
	if !a.zoom.pendingSet {
		return
	}
	if a.cfg.Font.Size != a.zoom.pending {
		a.cfg.Font.Size = a.zoom.pending
		if a.rebuildAtlasGridVisual(a.contentScaleX, a.contentScaleY) {
			a.zoom.ptyDirty = true
		}
	}
	if time.Now().Before(a.zoom.deadline) {
		return
	}
	a.zoom.pendingSet = false
	if a.zoom.ptyDirty {
		a.zoom.ptyDirty = false
		a.resizePTYToGrid()
	}
}

// handleScrollKey scrolls the scrollback viewport for Shift+PageUp/PageDown and
// Shift+Home/End. Plain (unshifted) PageUp/Home/etc. fall through to the normal
// encode path so full-screen apps still receive them. Returns true when the
// chord was a scroll chord (consumed, never sent to the PTY).
func (a *App) handleScrollKey(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&glfw.ModShift == 0 || mods&(glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
		return false
	}
	_, view, ok := a.focusedView()
	if !ok {
		return true
	}
	page := view.Snapshot.Rows - 1
	if page < 1 {
		page = 1
	}
	history := view.ScrollbackLines
	var lines int
	switch key {
	case glfw.KeyPageUp:
		lines = page
	case glfw.KeyPageDown:
		lines = -page
	case glfw.KeyHome:
		lines = history
	case glfw.KeyEnd:
		lines = -history
	default:
		return false
	}
	moved, _ := a.mux.ScrollViewport(a.focusedPane, lines)
	if moved {
		a.requestRedraw()
		a.markScrollEvent()
	}
	return true
}
