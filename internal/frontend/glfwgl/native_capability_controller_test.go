//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"

	termmux "cervterm/internal/mux"
)

type fakeNativeCapabilityPorts struct {
	log              []string
	accessibilityErr error
	adoptionErr      error
	initialRollback  error
	activationErr    error
	bindErr          error
	childRollback    error
	bound            termmux.WindowID
}

func (f *fakeNativeCapabilityPorts) activateInitialIME() {
	f.log = append(f.log, "ime")
}

func (f *fakeNativeCapabilityPorts) prepareInitialAccessibility() error {
	f.log = append(f.log, "accessibility")
	return f.accessibilityErr
}

func (f *fakeNativeCapabilityPorts) adoptInitialCapabilities() error {
	f.log = append(f.log, "adopt")
	return f.adoptionErr
}

func (f *fakeNativeCapabilityPorts) rollbackInitialCapabilities() error {
	f.log = append(f.log, "rollback-initial")
	return f.initialRollback
}

func (f *fakeNativeCapabilityPorts) activateChildCapabilities() error {
	f.log = append(f.log, "activate-child")
	return f.activationErr
}

func (f *fakeNativeCapabilityPorts) bindChildCapabilities(id termmux.WindowID) error {
	f.log = append(f.log, "bind-child")
	f.bound = id
	return f.bindErr
}

func (f *fakeNativeCapabilityPorts) markChildCapabilitiesReady() {
	f.log = append(f.log, "ready-child")
}

func (f *fakeNativeCapabilityPorts) rollbackChildCapabilities() error {
	f.log = append(f.log, "rollback-child")
	return f.childRollback
}

func newFakeNativeCapabilityController(ports *fakeNativeCapabilityPorts) *nativeCapabilityController {
	return newNativeCapabilityController(ports, ports)
}

func assertNativeCapabilityTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native capability trace = %v, want %v", got, want)
	}
}

func TestNativeCapabilityControllerOrdersInitialIMEAccessibilityAndAdoption(t *testing.T) {
	ports := &fakeNativeCapabilityPorts{}
	if err := newFakeNativeCapabilityController(ports).activateInitial(); err != nil {
		t.Fatal(err)
	}
	assertNativeCapabilityTrace(t, ports.log, []string{"ime", "accessibility", "adopt"})
}

func TestNativeCapabilityControllerReturnsInitialRollbackResults(t *testing.T) {
	causeAccessibility := errors.New("accessibility")
	causeAdoption := errors.New("adoption")
	rollback := errors.New("rollback")
	tests := []struct {
		name          string
		accessibility error
		adoption      error
		cause         error
		want          []string
	}{
		{
			name: "accessibility", accessibility: causeAccessibility, cause: causeAccessibility,
			want: []string{"ime", "accessibility", "rollback-initial"},
		},
		{
			name: "adoption", adoption: causeAdoption, cause: causeAdoption,
			want: []string{"ime", "accessibility", "adopt", "rollback-initial"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := &fakeNativeCapabilityPorts{accessibilityErr: test.accessibility, adoptionErr: test.adoption, initialRollback: rollback}
			err := newFakeNativeCapabilityController(ports).activateInitial()
			if !errors.Is(err, test.cause) || !errors.Is(err, rollback) {
				t.Fatalf("result = %v, want cause %v and rollback %v", err, test.cause, rollback)
			}
			assertNativeCapabilityTrace(t, ports.log, test.want)
		})
	}
}

func TestNativeCapabilityControllerOrdersChildActivationBindAndReadiness(t *testing.T) {
	ports := &fakeNativeCapabilityPorts{}
	if err := newFakeNativeCapabilityController(ports).activateChild(9); err != nil {
		t.Fatal(err)
	}
	assertNativeCapabilityTrace(t, ports.log, []string{"activate-child", "bind-child", "ready-child"})
	if ports.bound != 9 {
		t.Fatalf("bound ID = %d, want 9", ports.bound)
	}
}

func TestNativeCapabilityControllerReturnsChildRollbackResultsBeforeReadiness(t *testing.T) {
	activation := errors.New("activation")
	binding := errors.New("binding")
	rollback := errors.New("rollback")
	tests := []struct {
		name       string
		activation error
		binding    error
		cause      error
		want       []string
	}{
		{
			name: "activation", activation: activation, cause: activation,
			want: []string{"activate-child", "rollback-child"},
		},
		{
			name: "binding", binding: binding, cause: binding,
			want: []string{"activate-child", "bind-child", "rollback-child"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := &fakeNativeCapabilityPorts{activationErr: test.activation, bindErr: test.binding, childRollback: rollback}
			err := newFakeNativeCapabilityController(ports).activateChild(4)
			if !errors.Is(err, test.cause) || !errors.Is(err, rollback) {
				t.Fatalf("result = %v, want cause %v and rollback %v", err, test.cause, rollback)
			}
			assertNativeCapabilityTrace(t, ports.log, test.want)
		})
	}
}

func TestNativeCapabilityControllerSuccessfulRoutesDoNotAllocate(t *testing.T) {
	ports := &fakeNativeCapabilityPorts{log: make([]string, 0, 6)}
	controller := newFakeNativeCapabilityController(ports)
	allocs := testing.AllocsPerRun(1000, func() {
		ports.log = ports.log[:0]
		if controller.activateInitial() != nil || controller.activateChild(3) != nil {
			panic("successful capability route failed")
		}
	})
	if allocs != 0 {
		t.Fatalf("allocations per successful capability routes = %v, want 0", allocs)
	}
	assertNativeCapabilityTrace(t, ports.log, []string{"ime", "accessibility", "adopt", "activate-child", "bind-child", "ready-child"})
}

func TestNativeCapabilityControllerPortsAndFieldsAreExhaustiveNarrowAndDetached(t *testing.T) {
	ports := []reflect.Type{
		reflect.TypeOf((*nativeInitialCapabilityPort)(nil)).Elem(),
		reflect.TypeOf((*nativeChildCapabilityPort)(nil)).Elem(),
	}
	fields := []controllerFieldExpectation{
		{name: "initial", typ: ports[0]},
		{name: "child", typ: ports[1]},
	}
	assertControllerPortStructure(t, reflect.TypeOf(nativeCapabilityController{}), fields, ports, nativeCapabilityControllerPortBudget)
}
