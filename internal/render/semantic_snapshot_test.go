package render

import (
	"testing"

	"cervterm/internal/core"
)

func TestSnapshotProjectsDetachedSemanticZonesAndDamage(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.SetSemanticKind(core.SemanticPrompt)
	term.PutRune('P')
	term.SetSemanticKind(core.SemanticInput)
	term.PutRune('I')
	var snapshot Snapshot
	Capture(&snapshot, term)
	if snapshot.SemanticZonesTruncated || len(snapshot.SemanticZones) != 2 || snapshot.SemanticZones[0].Kind != core.SemanticPrompt || snapshot.SemanticZones[1].Kind != core.SemanticInput {
		t.Fatalf("zones=%#v", snapshot.SemanticZones)
	}
	snapshot.SemanticZones[0].Kind = core.SemanticOutput
	Capture(&snapshot, term)
	if snapshot.SemanticZones[0].Kind != core.SemanticPrompt {
		t.Fatal("snapshot zones alias prior projection")
	}
	cells := []core.Cell{{Rune: 'x'}, {Rune: 'x', SemanticKind: core.SemanticPrompt}}
	hashes := make([]uint64, 2)
	HashRows(hashes[:1], cells[:1], 1)
	HashRows(hashes[1:], cells[1:], 1)
	if hashes[0] == hashes[1] {
		t.Fatal("semantic-only change did not damage row")
	}
}
