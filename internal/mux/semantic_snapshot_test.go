package mux

import (
	"errors"
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

func TestSemanticRangeTextRequiresSnapshotMembershipAndFreshness(t *testing.T) {
	m, _, _ := newTestMux(t)
	p := lookupPaneForTest(t, m.sessions, 1)
	p.advanceTerminal([]byte("\x1b]133;B\x1b\\echo hi"))
	p.capture()
	snapshot, _ := m.SemanticSnapshot(1)
	text, err := m.SemanticRangeText(snapshot, snapshot.Ranges[0])
	if err != nil || text != "echo hi" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	forged := snapshot.Ranges[0]
	forged.Start.Col++
	if _, err := m.SemanticRangeText(snapshot, forged); !errors.Is(err, ErrSemanticRangeUnavailable) {
		t.Fatalf("forged err=%v", err)
	}
	forgedSnapshot := snapshot
	forgedSnapshot.Ranges = []core.SemanticRange{forged}
	if _, err := m.SemanticRangeText(forgedSnapshot, forged); !errors.Is(err, ErrSemanticRangeUnavailable) {
		t.Fatalf("forged snapshot err=%v", err)
	}
	p.advanceTerminal([]byte("x"))
	p.capture()
	if _, err := m.SemanticRangeText(snapshot, snapshot.Ranges[0]); !errors.Is(err, ErrSemanticSnapshotStale) {
		t.Fatalf("stale err=%v", err)
	}
}
