//go:build glfw

package glfwgl

import (
	"strings"
	"testing"
	"time"
)

func TestBindingSuppressionConsumesExactlyOneCharacter(t *testing.T) {
	var suppression charSuppression
	now := time.Unix(100, 0)
	suppression.armBinding(true)
	if !suppression.bindingArmed() || !suppression.consume('a', now) || suppression.consume('b', now) || suppression.bindingArmed() {
		t.Fatalf("binding suppression=%#v", suppression)
	}
	suppression.armBinding(false)
	if suppression.consume('c', now) {
		t.Fatal("disabled binding suppression consumed a character")
	}
}

func TestIMEEchoSuppressionMatchesFullSequenceAndDeadline(t *testing.T) {
	var suppression charSuppression
	now := time.Unix(200, 0)
	if !suppression.armIMEEcho(7, "日本😀", now) {
		t.Fatal("echo arm failed")
	}
	for _, r := range []rune("日本😀") {
		if !suppression.consume(r, now.Add(50*time.Millisecond)) {
			t.Fatalf("matching rune %q was not consumed", r)
		}
	}
	if suppression.echoGeneration != 0 || suppression.consume('x', now) {
		t.Fatalf("completed echo retained state: %#v", suppression)
	}

	if !suppression.armIMEEcho(8, "ab", now) || !suppression.consume('a', now) || suppression.consume('x', now) || suppression.consume('b', now) {
		t.Fatalf("mismatch did not clear echo: %#v", suppression)
	}
	if !suppression.armIMEEcho(9, "z", now) || suppression.consume('z', now.Add(imeEchoDeadline)) {
		t.Fatalf("expired echo consumed input: %#v", suppression)
	}
}

func TestIMEEchoSuppressionBoundsAndClearPaths(t *testing.T) {
	var suppression charSuppression
	now := time.Unix(300, 0)
	for _, text := range []string{"", string([]byte{0xff}), strings.Repeat("x", 64*1024+1)} {
		if suppression.armIMEEcho(1, text, now) {
			t.Fatalf("invalid echo armed for %d bytes", len(text))
		}
	}
	if suppression.armIMEEcho(0, "x", now) {
		t.Fatal("zero generation armed")
	}
	if !suppression.armIMEEcho(1, "x", now) {
		t.Fatal("valid echo did not arm")
	}
	suppression.clearOnNonEchoInput()
	if suppression.consume('x', now) {
		t.Fatal("non-echo input did not clear echo")
	}
	suppression.armBinding(true)
	suppression.clearOnNonEchoInput()
	if suppression.bindingArmed() || suppression.consume('x', now) {
		t.Fatal("non-echo input did not clear stale binding suppression")
	}
	suppression.armBinding(true)
	suppression.clear()
	if suppression.bindingArmed() || suppression.consume('x', now) {
		t.Fatal("clear retained suppression")
	}
}
