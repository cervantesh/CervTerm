//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
	"time"

	termmux "cervterm/internal/mux"
)

type fakeScriptLifecyclePorts struct {
	log     []string
	runtime bool
	wants   bool
	fail    string
	err     error
	resizes []scriptResizeEvent
	scrolls []scriptScrollEvent
	now     time.Time
}

func (f *fakeScriptLifecyclePorts) step(name string) error {
	f.log = append(f.log, name)
	if f.fail == name {
		return f.err
	}
	return nil
}

func (f *fakeScriptLifecyclePorts) scriptLifecycleRuntimeAvailable() bool {
	f.log = append(f.log, "runtime")
	return f.runtime
}

func (f *fakeScriptLifecyclePorts) scriptLifecycleWantsOutput() bool {
	f.log = append(f.log, "wants-output")
	return f.wants
}

func (f *fakeScriptLifecyclePorts) fireScriptOutput(termmux.PaneID, string) error {
	return f.step("output")
}

func (f *fakeScriptLifecyclePorts) fireScriptTitle(termmux.PaneID, string) error {
	return f.step("title")
}

func (f *fakeScriptLifecyclePorts) fireScriptCWD(termmux.PaneID, string) error {
	return f.step("cwd")
}

func (f *fakeScriptLifecyclePorts) fireScriptBell(termmux.PaneID) error {
	return f.step("bell")
}

func (f *fakeScriptLifecyclePorts) fireScriptFocus(termmux.PaneID, bool) error {
	return f.step("focus")
}

func (f *fakeScriptLifecyclePorts) fireScriptResize(termmux.PaneID, int, int) error {
	return f.step("resize")
}

func (f *fakeScriptLifecyclePorts) fireScriptScroll(termmux.PaneID, int) error {
	return f.step("scroll")
}

func (f *fakeScriptLifecyclePorts) reportScriptLifecycleError(err error) {
	if !errors.Is(err, f.err) {
		panic("unexpected script lifecycle error")
	}
	f.log = append(f.log, "report")
}

func (f *fakeScriptLifecyclePorts) clearPendingScriptLifecycle() {
	f.log = append(f.log, "clear")
	f.resizes = f.resizes[:0]
	f.scrolls = f.scrolls[:0]
}

func (f *fakeScriptLifecyclePorts) takePendingScriptResize() (scriptResizeEvent, bool) {
	if len(f.resizes) == 0 {
		return scriptResizeEvent{}, false
	}
	event := f.resizes[0]
	f.resizes = f.resizes[1:]
	f.log = append(f.log, "take-resize")
	return event, true
}

func (f *fakeScriptLifecyclePorts) takePendingScriptScroll() (scriptScrollEvent, bool) {
	if len(f.scrolls) == 0 {
		return scriptScrollEvent{}, false
	}
	event := f.scrolls[0]
	f.scrolls = f.scrolls[1:]
	f.log = append(f.log, "take-scroll")
	return event, true
}

func (f *fakeScriptLifecyclePorts) fireDueScriptTimers(now time.Time) {
	f.log = append(f.log, "timers")
	f.now = now
}

func (f *fakeScriptLifecyclePorts) syncScriptStatus() {
	f.log = append(f.log, "status")
}

func (f *fakeScriptLifecyclePorts) syncScriptOverlays() {
	f.log = append(f.log, "overlay")
}

func newFakeScriptLifecycleController(ports *fakeScriptLifecyclePorts) *scriptLifecycleController {
	return newScriptLifecycleController(ports, ports, ports, ports, ports, ports, ports)
}

func assertScriptLifecycleTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("script lifecycle trace = %v, want %v", got, want)
	}
}

func TestScriptLifecycleControllerPinsPaneAndFocusEventRoutes(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{runtime: true, wants: true}
	controller := newFakeScriptLifecycleController(ports)
	controller.output(1, "data")
	controller.title(1, "title")
	controller.cwd(1, "/cwd")
	controller.bell(1)
	controller.focus(1, true)
	assertScriptLifecycleTrace(t, ports.log, []string{
		"runtime", "wants-output", "output",
		"runtime", "title", "runtime", "cwd", "runtime", "bell", "runtime", "focus",
	})
}

func TestScriptLifecycleControllerFiltersOutputAndReportsExactEventFailure(t *testing.T) {
	injected := errors.New("script")
	tests := []struct {
		name string
		data string
		up   bool
		want bool
		fail string
		log  []string
	}{
		{name: "empty", data: "", up: true, want: true, log: nil},
		{name: "no runtime", data: "data", log: []string{"runtime"}},
		{name: "unwanted", data: "data", up: true, log: []string{"runtime", "wants-output"}},
		{name: "failure", data: "data", up: true, want: true, fail: "output", log: []string{"runtime", "wants-output", "output", "report"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := &fakeScriptLifecyclePorts{runtime: test.up, wants: test.want, fail: test.fail, err: injected}
			newFakeScriptLifecycleController(ports).output(2, test.data)
			assertScriptLifecycleTrace(t, ports.log, test.log)
		})
	}
}

func TestScriptLifecycleControllerNoRuntimeClearsPendingWithoutDispatch(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{
		resizes: []scriptResizeEvent{{pane: 1, cols: 80, rows: 24}},
		scrolls: []scriptScrollEvent{{pane: 1, offset: 4}},
	}
	newFakeScriptLifecycleController(ports).dispatchPending()
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "clear"})
	if len(ports.resizes) != 0 || len(ports.scrolls) != 0 {
		t.Fatalf("pending survived clear: resize=%v scroll=%v", ports.resizes, ports.scrolls)
	}
}

func TestScriptLifecycleControllerDrainsResizeBeforeScrollAndReportsWithoutStopping(t *testing.T) {
	injected := errors.New("resize")
	ports := &fakeScriptLifecyclePorts{
		runtime: true, fail: "resize", err: injected,
		resizes: []scriptResizeEvent{{pane: 1, cols: 80, rows: 24}, {pane: 2, cols: 40, rows: 12}},
		scrolls: []scriptScrollEvent{{pane: 1, offset: 7}},
	}
	newFakeScriptLifecycleController(ports).dispatchPending()
	assertScriptLifecycleTrace(t, ports.log, []string{
		"runtime", "take-resize", "resize", "report", "take-resize", "resize", "report", "take-scroll", "scroll",
	})
	if len(ports.resizes) != 0 || len(ports.scrolls) != 0 {
		t.Fatal("drained pending records survived")
	}
}

func TestScriptLifecycleControllerPinsTimersThenStatusBeforeOverlayRoutes(t *testing.T) {
	now := time.Unix(5, 0)
	ports := &fakeScriptLifecyclePorts{runtime: true}
	controller := newFakeScriptLifecycleController(ports)
	controller.fireDueTimers(now)
	controller.syncProjections()
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "timers", "runtime", "status", "overlay"})
	if !ports.now.Equal(now) {
		t.Fatalf("timer time = %v, want %v", ports.now, now)
	}
}

func TestScriptLifecycleControllerNoRuntimeNoOpRoutesDoNotAllocate(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{log: make([]string, 0, 8), resizes: make([]scriptResizeEvent, 0, 1), scrolls: make([]scriptScrollEvent, 0, 1)}
	controller := newFakeScriptLifecycleController(ports)
	now := time.Unix(1, 0)
	allocs := testing.AllocsPerRun(1000, func() {
		ports.log = ports.log[:0]
		ports.resizes = ports.resizes[:0]
		ports.scrolls = ports.scrolls[:0]
		controller.output(1, "")
		controller.title(1, "ignored")
		controller.dispatchPending()
		controller.fireDueTimers(now)
		controller.syncProjections()
	})
	if allocs != 0 {
		t.Fatalf("allocations per no-runtime routes = %v, want 0", allocs)
	}
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "runtime", "clear", "runtime", "runtime"})
}

func TestScriptLifecycleControllerPortsAndFieldsAreExhaustiveNarrowAndDetached(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*scriptLifecycleRuntimePort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleEventPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleDeferredEventPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleFailurePort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecyclePendingPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleTimerPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleProjectionPort)(nil)).Elem(),
	}
	fields := []controllerFieldExpectation{
		{name: "runtime", typ: ports[0]},
		{name: "events", typ: ports[1]},
		{name: "deferred", typ: ports[2]},
		{name: "failures", typ: ports[3]},
		{name: "pending", typ: ports[4]},
		{name: "timers", typ: ports[5]},
		{name: "projections", typ: ports[6]},
	}
	assertControllerPortStructure(t, reflect.TypeOf(scriptLifecycleController{}), fields, ports, scriptLifecycleControllerPortBudget)
}
