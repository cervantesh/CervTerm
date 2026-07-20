package render

import (
	"testing"

	"cervterm/internal/core"
)

func TestSnapshotProjectsOnlyVisibleResolvableHyperlinks(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.OpenHyperlink("https://one.test", "")
	term.PutRune('A')
	term.CloseHyperlink()
	term.OpenHyperlink("https://two.test", "")
	term.PutRune('B')
	var snapshot Snapshot
	Capture(&snapshot, term)
	if len(snapshot.Hyperlinks) != 2 || snapshot.Hyperlinks[0].URI != "https://one.test" || snapshot.Hyperlinks[1].URI != "https://two.test" {
		t.Fatalf("links=%#v", snapshot.Hyperlinks)
	}
	snapshot.Hyperlinks[0].URI = "mutated"
	Capture(&snapshot, term)
	if snapshot.Hyperlinks[0].URI != "https://one.test" {
		t.Fatal("snapshot mutation reached terminal")
	}
}

func TestHyperlinkIdentityParticipatesInRowDamage(t *testing.T) {
	cells := []core.Cell{{Rune: 'x'}, {Rune: 'x', HyperlinkID: 1}}
	hashes := make([]uint64, 2)
	HashRows(hashes[:1], cells[:1], 1)
	HashRows(hashes[1:], cells[1:], 1)
	if hashes[0] == hashes[1] {
		t.Fatal("hyperlink-only change did not damage row")
	}
}
