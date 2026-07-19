package vt

import (
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestOSCSemanticMarkersBELSTAndChunks(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var p Parser
	p.Advance(term, []byte("\x1b]133;A\aP"))
	p.Advance(term, []byte("\x1b]133;B\x1b\\I"))
	p.Advance(term, []byte("\x1b]633;C"))
	p.Advance(term, []byte("\aO"))
	p.Advance(term, []byte("\x1b]633;D;0\x1b\\N"))
	cells := copyCells(term)
	want := []core.SemanticKind{core.SemanticPrompt, core.SemanticInput, core.SemanticOutput, core.SemanticNone}
	for index, kind := range want {
		if cells[index].SemanticKind != kind {
			t.Fatalf("cell %d kind=%v want=%v", index, cells[index].SemanticKind, kind)
		}
	}
}

func TestOSC633CommandPayloadIsNotStoredAndUnknownIsAtomic(t *testing.T) {
	term := core.NewTerminal(8, 1)
	var p Parser
	p.Advance(term, []byte("\x1b]633;E;echo secret-token\x1b\\X"))
	if term.SemanticKind() != core.SemanticInput || copyCells(term)[0].SemanticKind != core.SemanticInput {
		t.Fatal("E marker not applied")
	}
	p.Advance(term, []byte("\x1b]633;P;Cwd=/private/secret-token\x1b\\"))
	if term.SemanticKind() != core.SemanticInput {
		t.Fatal("ignored property mutated state")
	}
	if strings.Contains(term.PlainText(), "secret-token") {
		t.Fatal("semantic payload reached terminal text")
	}
}

func TestOSCSemanticMalformedAndOversizedMarkersMutateNothing(t *testing.T) {
	term := core.NewTerminal(4, 1)
	var p Parser
	term.SetSemanticKind(core.SemanticOutput)
	for _, sequence := range []string{"\x1b]133;\x1b\\", "\x1b]133;AA\x1b\\", "\x1b]133;Z\x1b\\", "\x1b]133;E;data\x1b\\", "\x1b]633;" + strings.Repeat("x", maxSemanticOSCBytes+1) + "\x1b\\"} {
		p.Advance(term, []byte(sequence))
	}
	if term.SemanticKind() != core.SemanticOutput {
		t.Fatalf("kind=%v", term.SemanticKind())
	}
}
