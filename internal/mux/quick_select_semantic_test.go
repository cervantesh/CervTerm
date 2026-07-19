package mux

import (
	"testing"

	"cervterm/internal/core"
)

func TestQuickSelectSnapshotDetachesSemanticZones(t *testing.T) {
	m, _, _ := newTestMux(t)
	p := lookupPaneForTest(t, m.sessions, 1)
	p.advanceTerminal([]byte("\x1b]133;A\x1b\\P\x1b]133;B\x1b\\I"))
	p.capture()
	snapshot, ok := m.QuickSelectSnapshot(1, 0, 0)
	if !ok || snapshot.SemanticZonesTruncated || len(snapshot.SemanticZones) != 2 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if snapshot.Cells[0].SemanticKind != core.SemanticPrompt || snapshot.Cells[1].SemanticKind != core.SemanticInput {
		t.Fatalf("cells=%#v", snapshot.Cells[:2])
	}
	snapshot.Cells[0].SemanticKind = core.SemanticOutput
	snapshot.SemanticZones[0].Kind = core.SemanticOutput
	again, _ := m.QuickSelectSnapshot(1, 0, 0)
	if again.Cells[0].SemanticKind != core.SemanticPrompt || again.SemanticZones[0].Kind != core.SemanticPrompt {
		t.Fatal("snapshot aliases pane metadata")
	}
}
