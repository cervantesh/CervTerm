//go:build glfw

package glfwgl

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	termaction "cervterm/internal/action"
	termmux "cervterm/internal/mux"
)

type recordingWindowCapabilityFactory struct {
	owner    *termmux.Owner
	calls    int
	attested bool
}

func (f *recordingWindowCapabilityFactory) ForWindow(id termmux.WindowID, attest termmux.WindowAttestor) (*termmux.WindowOwner, error) {
	f.calls++
	window, err := f.owner.ForWindow(id, attest)
	if err == nil {
		f.attested = attest(window.Identity())
	}
	return window, err
}

func TestChildGlobalActionRoutesControllerWithExactOriginIdentity(t *testing.T) {
	var log []string
	process := termmux.NewOwner(idleTestFactory{}, termmux.Options{})
	t.Cleanup(func() { _ = process.Shutdown() })
	_, _, _, err := mustTestWindowMux(t, process).Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := process.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	primaryWindow, err := process.ForWindow(initialWindowID, func(termmux.WindowIdentity) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	childWindow, err := process.ForWindow(second.ID, func(termmux.WindowIdentity) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	factory := &recordingWindowCapabilityFactory{owner: process}
	controller := newWindowController(processServices{commands: process, windowCapabilities: factory}, fakeNativePump{log: &log})
	router := newProjectionMessageRouter(controller)
	primary := &App{host: controller, controller: router, mux: primaryWindow, windowID: initialWindowID, windowIdentity: primaryWindow.Identity()}
	child := &App{controller: router, mux: childWindow, windowID: second.ID, windowIdentity: childWindow.Identity()}
	controller.primary = primary
	if err := controller.attachApp(initialWindowID, &fakeNativeWindow{id: "primary", log: &log}, primary, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.attachApp(second.ID, &fakeNativeWindow{id: "child", log: &log}, child, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if controller.inLoop {
			for _, id := range controller.projectionIDs() {
				_ = controller.closeProjection(id)
			}
			controller.stopLoop()
		}
	})

	before := len(process.Workspaces())
	if err := child.executeCreateWorkspace(termaction.CreateWorkspace{Name: "child-origin"}); err != nil {
		t.Fatal(err)
	}
	if got := len(process.Workspaces()); got != before+1 {
		t.Fatalf("workspace count=%d want=%d", got, before+1)
	}
	stable := process.Workspaces()
	child.windowIdentity.Incarnation++
	if err := child.executeCreateWorkspace(termaction.CreateWorkspace{Name: "forged"}); !errors.Is(err, termmux.ErrWrongOrigin) {
		t.Fatalf("stale origin error=%v", err)
	}
	if !reflect.DeepEqual(process.Workspaces(), stable) {
		t.Fatal("stale child origin reached process command capability")
	}
}

func TestControllerMintsChildWindowCapabilityOnlyAfterDetachedNativeAttestation(t *testing.T) {
	var log []string
	process := termmux.NewOwner(idleTestFactory{}, termmux.Options{})
	t.Cleanup(func() { _ = process.Shutdown() })
	_, _, _, err := mustTestWindowMux(t, process).Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := process.CreateWindow(termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "two")
	if err != nil {
		t.Fatal(err)
	}
	factory := &recordingWindowCapabilityFactory{owner: process}
	controller := newWindowController(processServices{commands: process, windowCapabilities: factory}, fakeNativePump{log: &log})
	controller.contextCurrent = func(nativeWindowHost) bool { return true }
	child := &App{controller: newProjectionMessageRouter(controller), windowID: second.ID}
	host := &fakeNativeWindow{id: "child", log: &log}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.stopLoop)
	capability, identity, err := controller.acquireWindowCapability(second.ID, child, host)
	if err != nil {
		t.Fatal(err)
	}
	if capability == nil || identity.ID != second.ID || identity.Incarnation == 0 || factory.calls != 1 || !factory.attested {
		t.Fatalf("capability=%T identity=%#v calls=%d attested=%v", capability, identity, factory.calls, factory.attested)
	}
}

type shutdownRecordingCommands struct {
	processCommandCapability
	log      *[]string
	shutdown int
}

func (p *shutdownRecordingCommands) Shutdown() error {
	*p.log = append(*p.log, "shutdown:process")
	p.shutdown++
	return nil
}

func (p *shutdownRecordingCommands) Closed() bool { return p.shutdown != 0 }

type unusedWindowCapabilityFactory struct{ windowCapabilityFactory }

func TestOnlyPrimaryClosesProjectionsThenProcessServices(t *testing.T) {
	var log []string
	commands := &shutdownRecordingCommands{log: &log}
	controller := newWindowController(processServices{commands: commands, windowCapabilities: unusedWindowCapabilityFactory{}}, fakeNativePump{log: &log})
	router := newProjectionMessageRouter(controller)
	primary := &App{host: controller, controller: router}
	child := &App{controller: router}
	controller.primary = primary
	host := &fakeNativeWindow{id: "primary", log: &log}
	if err := controller.attachApp(initialWindowID, host, primary, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := controller.setTeardown(initialWindowID, func() error { log = append(log, "close:projection-resource"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	child.shutdownProcessServices()
	if commands.shutdown != 0 {
		t.Fatal("child projection shut down process services")
	}
	if err := primary.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
	if err := primary.rollbackInitializedMux(nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"current:primary", "close:projection-resource", "destroy:primary", "shutdown:process"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("shutdown order=%v want=%v", log, want)
	}
	if commands.shutdown != 1 || controller.inLoop || len(controller.windows) != 0 || controller.services.commands != nil || controller.services.windowCapabilities != nil {
		t.Fatalf("shutdowns=%d loop=%v windows=%d services=%#v", commands.shutdown, controller.inLoop, len(controller.windows), controller.services)
	}
}

func TestChildAppFieldGraphCannotReachHostOrProcessServiceStorage(t *testing.T) {
	controller := newWindowController(processServices{}, fakeNativePump{log: &[]string{}})
	primary := &App{host: controller, controller: newProjectionMessageRouter(controller)}
	child := newProjectionApp(primary)
	if child.host != nil {
		t.Fatalf("child retained concrete host %T", child.host)
	}
	router, ok := child.controller.(*projectionMessageRouter)
	if !ok || router == nil {
		t.Fatalf("child projection controller=%T want opaque message router", child.controller)
	}
	forbidden := map[reflect.Type]bool{
		reflect.TypeOf(windowController{}):    true,
		reflect.TypeOf(processServices{}):     true,
		reflect.TypeOf((*termmux.Owner)(nil)): true,
	}
	seen := make(map[reflect.Type]bool)
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, path string) {
		if typ == nil || seen[typ] {
			return
		}
		seen[typ] = true
		if forbidden[typ] {
			t.Errorf("child field graph reaches forbidden %s through %s", typ, path)
			return
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
			walk(typ.Elem(), path+"->"+typ.String())
		case reflect.Func:
			for i := 0; i < typ.NumIn(); i++ {
				walk(typ.In(i), path+"->argument")
			}
			for i := 0; i < typ.NumOut(); i++ {
				walk(typ.Out(i), path+"->result")
			}
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				walk(field.Type, path+"."+field.Name)
			}
		case reflect.Interface:
			if typ != reflect.TypeOf((*error)(nil)).Elem() {
				t.Errorf("child field graph reaches opaque interface %s through %s", typ, path)
			}
		}
	}
	walk(reflect.TypeOf(router), "child.controller")
	source, err := os.ReadFile("projection_controller.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "type projectionMessageRouter struct {")
	end := strings.Index(text[start:], "\n}")
	if start < 0 || end < 0 {
		t.Fatal("projectionMessageRouter AST fixture missing")
	}
	storage := text[start : start+end]
	for _, forbiddenText := range []string{"windowController", "processServices", "processCommandCapability", "windowCapabilityFactory", "termmux.Owner"} {
		if strings.Contains(storage, forbiddenText) {
			t.Fatalf("projection router field graph exposes forbidden locator %q", forbiddenText)
		}
	}
}
