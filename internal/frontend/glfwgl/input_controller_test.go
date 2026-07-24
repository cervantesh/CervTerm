//go:build glfw

package glfwgl

import (
	"reflect"
	"strings"
	"testing"

	terminput "cervterm/internal/input"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type fakeInputRoutes struct {
	log          []string
	stop         string
	reported     bool
	lastRepeat   bool
	terminalKey  terminput.Event
	positionRead int
}

func (f *fakeInputRoutes) step(name string) bool {
	f.log = append(f.log, name)
	return f.stop == name
}

func (f *fakeInputRoutes) routeModalKey(keyRouteEvent) bool {
	return f.step("modal-key")
}

func (f *fakeInputRoutes) routeModalButton(buttonRouteEvent) bool {
	return f.step("modal-button")
}

func (f *fakeInputRoutes) routeModalCursor(cursorRouteEvent) bool {
	return f.step("modal-cursor")
}

func (f *fakeInputRoutes) routeModalWheel(wheelRouteEvent) bool {
	return f.step("modal-wheel")
}

func (f *fakeInputRoutes) inputCursorPosition() cursorRouteEvent {
	f.positionRead++
	f.log = append(f.log, "cursor-position")
	return cursorRouteEvent{x: 12, y: 34}
}

func (f *fakeInputRoutes) clearKeyCharacterSuppression() {
	f.log = append(f.log, "clear-key")
}

func (f *fakeInputRoutes) routeSearchKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("search-key")
}

func (f *fakeInputRoutes) routeReloadKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("reload-key")
}

func (f *fakeInputRoutes) routeScriptTableKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("script-table-key")
}

func (f *fakeInputRoutes) routeScriptKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("script-key")
}

func (f *fakeInputRoutes) routeBuiltinKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("builtin-key")
}

func (f *fakeInputRoutes) routeSelectionCopyKey(_ keyRouteEvent, repeat bool) bool {
	f.lastRepeat = repeat
	return f.step("selection-copy-key")
}

func (f *fakeInputRoutes) routeTerminalKey(event terminput.Event) {
	f.terminalKey = event
	f.log = append(f.log, "terminal-key")
}

func (f *fakeInputRoutes) routeTabBarButton(buttonRouteEvent, cursorRouteEvent) bool {
	return f.step("tab-button")
}

func (f *fakeInputRoutes) routeReportedOrConfiguredButton(buttonRouteEvent, cursorRouteEvent) bool {
	return f.step("reported-configured-button")
}

func (f *fakeInputRoutes) routeActiveDividerButton(buttonRouteEvent, cursorRouteEvent) bool {
	return f.step("active-divider-button")
}

func (f *fakeInputRoutes) routeBeginDividerButton(buttonRouteEvent, cursorRouteEvent) bool {
	return f.step("begin-divider-button")
}

func (f *fakeInputRoutes) routeScrollbarButton(buttonRouteEvent, cursorRouteEvent) bool {
	return f.step("scrollbar-button")
}

func (f *fakeInputRoutes) routeSelectionButton(buttonRouteEvent, cursorRouteEvent) {
	f.log = append(f.log, "selection-button")
}

func (f *fakeInputRoutes) routeTabBarCursor(cursorRouteEvent) bool {
	return f.step("tab-cursor")
}

func (f *fakeInputRoutes) routeCapturedCursor(cursorRouteEvent) bool {
	return f.step("captured-cursor")
}

func (f *fakeInputRoutes) routeConfiguredDrag(cursorRouteEvent) bool {
	return f.step("configured-drag")
}

func (f *fakeInputRoutes) routeDividerDrag(cursorRouteEvent) bool {
	return f.step("divider-drag")
}

func (f *fakeInputRoutes) routeScrollbarCursor(cursorRouteEvent) bool {
	return f.step("scrollbar-cursor")
}

func (f *fakeInputRoutes) routeTerminalMouseMove(cursorRouteEvent) bool {
	f.log = append(f.log, "terminal-move")
	return f.reported
}

func (f *fakeInputRoutes) routeDividerCursor(cursorRouteEvent) bool {
	return f.step("divider-cursor")
}

func (f *fakeInputRoutes) routeSelectionCursor(cursorRouteEvent) {
	f.log = append(f.log, "selection-cursor")
}

func (f *fakeInputRoutes) routeTabBarWheel(cursorRouteEvent) bool {
	return f.step("tab-wheel")
}

func (f *fakeInputRoutes) routeReportedOrConfiguredWheel(wheelRouteEvent, cursorRouteEvent) bool {
	return f.step("reported-configured-wheel")
}

func (f *fakeInputRoutes) routeZoomWheel(wheelRouteEvent) bool {
	return f.step("zoom-wheel")
}

func (f *fakeInputRoutes) routeScrollbarWheel(wheelRouteEvent, cursorRouteEvent) bool {
	return f.step("scrollbar-wheel")
}

func (f *fakeInputRoutes) routeTerminalWheel(wheelRouteEvent) {
	f.log = append(f.log, "terminal-wheel")
}

func (f *fakeInputRoutes) recordInputFocus(bool) {
	f.log = append(f.log, "native-focus")
}

func (f *fakeInputRoutes) cleanupBlurInput() {
	f.log = append(f.log, "blur-cleanup")
}

func (f *fakeInputRoutes) routeScriptFocus(bool) {
	f.log = append(f.log, "script-focus")
}

func (f *fakeInputRoutes) routeTerminalFocus(bool) {
	f.log = append(f.log, "terminal-focus")
}

func newFakeInputController(routes *fakeInputRoutes) *inputController {
	return newInputController(
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
		routes,
	)
}

func assertInputRoute(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("route = %v, want %v", got, want)
	}
}

func TestInputControllerKeyboardOrderAndEveryConsumptionPath(t *testing.T) {
	full := []string{
		"clear-key",
		"modal-key",
		"search-key",
		"reload-key",
		"script-table-key",
		"script-key",
		"builtin-key",
		"selection-copy-key",
		"terminal-key",
	}
	stops := []string{
		"modal-key",
		"search-key",
		"reload-key",
		"script-table-key",
		"script-key",
		"builtin-key",
		"selection-copy-key",
		"",
	}
	for _, stop := range stops {
		name := stop
		if name == "" {
			name = "terminal"
		}
		t.Run(name, func(t *testing.T) {
			routes := &fakeInputRoutes{stop: stop}
			newFakeInputController(routes).handleKey(glfw.KeyC, glfw.Press, glfw.ModControl)
			want := full
			if stop != "" {
				for i, step := range full {
					if step == stop {
						want = full[:i+1]
						break
					}
				}
			}
			assertInputRoute(t, routes.log, want)
		})
	}
}

func TestInputControllerKeyboardReleaseRepeatAndUnencodablePaths(t *testing.T) {
	t.Run("release still offers modal first", func(t *testing.T) {
		routes := &fakeInputRoutes{}
		newFakeInputController(routes).handleKey(glfw.KeyC, glfw.Release, glfw.ModControl)
		assertInputRoute(t, routes.log, []string{"modal-key"})
	})

	t.Run("repeat reaches terminal with repeat semantics", func(t *testing.T) {
		routes := &fakeInputRoutes{}
		newFakeInputController(routes).handleKey(glfw.KeyC, glfw.Repeat, glfw.ModControl)
		if !routes.lastRepeat || routes.terminalKey.Rune != 'c' {
			t.Fatalf("repeat=%v terminal=%#v", routes.lastRepeat, routes.terminalKey)
		}
	})

	t.Run("unencodable key stops after builtin routes", func(t *testing.T) {
		routes := &fakeInputRoutes{}
		newFakeInputController(routes).handleKey(glfw.KeyUnknown, glfw.Press, 0)
		assertInputRoute(t, routes.log, []string{
			"clear-key",
			"modal-key",
			"search-key",
			"reload-key",
			"script-table-key",
			"script-key",
			"builtin-key",
		})
	})
}

func TestInputControllerButtonOrderEveryConsumptionAndModalBeforeCursorLookup(t *testing.T) {
	full := []string{
		"modal-button",
		"cursor-position",
		"tab-button",
		"reported-configured-button",
		"active-divider-button",
		"begin-divider-button",
		"scrollbar-button",
		"selection-button",
	}
	stops := []string{
		"modal-button",
		"tab-button",
		"reported-configured-button",
		"active-divider-button",
		"begin-divider-button",
		"scrollbar-button",
		"",
	}
	for _, stop := range stops {
		name := stop
		if name == "" {
			name = "selection"
		}
		t.Run(name, func(t *testing.T) {
			routes := &fakeInputRoutes{stop: stop}
			newFakeInputController(routes).handleButton(glfw.MouseButtonLeft, glfw.Press, glfw.ModShift)
			want := full
			if stop != "" {
				for i, step := range full {
					if step == stop {
						want = full[:i+1]
						break
					}
				}
			}
			assertInputRoute(t, routes.log, want)
			if stop == "modal-button" && routes.positionRead != 0 {
				t.Fatalf("cursor lookup ran after modal consumed: %d", routes.positionRead)
			}
		})
	}
}

func TestInputControllerCursorOrderAndEveryConsumptionPath(t *testing.T) {
	full := []string{
		"modal-cursor",
		"tab-cursor",
		"captured-cursor",
		"configured-drag",
		"divider-drag",
		"scrollbar-cursor",
		"terminal-move",
		"divider-cursor",
		"selection-cursor",
	}
	stops := []string{
		"modal-cursor",
		"tab-cursor",
		"captured-cursor",
		"configured-drag",
		"divider-drag",
		"scrollbar-cursor",
		"divider-cursor",
		"",
	}
	for _, stop := range stops {
		name := stop
		if name == "" {
			name = "selection"
		}
		t.Run(name, func(t *testing.T) {
			routes := &fakeInputRoutes{stop: stop}
			newFakeInputController(routes).handleCursor(12, 34)
			want := full
			if stop != "" {
				for i, step := range full {
					if step == stop {
						want = full[:i+1]
						break
					}
				}
			}
			assertInputRoute(t, routes.log, want)
		})
	}

	t.Run("terminal report still updates divider cursor before consuming", func(t *testing.T) {
		routes := &fakeInputRoutes{reported: true}
		newFakeInputController(routes).handleCursor(12, 34)
		assertInputRoute(t, routes.log, full[:8])
	})
}

func TestInputControllerWheelOrderEveryConsumptionAndModalBeforeCursorLookup(t *testing.T) {
	full := []string{
		"modal-wheel",
		"cursor-position",
		"tab-wheel",
		"reported-configured-wheel",
		"zoom-wheel",
		"scrollbar-wheel",
		"terminal-wheel",
	}
	stops := []string{
		"modal-wheel",
		"tab-wheel",
		"reported-configured-wheel",
		"zoom-wheel",
		"scrollbar-wheel",
		"",
	}
	for _, stop := range stops {
		name := stop
		if name == "" {
			name = "terminal"
		}
		t.Run(name, func(t *testing.T) {
			routes := &fakeInputRoutes{stop: stop}
			newFakeInputController(routes).handleWheel(2, -3)
			want := full
			if stop != "" {
				for i, step := range full {
					if step == stop {
						want = full[:i+1]
						break
					}
				}
			}
			assertInputRoute(t, routes.log, want)
			if stop == "modal-wheel" && routes.positionRead != 0 {
				t.Fatalf("cursor lookup ran after modal consumed: %d", routes.positionRead)
			}
		})
	}
}

func TestInputControllerFocusPreservesBlurCleanupScriptTerminalOrder(t *testing.T) {
	t.Run("focus", func(t *testing.T) {
		routes := &fakeInputRoutes{}
		newFakeInputController(routes).handleFocus(true)
		assertInputRoute(t, routes.log, []string{"native-focus", "script-focus", "terminal-focus"})
	})

	t.Run("blur", func(t *testing.T) {
		routes := &fakeInputRoutes{}
		newFakeInputController(routes).handleFocus(false)
		assertInputRoute(t, routes.log, []string{"native-focus", "blur-cleanup", "script-focus", "terminal-focus"})
	})
}

func TestControllerPortsStayNarrowAndControllerFieldsAvoidFacadePointers(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*actionExecutionRoute)(nil)).Elem(),
		reflect.TypeOf((*actionProjectionPort)(nil)).Elem(),
		reflect.TypeOf((*actionPanePort)(nil)).Elem(),
		reflect.TypeOf((*actionContextPort)(nil)).Elem(),
		reflect.TypeOf((*actionCommandPort)(nil)).Elem(),
		reflect.TypeOf((*inputModalPort)(nil)).Elem(),
		reflect.TypeOf((*inputCursorPositionPort)(nil)).Elem(),
		reflect.TypeOf((*inputKeyLifecyclePort)(nil)).Elem(),
		reflect.TypeOf((*inputKeyReservedPort)(nil)).Elem(),
		reflect.TypeOf((*inputKeyBindingPort)(nil)).Elem(),
		reflect.TypeOf((*inputKeyTerminalPort)(nil)).Elem(),
		reflect.TypeOf((*inputButtonPriorityPort)(nil)).Elem(),
		reflect.TypeOf((*inputButtonContentPort)(nil)).Elem(),
		reflect.TypeOf((*inputCursorPriorityPort)(nil)).Elem(),
		reflect.TypeOf((*inputCursorContentPort)(nil)).Elem(),
		reflect.TypeOf((*inputWheelPort)(nil)).Elem(),
		reflect.TypeOf((*inputFocusPort)(nil)).Elem(),
	}
	for _, port := range ports {
		if port.NumMethod() > 5 {
			t.Errorf("%s has %d methods", port.Name(), port.NumMethod())
		}
	}

	controllers := []reflect.Type{
		reflect.TypeOf(actionController{}),
		reflect.TypeOf(inputController{}),
	}
	for _, controller := range controllers {
		for i := 0; i < controller.NumField(); i++ {
			field := controller.Field(i)
			typeName := field.Type.String()
			if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Func || strings.Contains(typeName, "*glfwgl.App") || strings.Contains(typeName, "*mux.Mux") || strings.Contains(typeName, "*glfw.Window") || strings.Contains(typeName, "*script.Runtime") {
				t.Errorf("%s.%s has forbidden field type %s", controller.Name(), field.Name, typeName)
			}
		}
	}
}
