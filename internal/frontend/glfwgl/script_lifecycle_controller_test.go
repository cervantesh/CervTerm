//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"
	"time"

	termmux "cervterm/internal/mux"
)

type scriptResizeEvent struct {
	pane       termmux.PaneID
	cols, rows int
}

type scriptScrollEvent struct {
	pane   termmux.PaneID
	offset int
}

type fakeScriptLifecyclePorts struct {
	log     []string
	runtime bool
	wants   bool
	fail    string
	err     error
	resizes []scriptResizeEvent
	scrolls []scriptScrollEvent
	now     time.Time
	onStep  func(string)
}

func (f *fakeScriptLifecyclePorts) step(name string) error {
	f.log = append(f.log, name)
	if f.onStep != nil {
		f.onStep(name)
	}
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

func (f *fakeScriptLifecyclePorts) dispatchPendingScriptResizes() {
	f.log = append(f.log, "dispatch-resize")
	events := f.resizes
	f.resizes = nil
	for range events {
		reportScriptLifecycleError(f, f.step("resize"))
	}
}

func (f *fakeScriptLifecyclePorts) dispatchPendingScriptScrolls() {
	f.log = append(f.log, "dispatch-scroll")
	events := f.scrolls
	f.scrolls = nil
	for range events {
		reportScriptLifecycleError(f, f.step("scroll"))
	}
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

func newFakeScriptLifecycleController() *scriptLifecycleController {
	controller := newScriptLifecycleController()
	return &controller
}

func assertScriptLifecycleTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("script lifecycle trace = %v, want %v", got, want)
	}
}

func TestScriptLifecycleControllerPinsPaneAndFocusEventRoutes(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{runtime: true, wants: true}
	controller := newFakeScriptLifecycleController()
	controller.output(ports, ports, ports, 1, "data")
	controller.title(ports, ports, ports, 1, "title")
	controller.cwd(ports, ports, ports, 1, "/cwd")
	controller.bell(ports, ports, ports, 1)
	controller.focus(ports, ports, ports, 1, true)
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
			newFakeScriptLifecycleController().output(ports, ports, ports, 2, test.data)
			assertScriptLifecycleTrace(t, ports.log, test.log)
		})
	}
}

func TestScriptLifecycleControllerNoRuntimeClearsPendingWithoutDispatch(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{
		resizes: []scriptResizeEvent{{pane: 1, cols: 80, rows: 24}},
		scrolls: []scriptScrollEvent{{pane: 1, offset: 4}},
	}
	newFakeScriptLifecycleController().dispatchPending(ports, ports)
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
	newFakeScriptLifecycleController().dispatchPending(ports, ports)
	assertScriptLifecycleTrace(t, ports.log, []string{
		"runtime", "dispatch-resize", "resize", "report", "resize", "report", "dispatch-scroll", "scroll",
	})
	if len(ports.resizes) != 0 || len(ports.scrolls) != 0 {
		t.Fatal("drained pending records survived")
	}
}

func TestScriptLifecycleControllerPreservesArrivalsDuringDispatch(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{
		runtime: true,
		resizes: []scriptResizeEvent{{pane: 1, cols: 80, rows: 24}},
		scrolls: []scriptScrollEvent{{pane: 1, offset: 4}},
	}
	ports.onStep = func(name string) {
		if name != "resize" {
			return
		}
		ports.onStep = nil
		ports.resizes = append(ports.resizes, scriptResizeEvent{pane: 2, cols: 40, rows: 12})
		ports.scrolls = append(ports.scrolls, scriptScrollEvent{pane: 2, offset: 8})
	}
	controller := newFakeScriptLifecycleController()
	controller.dispatchPending(ports, ports)
	assertScriptLifecycleTrace(t, ports.log, []string{
		"runtime", "dispatch-resize", "resize", "dispatch-scroll", "scroll", "scroll",
	})
	if len(ports.resizes) != 1 || len(ports.scrolls) != 0 {
		t.Fatalf("arrivals after first dispatch: resizes=%v scrolls=%v", ports.resizes, ports.scrolls)
	}
	ports.log = nil
	controller.dispatchPending(ports, ports)
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "dispatch-resize", "resize", "dispatch-scroll"})
	if len(ports.resizes) != 0 || len(ports.scrolls) != 0 {
		t.Fatalf("arrivals survived second dispatch: resizes=%v scrolls=%v", ports.resizes, ports.scrolls)
	}
}

func TestScriptLifecycleControllerPinsTimersThenStatusBeforeOverlayRoutes(t *testing.T) {
	now := time.Unix(5, 0)
	ports := &fakeScriptLifecyclePorts{runtime: true}
	controller := newFakeScriptLifecycleController()
	controller.fireDueTimers(ports, ports, now)
	controller.syncProjections(ports, ports)
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "timers", "runtime", "status", "overlay"})
	if !ports.now.Equal(now) {
		t.Fatalf("timer time = %v, want %v", ports.now, now)
	}
}

func TestScriptLifecycleControllerNoRuntimeNoOpRoutesDoNotAllocate(t *testing.T) {
	ports := &fakeScriptLifecyclePorts{log: make([]string, 0, 8), resizes: make([]scriptResizeEvent, 0, 1), scrolls: make([]scriptScrollEvent, 0, 1)}
	controller := newFakeScriptLifecycleController()
	now := time.Unix(1, 0)
	payload := []byte("ignored")
	allocs := testing.AllocsPerRun(1000, func() {
		ports.log = ports.log[:0]
		ports.resizes = ports.resizes[:0]
		ports.scrolls = ports.scrolls[:0]
		controller.output(ports, ports, ports, 1, "")
		controller.outputBytes(ports, ports, ports, 1, payload)
		controller.title(ports, ports, ports, 1, "ignored")
		controller.dispatchPending(ports, ports)
		controller.fireDueTimers(ports, ports, now)
		controller.syncProjections(ports, ports)
	})
	if allocs != 0 {
		t.Fatalf("allocations per no-runtime routes = %v, want 0", allocs)
	}
	assertScriptLifecycleTrace(t, ports.log, []string{"runtime", "runtime", "runtime", "clear", "runtime", "runtime"})
}

func TestAppScriptLifecycleControllersAreProjectionLocalEagerLazyAndIdempotent(t *testing.T) {
	owner, _ := newRecordingActionApp(t)
	owner.initScriptLifecycleController()
	root := owner.ensureScriptLifecycleController()
	if !owner.scriptLifecycleReady || owner.ensureScriptLifecycleController() != root {
		t.Fatal("root script lifecycle controller was not eager and idempotent")
	}
	child := newProjectionApp(owner)
	childController := child.ensureScriptLifecycleController()
	if !child.scriptLifecycleReady || childController == root {
		t.Fatal("child did not receive a distinct eager script lifecycle controller")
	}

	zero := &App{}
	if zero.scriptLifecycleReady {
		t.Fatal("zero App unexpectedly initialized its script lifecycle controller")
	}
	lazy := zero.ensureScriptLifecycleController()
	if !zero.scriptLifecycleReady || zero.ensureScriptLifecycleController() != lazy {
		t.Fatal("zero App script lifecycle controller was not lazy and idempotent")
	}
}

func TestScriptLifecycleControllerEphemeralPortsPreserveOrderAndFaultReporting(t *testing.T) {
	injected := errors.New("title")
	ports := &fakeScriptLifecyclePorts{
		runtime: true, wants: true, fail: "title", err: injected,
		resizes: []scriptResizeEvent{{pane: 1, cols: 80, rows: 24}},
		scrolls: []scriptScrollEvent{{pane: 1, offset: 4}},
	}
	controller := newFakeScriptLifecycleController()

	controller.output(ports, ports, ports, 1, "output")
	controller.title(ports, ports, ports, 1, "title")
	controller.cwd(ports, ports, ports, 1, "/cwd")
	controller.bell(ports, ports, ports, 1)
	controller.focus(ports, ports, ports, 1, true)
	controller.dispatchPending(ports, ports)
	now := time.Unix(9, 0)
	controller.fireDueTimers(ports, ports, now)
	controller.syncProjections(ports, ports)

	assertScriptLifecycleTrace(t, ports.log, []string{
		"runtime", "wants-output", "output",
		"runtime", "title", "report",
		"runtime", "cwd",
		"runtime", "bell",
		"runtime", "focus",
		"runtime", "dispatch-resize", "resize", "dispatch-scroll", "scroll",
		"runtime", "timers",
		"runtime", "status", "overlay",
	})
	if !ports.now.Equal(now) {
		t.Fatalf("timer time = %v, want %v", ports.now, now)
	}
}

func TestAppScriptLifecycleWarmedNoRuntimeRoutesDoNotAllocate(t *testing.T) {
	app := &App{}
	controller := app.ensureScriptLifecycleController()
	now := time.Unix(1, 0)
	allocs := testing.AllocsPerRun(1000, func() {
		app.fireLifecycleEvents()
		app.fireDueTimers(now)
		app.ensureScriptLifecycleController().syncProjections(app, app)
	})
	if &app.scriptLifecycle != controller || allocs != 0 {
		t.Fatalf("controller=%p want=%p allocations=%v", &app.scriptLifecycle, controller, allocs)
	}
}

func TestScriptLifecycleControllerPortsAndFieldsAreExhaustiveNarrowAndDetached(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*scriptLifecycleRuntimePort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleEventPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleFailurePort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecyclePendingPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleTimerPort)(nil)).Elem(),
		reflect.TypeOf((*scriptLifecycleProjectionPort)(nil)).Elem(),
	}
	fields := []controllerFieldExpectation{}
	appType := reflect.TypeOf(App{})
	controllerField, ok := appType.FieldByName("scriptLifecycle")
	if !ok || controllerField.Type != reflect.TypeOf(scriptLifecycleController{}) {
		t.Fatalf("App.scriptLifecycle = %v, want retained controller value", controllerField.Type)
	}
	assertControllerPortStructure(t, reflect.TypeOf(scriptLifecycleController{}), fields, ports, scriptLifecycleControllerPortBudget)
}
