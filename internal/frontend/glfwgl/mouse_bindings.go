//go:build glfw

package glfwgl

import (
	"time"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const maxMouseClickCount = 3

type pointerEvent struct {
	Event      script.MouseEvent
	Button     script.MouseButton
	Mods       script.Mod
	ClickCount int
	X, Y       float64
}

type mouseBindingCapture struct {
	active     bool
	button     script.MouseButton
	mods       script.Mod
	clickCount int
	origin     termmux.PaneID
}

type mouseClickState struct {
	button script.MouseButton
	at     time.Time
	count  int
}

func normalizeMouseButton(button glfw.MouseButton) (script.MouseButton, bool) {
	switch button {
	case glfw.MouseButtonLeft:
		return script.MouseLeft, true
	case glfw.MouseButtonMiddle:
		return script.MouseMiddle, true
	case glfw.MouseButtonRight:
		return script.MouseRight, true
	default:
		return "", false
	}
}

func normalizePointerButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey, x, y float64, clicks int) (pointerEvent, bool) {
	mapped, ok := normalizeMouseButton(button)
	if !ok || (action != glfw.Press && action != glfw.Release) {
		return pointerEvent{}, false
	}
	event := script.MousePress
	if action == glfw.Release {
		event = script.MouseRelease
	}
	return pointerEvent{Event: event, Button: mapped, Mods: scriptModsFromGLFW(mods), ClickCount: boundClickCount(clicks), X: x, Y: y}, true
}

func normalizePointerWheel(yoff float64, mods glfw.ModifierKey, x, y float64) (pointerEvent, bool) {
	if yoff == 0 {
		return pointerEvent{}, false
	}
	button := script.MouseDown
	if yoff > 0 {
		button = script.MouseUp
	}
	return pointerEvent{Event: script.MouseWheel, Button: button, Mods: scriptModsFromGLFW(mods), ClickCount: 1, X: x, Y: y}, true
}

func boundClickCount(count int) int {
	if count < 1 {
		return 1
	}
	if count > maxMouseClickCount {
		return maxMouseClickCount
	}
	return count
}

func scriptModsFromGLFW(mods glfw.ModifierKey) script.Mod {
	var out script.Mod
	if mods&glfw.ModControl != 0 {
		out |= script.ModCtrl
	}
	if mods&glfw.ModAlt != 0 {
		out |= script.ModAlt
	}
	if mods&glfw.ModShift != 0 {
		out |= script.ModShift
	}
	if mods&glfw.ModSuper != 0 {
		out |= script.ModSuper
	}
	return out
}

func matchMouseBinding(bindings []script.MouseBinding, event pointerEvent) *script.MouseBinding {
	spec := script.MouseSpec{Event: event.Event, Button: event.Button, Mods: event.Mods, ClickCount: boundClickCount(event.ClickCount)}
	for i := range bindings {
		if bindings[i].Spec == spec {
			binding := bindings[i]
			return &binding
		}
	}
	return nil
}

func (a *App) nextClickCount(button script.MouseButton, now time.Time) int {
	if a.mouseClicks.button == button && now.Sub(a.mouseClicks.at) <= 500*time.Millisecond {
		a.mouseClicks.count = boundClickCount(a.mouseClicks.count + 1)
	} else {
		a.mouseClicks.count = 1
	}
	a.mouseClicks.button, a.mouseClicks.at = button, now
	return a.mouseClicks.count
}

func (a *App) dispatchMouseBinding(binding script.MouseBinding, origin termmux.PaneID) {
	if binding.Callback != nil {
		if a.scriptRT == nil {
			a.Notify("script error: runtime is unavailable")
		} else if err := a.scriptRT.DispatchRef(*binding.Callback, "mouse_bindings", paneHost{app: a, pane: origin}); err != nil {
			a.Notify("script error: " + err.Error())
		}
		return
	}
	context := a.actionContext(termaction.SourceMouse)
	context.Origin = termaction.Ref{Kind: termaction.RefPane, ID: uint64(origin)}
	if err := a.executeAction(binding.Action, context); err != nil {
		a.notifyActionError(err)
	}
}

func (a *App) dispatchUserPointer(event pointerEvent, origin termmux.PaneID) bool {
	if a.scriptRT == nil {
		return false
	}
	binding := matchMouseBinding(a.scriptRT.BindingSet().Mouse, event)
	if binding == nil {
		return false
	}
	a.dispatchMouseBinding(*binding, origin)
	return true
}

func (a *App) handleConfiguredMouseButton(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey, x, y float64) bool {
	if a.sendMouseButton(button, action, mods) {
		return true
	}
	event, ok := normalizePointerButton(button, action, mods, x, y, 1)
	if !ok {
		return false
	}
	if action == glfw.Press {
		event.ClickCount = a.nextClickCount(event.Button, time.Now())
		pane, _, found := a.paneAtWindowPosition(x, y)
		if !found {
			return false
		}
		a.focusPane(pane)
		if !a.dispatchUserPointer(event, pane) {
			return false
		}
		a.mouseBindingCapture = mouseBindingCapture{active: true, button: event.Button, mods: event.Mods, clickCount: event.ClickCount, origin: pane}
		return true
	}
	if a.mouseBindingCapture.active && event.Button == a.mouseBindingCapture.button {
		capture := a.mouseBindingCapture
		a.mouseBindingCapture = mouseBindingCapture{}
		event.Mods, event.ClickCount = capture.mods, capture.clickCount
		a.dispatchUserPointer(event, capture.origin)
		return true
	}
	if pane, _, found := a.paneAtWindowPosition(x, y); found {
		return a.dispatchUserPointer(event, pane)
	}
	return false
}

func (a *App) handleConfiguredMouseDrag(x, y float64) bool {
	if !a.mouseBindingCapture.active {
		return false
	}
	capture := a.mouseBindingCapture
	event := pointerEvent{Event: script.MouseDrag, Button: capture.button, Mods: capture.mods, ClickCount: capture.clickCount, X: x, Y: y}
	a.dispatchUserPointer(event, capture.origin)
	return true
}

func (a *App) handleConfiguredMouseWheel(yoff float64, x, y float64) bool {
	mods := a.currentModifiers()
	if a.sendMouseWheel(yoff, mods) {
		return true
	}
	event, ok := normalizePointerWheel(yoff, mods, x, y)
	if !ok {
		return false
	}
	pane, _, found := a.paneAtWindowPosition(x, y)
	if !found {
		return false
	}
	a.focusPane(pane)
	return a.dispatchUserPointer(event, pane)
}
