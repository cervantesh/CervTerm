package core

import "testing"

func TestLineWrappedUsesCurrentViewport(t *testing.T) {
	term := NewTerminal(2, 2)
	term.rowWrapped[0] = false
	term.appendScrollbackLine(make([]Cell, 2), true)

	if wrapped, ok := term.LineWrapped(0); !ok || wrapped {
		t.Fatalf("live row wrapped = %t, ok = %t; want false, true", wrapped, ok)
	}
	if !term.ScrollViewport(1) {
		t.Fatal("expected viewport to move into scrollback")
	}
	if wrapped, ok := term.LineWrapped(0); !ok || !wrapped {
		t.Fatalf("history row wrapped = %t, ok = %t; want true, true", wrapped, ok)
	}
	if wrapped, ok := term.LineWrapped(2); ok || wrapped {
		t.Fatalf("out-of-range wrapped = %t, ok = %t; want false, false", wrapped, ok)
	}
}
