//go:build glfw

package glfwgl

import (
	"errors"
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
	want := []string{"poll", "wait:25ms", "current:two", "frame:two", "current:one", "resources:one", "destroy:one"}
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
