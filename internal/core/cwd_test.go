package core

import "testing"

func TestTerminalCwdChangeDetection(t *testing.T) {
	term := NewTerminal(4, 1)
	if term.Cwd() != "" || term.CwdSeq() != 0 {
		t.Fatalf("initial cwd state = %q seq %d", term.Cwd(), term.CwdSeq())
	}
	term.SetCwd("/one")
	if term.Cwd() != "/one" || term.CwdSeq() != 1 {
		t.Fatalf("first cwd state = %q seq %d", term.Cwd(), term.CwdSeq())
	}
	term.SetCwd("/one")
	if term.CwdSeq() != 1 {
		t.Fatalf("unchanged cwd incremented seq to %d", term.CwdSeq())
	}
	term.SetCwd("/two")
	if term.Cwd() != "/two" || term.CwdSeq() != 2 {
		t.Fatalf("second cwd state = %q seq %d", term.Cwd(), term.CwdSeq())
	}
}
