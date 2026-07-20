//go:build glfw

package glfwgl

import (
	"time"

	termmux "cervterm/internal/mux"
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

func (a *App) dispatchScriptTableKey(key glfw.Key, mods glfw.ModifierKey, repeat bool) bool {
	if a.scriptRT == nil {
		a.keyTable.cancel()
		return false
	}
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		if a.keyTable.mode != keyTableInactive {
			a.keyTable.cancel()
			return true
		}
		return false
	}
	set := a.scriptRT.BindingSet()
	result := a.keyTable.step(set, spec, repeat, time.Now(), uint64(a.focusedPane))
	if result.binding != nil {
		a.dispatchScriptBinding(*result.binding, key, mods, repeat, result.origin)
	}
	if result.consume {
		a.charSuppression.armBinding(scriptKeyProducesChar(key, mods))
	}
	return result.consume
}

func (a *App) dispatchScriptKey(key glfw.Key, mods glfw.ModifierKey, repeat bool) bool {
	if a.scriptRT == nil {
		return false
	}
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		return false
	}
	for _, binding := range a.scriptRT.BindingSet().Root {
		if binding.Spec != spec {
			continue
		}
		if binding.ToTable != "" {
			table, exists := a.scriptRT.BindingSet().Table(binding.ToTable)
			if !exists {
				a.charSuppression.armBinding(scriptKeyProducesChar(key, mods))
				return true
			}
			a.keyTable = keyTableState{mode: keyTableNamed, table: table.Name, deadline: time.Now().Add(time.Duration(table.TimeoutMS) * time.Millisecond), origin: uint64(a.focusedPane)}
			a.charSuppression.armBinding(scriptKeyProducesChar(key, mods))
			return true
		}
		return a.dispatchScriptBinding(binding, key, mods, repeat, uint64(a.focusedPane))
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

// markResizeEvent records a focused-pane resize for deferred Lua dispatch.
func (a *App) markResizeEvent(cols, rows int) {
	if a.focusedPane == 0 {
		return
	}
	if a.pendingPaneResize == nil {
		a.pendingPaneResize = make(map[termmux.PaneID]termmux.PaneGeometry)
	}
	a.pendingPaneResize[a.focusedPane] = termmux.PaneGeometry{Pane: a.focusedPane, Cols: cols, Rows: rows}
}

// markScrollEvent records the post-clamp viewport offset for events.scroll.
// Coalescing per loop iteration (last offset wins) means a burst of wheel ticks
// fires the handler once with the final offset, not once per tick. Called from
// the wheel callback and term:scroll / term:scroll_to_bottom.
func (a *App) markScrollEvent() { a.recordPaneScroll(a.focusedPane) }

func (a *App) recordPaneScroll(id termmux.PaneID) {
	if view, ok := a.mux.PaneView(id); ok {
		a.pendingPaneScroll[id] = view.DisplayOffset
	}
}

// fireLifecycleEvents dispatches the deferred resize/scroll handlers on the loop
// thread, after processTermEvents/resizeToWindow/fireDueTimers, never inside
// draw(). Each pending event fires at most once per iteration with the final
// coalesced value.
func (a *App) fireLifecycleEvents() {
	if a.scriptRT == nil {
		clear(a.pendingPaneResize)
		clear(a.pendingPaneScroll)
		return
	}
	resizeEvents := a.pendingPaneResize
	a.pendingPaneResize = make(map[termmux.PaneID]termmux.PaneGeometry)
	for pane, geometry := range resizeEvents {
		host := paneHost{app: a, pane: pane}
		a.fireScriptEvent(func() error { return a.scriptRT.FireResize(host, geometry.Cols, geometry.Rows) })
	}
	scrollEvents := a.pendingPaneScroll
	a.pendingPaneScroll = make(map[termmux.PaneID]int)
	for pane, offset := range scrollEvents {
		host := paneHost{app: a, pane: pane}
		a.fireScriptEvent(func() error { return a.scriptRT.FireScroll(host, offset) })
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
	a.scriptRT.FireDueTimers(now, a.hostForFocused())
}
