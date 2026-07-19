//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	termmux "cervterm/internal/mux"
)

type fakeNativeWindow struct {
	id        string
	log       *[]string
	close     bool
	destroyed int
}

func (w *fakeNativeWindow) MakeContextCurrent() { *w.log = append(*w.log, "current:"+w.id) }
func (w *fakeNativeWindow) Focus()              { *w.log = append(*w.log, "focus:"+w.id) }
func (w *fakeNativeWindow) ShouldClose() bool   { return w.close }
func (w *fakeNativeWindow) Destroy()            { w.destroyed++; *w.log = append(*w.log, "destroy:"+w.id) }

type fakeNativePump struct{ log *[]string }

func (p fakeNativePump) PollEvents() { *p.log = append(*p.log, "poll") }
func (p fakeNativePump) WaitEventsTimeout(d time.Duration) {
	*p.log = append(*p.log, "wait:"+d.String())
}

func TestWindowControllerSerializesContextEventsFrameAndClose(t *testing.T) {
	var log []string
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	w1, w2 := &fakeNativeWindow{id: "one", log: &log}, &fakeNativeWindow{id: "two", log: &log}
	if err := c.attach(1, w1, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := c.attach(2, w2, func([]termmux.Event) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := c.setTeardown(1, func() error { log = append(log, "resources:one"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	if err := c.pollEvents(); err != nil {
		t.Fatal(err)
	}
	if err := c.waitEvents(25 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := c.withCurrent(2, func() { log = append(log, "frame:two") }); err != nil {
		t.Fatal(err)
	}
	if c.active != 1 || c.current != 2 {
		t.Fatalf("focus/current=%d/%d", c.active, c.current)
	}
	if err := c.focus(2); err != nil {
		t.Fatal(err)
	}
	if err := c.closeProjection(1); err != nil {
		t.Fatal(err)
	}
	if err := c.closeProjection(1); err != nil {
		t.Fatal(err)
	}
	want := []string{"poll", "wait:25ms", "current:two", "frame:two", "focus:two", "current:one", "resources:one", "destroy:one"}
	if !reflect.DeepEqual(log, want) || w1.destroyed != 1 {
		t.Fatalf("log=%v destroyed=%d", log, w1.destroyed)
	}
}

func TestWindowControllerRoutesAddressedEventsAndDamage(t *testing.T) {
	var log []string
	var one, two []termmux.Event
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	if err := c.attach(1, &fakeNativeWindow{id: "one", log: &log}, func(events []termmux.Event) bool { one = append(one, events...); return true }); err != nil {
		t.Fatal(err)
	}
	if err := c.attach(2, &fakeNativeWindow{id: "two", log: &log}, func(events []termmux.Event) bool { two = append(two, events...); return true }); err != nil {
		t.Fatal(err)
	}
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	if err := c.focus(2); err != nil {
		t.Fatal(err)
	}
	events := []termmux.Event{{Kind: termmux.PaneOutput}, {Kind: termmux.PaneTransferred, Window: 1, SourceWindow: 2}, {Kind: termmux.TabRevisionChanged, Window: 2, SourceWindow: 1}}
	if !c.dispatch(events) {
		t.Fatal("events not consumed")
	}
	if len(one) != 1 || one[0].Kind != termmux.PaneTransferred {
		t.Fatalf("one=%#v", one)
	}
	if len(two) != 2 || two[0].Kind != termmux.PaneOutput || two[1].Kind != termmux.TabRevisionChanged {
		t.Fatalf("two=%#v", two)
	}
	if !c.windows[1].dirty || !c.windows[2].dirty {
		t.Fatal("damage not routed")
	}
}

func TestWindowControllerRejectsLifecycleOutsideLoopAndDuplicates(t *testing.T) {
	var log []string
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	host := &fakeNativeWindow{id: "one", log: &log}
	if err := c.attach(1, host, func([]termmux.Event) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if err := c.attach(1, host, func([]termmux.Event) bool { return false }); !errors.Is(err, errWindowProjectionExists) {
		t.Fatalf("duplicate=%v", err)
	}
	if err := c.activate(1); !errors.Is(err, errWindowLoopInactive) {
		t.Fatalf("activate=%v", err)
	}
	if err := c.closeProjection(1); !errors.Is(err, errWindowLoopInactive) {
		t.Fatalf("close=%v", err)
	}
	if host.destroyed != 0 {
		t.Fatal("destroyed outside loop")
	}
}

func TestWindowControllerRetainsEventsUntilProjectionAttaches(t *testing.T) {
	var log []string
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	event := termmux.Event{Kind: termmux.WindowTabsEmpty, Window: 9}
	if c.dispatch([]termmux.Event{event}) {
		t.Fatal("missing projection consumed event")
	}
	if len(c.pending[9]) != 1 {
		t.Fatalf("pending=%#v", c.pending)
	}
	var got []termmux.Event
	if err := c.attach(9, &fakeNativeWindow{id: "nine", log: &log}, func(events []termmux.Event) bool { got = append(got, events...); return true }); err != nil {
		t.Fatal(err)
	}
	if !c.dispatch(nil) || len(got) != 1 || got[0].Kind != event.Kind {
		t.Fatalf("got=%#v pending=%#v", got, c.pending)
	}
}

type fakeProjectionFactory struct {
	log       *[]string
	failStage int
	created   map[termmux.WindowID]*fakeNativeWindow
	stages    []string
}

func (f *fakeProjectionFactory) Create(id termmux.WindowID) (*nativeProjectionBundle, error) {
	host := &fakeNativeWindow{id: fmt.Sprintf("%d", id), log: f.log}
	bundle := &nativeProjectionBundle{host: host, handle: func([]termmux.Event) bool { return true }}
	f.created[id] = host
	for i, stage := range f.stages {
		if i == f.failStage {
			return bundle, fmt.Errorf("injected %s failure", stage)
		}
		name := stage
		*f.log = append(*f.log, "open:"+name)
		bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
			*f.log = append(*f.log, "close:"+name)
			return nil
		}))
	}
	return bundle, nil
}

func projectionStages() []string {
	return []string{"model", "native", "config", "callback", "context", "renderer", "atlas", "font", "background", "blur", "pty", "session"}
}

func TestWindowControllerCandidateFailureRollsBackEveryAcquiredStageBeforePublication(t *testing.T) {
	for fail, stage := range projectionStages() {
		t.Run(stage, func(t *testing.T) {
			var log []string
			factory := &fakeProjectionFactory{log: &log, failStage: fail, created: make(map[termmux.WindowID]*fakeNativeWindow), stages: projectionStages()}
			c := newWindowController(processServices{}, fakeNativePump{log: &log})
			c.setProjectionFactory(factory)
			if err := c.startLoop(); err != nil {
				t.Fatal(err)
			}
			if err := c.createProjection(2); err == nil {
				t.Fatal("expected injected failure")
			}
			if len(c.windows) != 0 || len(c.order) != 0 || c.active != 0 || c.current != 0 {
				t.Fatalf("failed candidate published: windows=%v order=%v", c.windows, c.order)
			}
			want := make([]string, 0, fail*2+1)
			for _, opened := range projectionStages()[:fail] {
				want = append(want, "open:"+opened)
			}
			for i := fail - 1; i >= 0; i-- {
				want = append(want, "close:"+projectionStages()[i])
			}
			want = append(want, "destroy:2")
			if !reflect.DeepEqual(log, want) {
				t.Fatalf("log=%v want=%v", log, want)
			}
		})
	}
}

func TestWindowControllerCreateFocusCloseLoopsOwnIndependentBundles(t *testing.T) {
	var log []string
	factory := &fakeProjectionFactory{log: &log, failStage: -1, created: make(map[termmux.WindowID]*fakeNativeWindow), stages: projectionStages()}
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	c.setProjectionFactory(factory)
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	for id := termmux.WindowID(1); id <= 3; id++ {
		if err := c.createProjection(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.focus(2); err != nil {
		t.Fatal(err)
	}
	c.clearDamage(1)
	c.clearDamage(2)
	c.clearDamage(3)
	event := termmux.Event{Kind: termmux.PaneOutput, Window: 2}
	if !c.dispatch([]termmux.Event{event}) || c.windows[1].dirty || !c.windows[2].dirty || c.windows[3].dirty {
		t.Fatalf("addressed damage leaked: one=%v two=%v three=%v", c.windows[1].dirty, c.windows[2].dirty, c.windows[3].dirty)
	}
	if err := c.closeProjection(2); err != nil {
		t.Fatal(err)
	}
	if factory.created[1].destroyed != 0 || factory.created[2].destroyed != 1 || factory.created[3].destroyed != 0 {
		t.Fatalf("sibling close leakage: %#v", factory.created)
	}
	if c.active != 1 || len(c.windows) != 2 {
		t.Fatalf("fallback active=%d windows=%d", c.active, len(c.windows))
	}
	if err := c.closeProjection(1); err != nil {
		t.Fatal(err)
	}
	if err := c.closeProjection(3); err != nil {
		t.Fatal(err)
	}
	if len(c.windows) != 0 || len(c.order) != 0 || c.active != 0 || c.current != 0 {
		t.Fatalf("controller not empty: windows=%v order=%v active=%d current=%d", c.windows, c.order, c.active, c.current)
	}
	wantLast := []string{"current:3", "close:session", "close:pty", "close:blur", "close:background", "close:font", "close:atlas", "close:renderer", "close:context", "close:callback", "close:config", "close:native", "close:model", "destroy:3"}
	last := log[len(log)-len(wantLast):]
	if !reflect.DeepEqual(last, wantLast) {
		t.Fatalf("final close order=%v want=%v", last, wantLast)
	}
}

type fakeCandidateFactory struct {
	log      *[]string
	prepare  error
	bind     error
	host     *fakeNativeWindow
	app      *App
	resource projectionResource
}

func (f *fakeCandidateFactory) Prepare() (*nativeProjectionBundle, termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics, string, error) {
	*f.log = append(*f.log, "prepare-native")
	bundle := &nativeProjectionBundle{host: f.host, app: f.app, handle: func([]termmux.Event) bool { return true }}
	bundle.bind = func(id termmux.WindowID) error {
		*f.log = append(*f.log, fmt.Sprintf("bind:%d", id))
		return f.bind
	}
	if f.resource != nil {
		bundle.resources = append(bundle.resources, f.resource)
	}
	return bundle, termmux.SpawnSpec{}, termmux.PixelRect{Width: 800, Height: 480}, termmux.CellMetrics{CellWidth: 8, CellHeight: 16}, "candidate", f.prepare
}

type fakeRuntimeWindows struct {
	log       *[]string
	createErr error
	closeErr  error
	closed    int
	next      termmux.WindowID
}

func (f *fakeRuntimeWindows) CreateWindow(termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics, string) (termmux.WindowView, []termmux.Event, error) {
	*f.log = append(*f.log, "create-runtime")
	if f.createErr != nil {
		return termmux.WindowView{}, nil, f.createErr
	}
	if f.next == 0 {
		f.next = 2
	}
	return termmux.WindowView{ID: f.next}, []termmux.Event{{Kind: termmux.WindowCreated, Window: f.next}}, nil
}

func (f *fakeRuntimeWindows) ActivateWindow(id termmux.WindowID) ([]termmux.Event, error) {
	*f.log = append(*f.log, fmt.Sprintf("activate-runtime:%d", id))
	return []termmux.Event{{Kind: termmux.WindowActivated, Window: id}}, nil
}

func (f *fakeRuntimeWindows) CloseWindow(id termmux.WindowID) (termmux.CloseWindowResult, []termmux.Event, error) {
	f.closed++
	*f.log = append(*f.log, fmt.Sprintf("close-runtime:%d", id))
	return termmux.CloseWindowResult{Closed: true, Empty: true}, []termmux.Event{{Kind: termmux.WindowClosed, Window: id}}, f.closeErr
}

func (f *fakeRuntimeWindows) RollbackWindow(id termmux.WindowID) error {
	f.closed++
	*f.log = append(*f.log, fmt.Sprintf("rollback-runtime:%d", id))
	return f.closeErr
}

func TestWindowControllerRuntimeCreateRollsBackMuxBeforeNativeOnBindFailure(t *testing.T) {
	var log []string
	host := &fakeNativeWindow{id: "two", log: &log}
	factory := &fakeCandidateFactory{log: &log, bind: errors.New("bind"), host: host, resource: projectionResourceFunc(func() error { log = append(log, "close-resource"); return nil })}
	runtimeWindows := &fakeRuntimeWindows{log: &log}
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	c.setCandidateFactory(factory)
	c.setRuntimeWindows(runtimeWindows)
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.createRuntimeProjection(); !errors.Is(err, factory.bind) {
		t.Fatalf("err=%v", err)
	}
	want := []string{"prepare-native", "create-runtime", "bind:2", "rollback-runtime:2", "close-resource", "destroy:two"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("log=%v want=%v", log, want)
	}
	if runtimeWindows.closed != 1 || host.destroyed != 1 || len(c.windows) != 0 {
		t.Fatalf("closed=%d destroyed=%d windows=%d", runtimeWindows.closed, host.destroyed, len(c.windows))
	}
}

func TestWindowControllerRuntimeCreatePublishActivateAndCloseOrdering(t *testing.T) {
	var log []string
	host := &fakeNativeWindow{id: "two", log: &log}
	factory := &fakeCandidateFactory{log: &log, host: host, resource: projectionResourceFunc(func() error { log = append(log, "close-resource"); return nil })}
	runtimeWindows := &fakeRuntimeWindows{log: &log}
	c := newWindowController(processServices{}, fakeNativePump{log: &log})
	c.setCandidateFactory(factory)
	c.setRuntimeWindows(runtimeWindows)
	if err := c.startLoop(); err != nil {
		t.Fatal(err)
	}
	id, err := c.createRuntimeProjection()
	if err != nil || id != 2 || c.windows[id] == nil {
		t.Fatalf("id=%d err=%v windows=%v", id, err, c.windows)
	}
	if err := c.activateRuntimeProjection(id); err != nil {
		t.Fatal(err)
	}
	result, err := c.closeRuntimeProjection(id)
	if err != nil || !result.Empty {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	want := []string{"prepare-native", "create-runtime", "bind:2", "focus:two", "activate-runtime:2", "focus:two", "close-runtime:2", "current:two", "close-resource", "destroy:two"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("log=%v want=%v", log, want)
	}
	if host.destroyed != 1 || len(c.windows) != 0 {
		t.Fatalf("destroyed=%d windows=%d", host.destroyed, len(c.windows))
	}
}
