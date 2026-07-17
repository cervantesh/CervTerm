//go:build glfw

package glfwgl

import (
	"errors"

	termaction "cervterm/internal/action"
	"cervterm/internal/input"
	"cervterm/internal/script"

	"github.com/go-gl/glfw/v3.3/glfw"
)

const allScriptMods = script.ModCtrl | script.ModAlt | script.ModShift | script.ModSuper

type keyActionMatcher struct {
	key       string
	required  script.Mod
	forbidden script.Mod
}

func (m keyActionMatcher) matches(spec script.Spec) bool {
	return spec.Key == m.key && spec.Mods&m.required == m.required && spec.Mods&m.forbidden == 0
}

type keyActionBinding struct {
	matcher  keyActionMatcher
	envelope termaction.Envelope
}

func exactActionMatcher(spec script.Spec) keyActionMatcher {
	return keyActionMatcher{key: spec.Key, required: spec.Mods, forbidden: allScriptMods &^ spec.Mods}
}

func requiredActionMatcher(key string, required, forbidden script.Mod) keyActionMatcher {
	return keyActionMatcher{key: key, required: required, forbidden: forbidden}
}

func actionEnvelope(command termaction.Action) termaction.Envelope {
	return termaction.Envelope{Action: command, Target: termaction.TargetFocused}
}

func (a *App) initActionBindings() {
	bindings := make([]keyActionBinding, 0, 20)
	addExact := func(spec script.Spec, command termaction.Action) {
		bindings = append(bindings, keyActionBinding{matcher: exactActionMatcher(spec), envelope: actionEnvelope(command)})
	}
	addRequired := func(key string, required, forbidden script.Mod, command termaction.Action) {
		bindings = append(bindings, keyActionBinding{
			matcher:  requiredActionMatcher(key, required, forbidden),
			envelope: actionEnvelope(command),
		})
	}

	if a.statsSpecOK {
		addExact(a.statsSpec, termaction.ToggleStats{})
	}
	if a.zoom.inOK {
		addExact(a.zoom.in, termaction.Zoom{Mode: termaction.ZoomDelta, Amount: zoomFontStep})
	}
	if a.zoom.outOK {
		addExact(a.zoom.out, termaction.Zoom{Mode: termaction.ZoomDelta, Amount: -zoomFontStep})
	}
	if a.zoom.resetOK {
		addExact(a.zoom.reset, termaction.Zoom{Mode: termaction.ZoomReset})
	}

	addRequired("pageup", script.ModShift, script.ModCtrl|script.ModAlt|script.ModSuper, termaction.Scroll{Unit: termaction.ScrollPage, Amount: 1})
	addRequired("pagedown", script.ModShift, script.ModCtrl|script.ModAlt|script.ModSuper, termaction.Scroll{Unit: termaction.ScrollPage, Amount: -1})
	addRequired("home", script.ModShift, script.ModCtrl|script.ModAlt|script.ModSuper, termaction.Scroll{Unit: termaction.ScrollBuffer, Amount: 1})
	addRequired("end", script.ModShift, script.ModCtrl|script.ModAlt|script.ModSuper, termaction.Scroll{Unit: termaction.ScrollBuffer, Amount: -1})

	addRequired("equal", script.ModAlt|script.ModShift, 0, termaction.SplitPane{Axis: termaction.SplitColumns})
	addRequired("minus", script.ModAlt|script.ModShift, 0, termaction.SplitPane{Axis: termaction.SplitRows})
	addRequired("left", script.ModAlt, 0, termaction.FocusPane{Direction: termaction.FocusLeft})
	addRequired("right", script.ModAlt, 0, termaction.FocusPane{Direction: termaction.FocusRight})
	addRequired("up", script.ModAlt, 0, termaction.FocusPane{Direction: termaction.FocusUp})
	addRequired("down", script.ModAlt, 0, termaction.FocusPane{Direction: termaction.FocusDown})
	addRequired("w", script.ModCtrl|script.ModShift, 0, termaction.ClosePane{})

	addRequired("v", script.ModCtrl, 0, termaction.PasteClipboard{})
	addRequired("insert", script.ModShift, 0, termaction.PasteClipboard{})
	addRequired("insert", script.ModCtrl, script.ModShift, termaction.CopySelection{})
	addRequired("c", script.ModCtrl|script.ModShift, 0, termaction.CopySelection{})

	a.actionBindings = bindings
}

func (a *App) dispatchBuiltinAction(key glfw.Key, mods glfw.ModifierKey, repeat bool) bool {
	if len(a.actionBindings) == 0 {
		a.initActionBindings()
	}
	spec, ok := specFromGLFW(key, mods)
	if !ok {
		return false
	}
	for _, binding := range a.actionBindings {
		if binding.matcher.matches(spec) && a.dispatchKeyAction(binding.envelope, key, mods, repeat) {
			return true
		}
	}
	return false
}

func (a *App) dispatchReservedAction(command termaction.Action, key glfw.Key, mods glfw.ModifierKey, repeat bool) bool {
	return a.dispatchKeyAction(actionEnvelope(command), key, mods, repeat)
}

func (a *App) dispatchKeyAction(envelope termaction.Envelope, key glfw.Key, mods glfw.ModifierKey, repeat bool) bool {
	descriptor, err := termaction.DefaultRegistry().Describe(envelope.Action)
	if err != nil {
		a.Notify(err.Error())
		return true
	}
	consume, execute := descriptor.TriggerPolicy.ConsumePress, descriptor.TriggerPolicy.ExecutePress
	if repeat {
		consume, execute = descriptor.TriggerPolicy.ConsumeRepeat, descriptor.TriggerPolicy.ExecuteRepeat
	}
	if execute {
		if err := a.executeAction(envelope, a.actionContext(termaction.SourceKeyboard)); err != nil {
			a.notifyActionError(err)
		}
	}
	if consume {
		a.suppressNextChar = scriptKeyProducesChar(key, mods)
	}
	return consume
}

func (a *App) notifyActionError(err error) {
	var actionErr *termaction.ExecutionError
	if !errors.As(err, &actionErr) {
		a.Notify(err.Error())
		return
	}
	message := actionErr.Err
	if message == nil {
		message = err
	}
	switch actionErr.Class {
	case termaction.ErrorScript:
		a.Notify("script error: " + message.Error())
	case termaction.ErrorMux:
		a.Notify("mux: " + message.Error())
	case termaction.ErrorInput:
		a.Notify("input: " + message.Error())
	case termaction.ErrorTarget:
		a.Notify("action target: " + message.Error())
	default:
		a.Notify(message.Error())
	}
}

func searchActivationChord(key glfw.Key, mods glfw.ModifierKey) bool {
	return key == glfw.KeyF && mods&(glfw.ModControl|glfw.ModShift) == (glfw.ModControl|glfw.ModShift)
}

func reloadChord(key glfw.Key, mods glfw.ModifierKey) bool {
	return key == glfw.KeyR && mods&(glfw.ModControl|glfw.ModShift) == (glfw.ModControl|glfw.ModShift) && mods&(glfw.ModAlt|glfw.ModSuper) == 0
}

func (a *App) handleKeyEvent(key glfw.Key, eventAction glfw.Action, mods glfw.ModifierKey) {
	if eventAction != glfw.Press && eventAction != glfw.Repeat {
		return
	}
	repeat := eventAction == glfw.Repeat
	// Active modal capture remains first. The inactive search activation chord
	// is a reserved typed action and therefore still precedes user bindings.
	if a.search.active {
		if a.search.handleKey(key, mods) {
			return
		}
	} else if searchActivationChord(key, mods) {
		if a.dispatchReservedAction(termaction.ToggleSearch{}, key, mods, repeat) {
			return
		}
	}
	if reloadChord(key, mods) {
		if a.dispatchReservedAction(termaction.ReloadConfig{}, key, mods, repeat) {
			return
		}
	}
	if a.dispatchScriptKey(key, mods, repeat) {
		return
	}
	if a.dispatchBuiltinAction(key, mods, repeat) {
		return
	}
	event, hasEvent := inputEventFromGLFW(key, mods)
	// Plain Ctrl+C copies only when a selection exists; otherwise it must still
	// reach the PTY as an interrupt byte.
	if key == glfw.KeyC && mods&glfw.ModControl != 0 && a.Selection() != "" {
		if a.dispatchReservedAction(termaction.CopySelection{}, key, mods, repeat) {
			return
		}
	}
	if !hasEvent {
		return
	}
	if encoded, ok := input.EncodeWithMode(event, a.inputMode()); ok {
		a.writeInputBytes(encoded)
	}
}
