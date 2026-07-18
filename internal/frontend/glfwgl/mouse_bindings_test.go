//go:build glfw

package glfwgl

import (
	"testing"
	"time"

	"cervterm/internal/script"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestNormalizePointerButtonAndWheel(t *testing.T) {
	event, ok := normalizePointerButton(glfw.MouseButtonRight, glfw.Press, glfw.ModShift|glfw.ModControl, 12, 34, 4)
	if !ok || event.Event != script.MousePress || event.Button != script.MouseRight || event.Mods != script.ModShift|script.ModCtrl || event.ClickCount != 3 || event.X != 12 || event.Y != 34 {
		t.Fatalf("button event=%#v ok=%v", event, ok)
	}
	release, ok := normalizePointerButton(glfw.MouseButtonRight, glfw.Release, 0, 1, 2, 0)
	if !ok || release.Event != script.MouseRelease || release.ClickCount != 1 {
		t.Fatalf("release=%#v ok=%v", release, ok)
	}
	wheel, ok := normalizePointerWheel(-1, glfw.ModAlt, 5, 6)
	if !ok || wheel.Event != script.MouseWheel || wheel.Button != script.MouseDown || wheel.Mods != script.ModAlt {
		t.Fatalf("wheel=%#v ok=%v", wheel, ok)
	}
	if _, ok := normalizePointerWheel(0, 0, 0, 0); ok {
		t.Fatal("zero wheel delta normalized")
	}
}

func TestMouseBindingMatchIsExact(t *testing.T) {
	bindings := []script.MouseBinding{
		{Spec: script.MouseSpec{Event: script.MousePress, Button: script.MouseLeft, Mods: script.ModShift, ClickCount: 1}},
		{Spec: script.MouseSpec{Event: script.MousePress, Button: script.MouseLeft, Mods: script.ModShift, ClickCount: 2}},
	}
	event := pointerEvent{Event: script.MousePress, Button: script.MouseLeft, Mods: script.ModShift, ClickCount: 2}
	matched := matchMouseBinding(bindings, event)
	if matched == nil || matched.Spec.ClickCount != 2 {
		t.Fatalf("matched=%#v", matched)
	}
	event.Mods = script.ModShift | script.ModCtrl
	if got := matchMouseBinding(bindings, event); got != nil {
		t.Fatalf("non-exact modifiers matched %#v", got)
	}
}

func TestClickCountIsBoundedAndResets(t *testing.T) {
	a := &App{}
	now := time.Unix(100, 0)
	if got := a.nextClickCount(script.MouseLeft, now); got != 1 {
		t.Fatal(got)
	}
	if got := a.nextClickCount(script.MouseLeft, now.Add(100*time.Millisecond)); got != 2 {
		t.Fatal(got)
	}
	if got := a.nextClickCount(script.MouseLeft, now.Add(200*time.Millisecond)); got != 3 {
		t.Fatal(got)
	}
	if got := a.nextClickCount(script.MouseLeft, now.Add(300*time.Millisecond)); got != 3 {
		t.Fatal(got)
	}
	if got := a.nextClickCount(script.MouseRight, now.Add(350*time.Millisecond)); got != 1 {
		t.Fatal(got)
	}
	if got := a.nextClickCount(script.MouseRight, now.Add(time.Second)); got != 1 {
		t.Fatal(got)
	}
}

func TestCapturedDragKeepsPressOwnership(t *testing.T) {
	a := &App{mouseBindingCapture: mouseBindingCapture{active: true, button: script.MouseLeft, mods: script.ModShift, clickCount: 2, origin: 7}}
	if !a.handleConfiguredMouseDrag(20, 30) {
		t.Fatal("captured drag was not consumed")
	}
	if !a.mouseBindingCapture.active || a.mouseBindingCapture.mods != script.ModShift || a.mouseBindingCapture.origin != 7 {
		t.Fatalf("capture=%#v", a.mouseBindingCapture)
	}
}
