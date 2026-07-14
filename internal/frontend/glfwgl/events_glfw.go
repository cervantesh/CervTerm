//go:build glfw

package glfwgl

import (
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// fireScriptEvent runs a terminal-event handler when a runtime is present,
// surfacing any script error as a transient notice. Called on the main thread.
func (a *App) fireScriptEvent(fire func() error) {
	if a.scriptRT == nil {
		return
	}
	if err := fire(); err != nil {
		a.Notify("script error: " + err.Error())
	}
}

func (a *App) dispatchScriptKey(key glfw.Key, mods glfw.ModifierKey, dispatch bool) bool {
	if a.scriptRT == nil {
		return false
	}
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		return false
	}
	for i, binding := range a.scriptRT.Bindings() {
		if binding == spec {
			if dispatch {
				if err := a.scriptRT.Dispatch(i, a); err != nil {
					a.Notify("script error: " + err.Error())
				}
			}
			a.suppressNextChar = scriptKeyProducesChar(key, mods)
			return true
		}
	}
	return false
}

func scriptKeyProducesChar(key glfw.Key, mods glfw.ModifierKey) bool {
	if mods&(glfw.ModControl|glfw.ModAlt|glfw.ModSuper) != 0 {
		return false
	}
	if key >= glfw.KeyA && key <= glfw.KeyZ {
		return true
	}
	if key >= glfw.Key0 && key <= glfw.Key9 {
		return true
	}
	switch key {
	case glfw.KeySpace, glfw.KeyMinus, glfw.KeyEqual, glfw.KeyComma, glfw.KeyPeriod,
		glfw.KeySlash, glfw.KeyBackslash, glfw.KeySemicolon, glfw.KeyApostrophe,
		glfw.KeyGraveAccent:
		return true
	default:
		return false
	}
}

// markResizeEvent records the latest grid size for events.resize. Called from
// resizeToWindow and the initial spawn; the loop drains it in
// fireLifecycleEvents so the handler never runs re-entrantly (a resize can be
// triggered by term:set_font_size from inside another handler).
func (a *App) markResizeEvent(cols, rows int) {
	a.resizeEventCols, a.resizeEventRows = cols, rows
	a.resizeEventPending = true
}

// markScrollEvent records the post-clamp viewport offset for events.scroll.
// Coalescing per loop iteration (last offset wins) means a burst of wheel ticks
// fires the handler once with the final offset, not once per tick. Called from
// the wheel callback and term:scroll / term:scroll_to_bottom.
func (a *App) markScrollEvent() {
	a.mu.Lock()
	a.scrollEventOffset = a.term.DisplayOffset()
	a.mu.Unlock()
	a.scrollEventPending = true
}

// fireLifecycleEvents dispatches the deferred resize/scroll handlers on the loop
// thread, after processTermEvents/resizeToWindow/fireDueTimers, never inside
// draw(). Each pending event fires at most once per iteration with the final
// coalesced value.
func (a *App) fireLifecycleEvents() {
	if a.scriptRT == nil {
		a.resizeEventPending, a.scrollEventPending = false, false
		return
	}
	if a.resizeEventPending {
		a.resizeEventPending = false
		cols, rows := a.resizeEventCols, a.resizeEventRows
		a.fireScriptEvent(func() error { return a.scriptRT.FireResize(a, cols, rows) })
	}
	if a.scrollEventPending {
		a.scrollEventPending = false
		offset := a.scrollEventOffset
		a.fireScriptEvent(func() error { return a.scriptRT.FireScroll(a, offset) })
	}
}

// fireDueTimers runs any script timers whose deadline has passed. No timers (or
// no runtime) is zero cost: NextTimerDeadline returns false and FireDueTimers
// returns immediately. Handlers run under the shared watchdog on the loop
// thread; a timer they schedule is seen by the next nextWakeTimeout because both
// mutate the table on this same thread (no cross-thread wake needed).
func (a *App) fireDueTimers(now time.Time) {
	if a.scriptRT == nil {
		return
	}
	a.scriptRT.FireDueTimers(now, a)
}
