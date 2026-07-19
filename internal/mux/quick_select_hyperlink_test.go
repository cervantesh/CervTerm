package mux

import "testing"

func TestQuickSelectSnapshotDetachesHyperlinkMetadata(t *testing.T) {
	m, _, _ := newTestMux(t)
	p := lookupPaneForTest(t, m.sessions, 1)
	p.advanceTerminal([]byte("\x1b]8;id=x;https://example.test\x1b\\link\x1b]8;;\x1b\\"))
	p.capture()
	snapshot, ok := m.QuickSelectSnapshot(1, 0, 0)
	if !ok || len(snapshot.Hyperlinks) != 1 || snapshot.Hyperlinks[0].URI != "https://example.test" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	id := snapshot.Cells[0].HyperlinkID
	if id == 0 {
		t.Fatal("cell identity missing")
	}
	snapshot.Hyperlinks[0].URI = "mutated"
	snapshot.Cells[0].HyperlinkID = 0
	again, _ := m.QuickSelectSnapshot(1, 0, 0)
	if again.Hyperlinks[0].URI != "https://example.test" || again.Cells[0].HyperlinkID != id {
		t.Fatal("snapshot aliases pane state")
	}
}
