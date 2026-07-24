//go:build glfw

package glfwgl

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRenderPorts struct {
	log       []string
	ready     bool
	panicBody bool
}

func (f *fakeRenderPorts) tickRenderProjection() {
	f.log = append(f.log, "tick")
}

func (f *fakeRenderPorts) renderReady(time.Time) bool {
	f.log = append(f.log, "ready")
	return f.ready
}

func (f *fakeRenderPorts) throttleRender(time.Time) {
	f.log = append(f.log, "throttle")
}

func (f *fakeRenderPorts) beginRenderFrame() {
	f.log = append(f.log, "begin")
}

func (f *fakeRenderPorts) drawRenderFrameBody() {
	f.log = append(f.log, "body")
	if f.panicBody {
		panic("draw panic")
	}
}

func (f *fakeRenderPorts) finishRenderFrame() {
	f.log = append(f.log, "finish")
}

func (f *fakeRenderPorts) endRenderFrame() {
	f.log = append(f.log, "end")
}

func newFakeRenderController(ports *fakeRenderPorts) *renderController {
	return newRenderController(ports, ports, ports)
}

func assertRenderTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("render trace = %v, want %v", got, want)
	}
}

func TestRenderControllerOnDemandReadyDrawOrder(t *testing.T) {
	ports := &fakeRenderPorts{ready: true}
	if !newFakeRenderController(ports).renderProjection(time.Unix(1, 0), false) {
		t.Fatal("ready on-demand projection did not draw")
	}
	assertRenderTrace(t, ports.log, []string{"tick", "ready", "begin", "body", "finish", "end"})
}

func TestRenderControllerOnDemandRejectionStopsAfterReadiness(t *testing.T) {
	ports := &fakeRenderPorts{}
	if newFakeRenderController(ports).renderProjection(time.Unix(1, 0), false) {
		t.Fatal("rejected on-demand projection reported a draw")
	}
	assertRenderTrace(t, ports.log, []string{"tick", "ready"})
}

func TestRenderControllerContinuousThrottlesWithoutReadinessCheck(t *testing.T) {
	ports := &fakeRenderPorts{}
	if !newFakeRenderController(ports).renderProjection(time.Unix(1, 0), true) {
		t.Fatal("continuous projection did not draw")
	}
	assertRenderTrace(t, ports.log, []string{"tick", "throttle", "begin", "body", "finish", "end"})
}

func TestRenderControllerFinishesDrawOnPanicWithoutEndingFrame(t *testing.T) {
	ports := &fakeRenderPorts{ready: true, panicBody: true}
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("draw body did not panic")
		}
		assertRenderTrace(t, ports.log, []string{"tick", "ready", "begin", "body", "finish"})
	}()
	newFakeRenderController(ports).renderProjection(time.Unix(1, 0), false)
}

func TestRenderControllerRejectedPathDoesNotAllocate(t *testing.T) {
	ports := &fakeRenderPorts{log: make([]string, 0, 2)}
	controller := newFakeRenderController(ports)
	now := time.Unix(1, 0)
	allocs := testing.AllocsPerRun(1000, func() {
		ports.log = ports.log[:0]
		if controller.renderProjection(now, false) {
			panic("rejected projection drew")
		}
	})
	if allocs != 0 {
		t.Fatalf("allocations per rejected render = %v, want 0", allocs)
	}
}

func TestRenderControllerPortsStayNarrowAndFieldsStayDetached(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*renderTickPort)(nil)).Elem(),
		reflect.TypeOf((*renderPresentationPort)(nil)).Elem(),
		reflect.TypeOf((*renderFramePort)(nil)).Elem(),
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
				assertDetachedControllerType(t, port.Name()+"."+port.Method(method).Name, methodType.In(input))
			}
			for output := 0; output < methodType.NumOut(); output++ {
				assertDetachedControllerType(t, port.Name()+"."+port.Method(method).Name, methodType.Out(output))
			}
		}
	}
	if methodCount != renderControllerPortBudget {
		t.Fatalf("aggregate render controller methods = %d, budget = %d", methodCount, renderControllerPortBudget)
	}
	controller := reflect.TypeOf(renderController{})
	for fieldIndex := 0; fieldIndex < controller.NumField(); fieldIndex++ {
		field := controller.Field(fieldIndex)
		assertDetachedControllerType(t, controller.Name()+"."+field.Name, field.Type)
		if field.Type.Kind() != reflect.Interface {
			t.Errorf("%s.%s is not a narrow port: %s", controller.Name(), field.Name, field.Type)
			continue
		}
		if _, ok := allowed[field.Type]; !ok {
			t.Errorf("%s.%s uses unlisted port %s", controller.Name(), field.Name, field.Type)
		}
	}
}

func assertDetachedControllerType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	typeName := typ.String()
	lower := strings.ToLower(typeName)
	for _, forbidden := range []string{"*glfwgl.app", "*mux.mux", "*glfw.window", "gpu.renderer", "script.runtime", "runtime.", "map[", "func("} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("%s has forbidden type %s", name, typeName)
		}
	}
}
