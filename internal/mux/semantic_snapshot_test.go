package mux

import (
	"testing"

	"cervterm/internal/core"
)

func TestSemanticSnapshotDetachedAndGenerationChecked(t *testing.T) {
	m, _, _ := newTestMux(t)
	p := lookupPaneForTest(t, m.sessions, 1)
	p.advanceTerminal([]byte("\x1b]133;A\x1b\\P\x1b]133;B\x1b\\I"))
	p.capture()
	snapshot, ok := m.SemanticSnapshot(1)
	if !ok || len(snapshot.Ranges) != 2 || snapshot.Ranges[0].Kind != core.SemanticPrompt || !m.SemanticSnapshotCurrent(snapshot) {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	snapshot.Ranges[0].Kind = core.SemanticOutput
	again, _ := m.SemanticSnapshot(1)
	if again.Ranges[0].Kind != core.SemanticPrompt {
		t.Fatal("snapshot aliases terminal history")
	}
	p.advanceTerminal([]byte("x"))
	p.capture()
	if m.SemanticSnapshotCurrent(snapshot) {
		t.Fatal("content mutation did not stale snapshot")
	}
}
