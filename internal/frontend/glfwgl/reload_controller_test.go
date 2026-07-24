//go:build glfw

package glfwgl

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeReloadPorts struct {
	log        []string
	source     bool
	watch      bool
	pending    bool
	workers    int
	drainCount int
	drainTo    int
	starts     int
	reports    int
	pollAt     time.Time
}

func (f *fakeReloadPorts) reloadSourceActive() bool {
	f.log = append(f.log, "source")
	return f.source
}

func (f *fakeReloadPorts) pollReloadWatch(now time.Time) bool {
	f.log = append(f.log, "poll")
	f.pollAt = now
	return f.watch
}

func (f *fakeReloadPorts) markReloadPending() {
	f.log = append(f.log, "mark")
	f.pending = true
}

func (f *fakeReloadPorts) reloadIsPending() bool {
	f.log = append(f.log, "pending")
	return f.pending
}

func (f *fakeReloadPorts) consumeReloadPending() {
	f.log = append(f.log, "consume")
	f.pending = false
}

func (f *fakeReloadPorts) drainReloadResults() {
	f.log = append(f.log, "drain")
	f.drainCount++
	if f.drainTo >= 0 {
		f.workers = f.drainTo
	}
}

func (f *fakeReloadPorts) reloadWorkerCount() int {
	f.log = append(f.log, "workers")
	return f.workers
}

func (f *fakeReloadPorts) startReloadWorker() {
	f.log = append(f.log, "start")
	f.starts++
	f.workers++
}

func (f *fakeReloadPorts) reportMissingReloadSource() {
	f.log = append(f.log, "report")
	f.reports++
}

func newFakeReloadController(ports *fakeReloadPorts) *reloadController {
	return newReloadController(ports, ports, ports, ports, ports)
}

func assertReloadTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reload trace = %v, want %v", got, want)
	}
}

func TestReloadControllerRequestRequiresActiveSourceAndMarksAppPending(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		ports := &fakeReloadPorts{drainTo: -1}
		controller := newFakeReloadController(ports)
		if controller.requestReload() || ports.pending {
			t.Fatal("missing source queued a reload")
		}
		assertReloadTrace(t, ports.log, []string{"source"})
	})

	t.Run("active source", func(t *testing.T) {
		ports := &fakeReloadPorts{source: true, drainTo: -1}
		controller := newFakeReloadController(ports)
		if !controller.requestReload() || !ports.pending || !controller.requestReload() || !ports.pending {
			t.Fatal("active source did not leave App-owned reload state pending")
		}
		assertReloadTrace(t, ports.log, []string{"source", "mark", "source", "mark"})
	})
}

func TestReloadControllerPollMarksOnlyStableWatchChangePending(t *testing.T) {
	ports := &fakeReloadPorts{drainTo: -1}
	controller := newFakeReloadController(ports)
	controller.pollReload(time.Unix(1, 0))
	if ports.pending {
		t.Fatal("unchanged watch queued a reload")
	}
	ports.watch = true
	now := time.Unix(2, 0)
	controller.pollReload(now)
	if !ports.pending {
		t.Fatal("watch change did not queue a reload")
	}
	if !ports.pollAt.Equal(now) {
		t.Fatalf("poll time = %v, want caller-provided %v", ports.pollAt, now)
	}
	assertReloadTrace(t, ports.log, []string{"poll", "poll", "mark"})
}

func TestReloadControllerApplyDrainsBeforeEveryShortCircuit(t *testing.T) {
	t.Run("inactive", func(t *testing.T) {
		ports := &fakeReloadPorts{drainTo: -1}
		controller := newFakeReloadController(ports)
		controller.applyReload()
		assertReloadTrace(t, ports.log, []string{"drain", "pending"})
	})

	t.Run("worker cap retains App pending state", func(t *testing.T) {
		ports := &fakeReloadPorts{pending: true, workers: reloadControllerWorkerCap, drainTo: -1}
		controller := newFakeReloadController(ports)
		controller.applyReload()
		if !ports.pending || ports.starts != 0 || ports.reports != 0 {
			t.Fatalf("capped apply: pending=%v starts=%d reports=%d", ports.pending, ports.starts, ports.reports)
		}
		assertReloadTrace(t, ports.log, []string{"drain", "pending", "workers"})
	})

	t.Run("drain can lower cap before dispatch", func(t *testing.T) {
		ports := &fakeReloadPorts{source: true, pending: true, workers: reloadControllerWorkerCap, drainTo: 1}
		controller := newFakeReloadController(ports)
		controller.applyReload()
		if ports.pending || ports.starts != 1 || ports.workers != reloadControllerWorkerCap {
			t.Fatalf("post-drain dispatch: pending=%v starts=%d workers=%d", ports.pending, ports.starts, ports.workers)
		}
		assertReloadTrace(t, ports.log, []string{"drain", "pending", "workers", "consume", "source", "start"})
	})
}

func TestReloadControllerApplyConsumesBeforeMissingSourceReportOrStart(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		ports := &fakeReloadPorts{pending: true, drainTo: -1}
		controller := newFakeReloadController(ports)
		controller.applyReload()
		if ports.pending || ports.reports != 1 || ports.starts != 0 {
			t.Fatalf("missing-source apply: pending=%v reports=%d starts=%d", ports.pending, ports.reports, ports.starts)
		}
		assertReloadTrace(t, ports.log, []string{"drain", "pending", "workers", "consume", "source", "report"})
	})

	t.Run("active source", func(t *testing.T) {
		ports := &fakeReloadPorts{source: true, pending: true, drainTo: -1}
		controller := newFakeReloadController(ports)
		controller.applyReload()
		if ports.pending || ports.starts != 1 || ports.reports != 0 {
			t.Fatalf("active-source apply: pending=%v starts=%d reports=%d", ports.pending, ports.starts, ports.reports)
		}
		assertReloadTrace(t, ports.log, []string{"drain", "pending", "workers", "consume", "source", "start"})
	})
}

func TestReloadControllerApplyPathsDoNotAllocate(t *testing.T) {
	tests := []struct {
		name     string
		ports    *fakeReloadPorts
		logLimit int
	}{
		{name: "inactive", ports: &fakeReloadPorts{drainTo: -1}, logLimit: 2},
		{name: "worker cap", ports: &fakeReloadPorts{pending: true, workers: reloadControllerWorkerCap, drainTo: -1}, logLimit: 3},
		{name: "dispatch", ports: &fakeReloadPorts{source: true, pending: true, drainTo: -1}, logLimit: 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.ports.log = make([]string, 0, test.logLimit)
			controller := newFakeReloadController(test.ports)
			allocs := testing.AllocsPerRun(1000, func() {
				test.ports.log = test.ports.log[:0]
				if test.name != "inactive" {
					test.ports.pending = true
				}
				if test.name == "dispatch" {
					test.ports.workers = 0
				}
				controller.applyReload()
			})
			if allocs != 0 {
				t.Fatalf("allocations per %s reload apply = %v, want 0", test.name, allocs)
			}
		})
	}
}

func TestAppReloadEntryPointsForwardThroughLazyController(t *testing.T) {
	app := &App{configPath: "cervterm.lua"}
	if !app.requestConfigReload() || !app.reloadPending {
		t.Fatal("App request entry point did not preserve App-owned pending state")
	}
	flow := app.reloadFlow
	if flow == nil {
		t.Fatal("App request entry point did not initialize reload controller")
	}
	app.pollConfigReload(time.Unix(1, 0))
	app.reloadPending = false
	app.applyPendingConfigReload()
	if app.reloadFlow != flow {
		t.Fatal("App reload entry points replaced the controller")
	}
}

func TestAppMissingReloadSourceAdapterCapturesFailureTime(t *testing.T) {
	app := &App{}
	app.reportMissingReloadSource()
	if app.configReloadAsync.lastNoticeAt.IsZero() {
		t.Fatal("missing-source adapter did not capture its failure time")
	}
}

func TestReloadControllerPortsStayNarrowAndOwnNoPendingPreparedRuntimeConfigOrGPUState(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*reloadSourcePort)(nil)).Elem(),
		reflect.TypeOf((*reloadWatchPort)(nil)).Elem(),
		reflect.TypeOf((*reloadPendingPort)(nil)).Elem(),
		reflect.TypeOf((*reloadWorkerPort)(nil)).Elem(),
		reflect.TypeOf((*reloadFailurePort)(nil)).Elem(),
	}
	allowed := make(map[reflect.Type]struct{}, len(ports))
	methodCount := 0
	for _, port := range ports {
		allowed[port] = struct{}{}
		methodCount += port.NumMethod()
		if port.NumMethod() > 5 {
			t.Errorf("%s has %d methods", port.Name(), port.NumMethod())
		}
		for method := 0; method < port.NumMethod(); method++ {
			methodType := port.Method(method).Type
			for input := 0; input < methodType.NumIn(); input++ {
				assertReloadControllerType(t, port.Name()+"."+port.Method(method).Name, methodType.In(input))
			}
			for output := 0; output < methodType.NumOut(); output++ {
				assertReloadControllerType(t, port.Name()+"."+port.Method(method).Name, methodType.Out(output))
			}
		}
	}
	if methodCount != reloadControllerPortBudget {
		t.Fatalf("aggregate reload controller methods = %d, budget = %d", methodCount, reloadControllerPortBudget)
	}
	controller := reflect.TypeOf(reloadController{})
	for fieldIndex := 0; fieldIndex < controller.NumField(); fieldIndex++ {
		field := controller.Field(fieldIndex)
		assertReloadControllerType(t, controller.Name()+"."+field.Name, field.Type)
		if field.Type.Kind() != reflect.Interface {
			t.Errorf("%s.%s is not a narrow port: %s", controller.Name(), field.Name, field.Type)
			continue
		}
		if _, ok := allowed[field.Type]; !ok {
			t.Errorf("%s.%s uses unlisted port %s", controller.Name(), field.Name, field.Type)
		}
	}
}

func assertReloadControllerType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	typeName := typ.String()
	lower := strings.ToLower(typeName)
	for _, forbidden := range []string{"prepared", "config.", "gpu.", "script.", "runtime.", "*glfwgl.app", "*mux.mux", "*glfw.window", "map[", "func("} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("%s has forbidden type %s", name, typeName)
		}
	}
}
