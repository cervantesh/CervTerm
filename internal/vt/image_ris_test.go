package vt

import (
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

func TestRISResetsImageEpochAndParserRemainsReusable(t *testing.T) {
	process := termimage.NewProcessBudget()
	store := termimage.NewStore(process, termimage.DefaultLimits())
	terminal := core.NewTerminal(4, 2)
	if err := terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.WriteRGBAAt(0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if _, err = terminal.CommitImage(core.ImageCommit{Candidate: candidate, Placement: &termimage.PlacementSpec{ID: 1, Cols: 1, Rows: 1}}); err != nil {
		t.Fatal(err)
	}
	epoch := store.Epoch()
	var parser Parser
	parser.Advance(terminal, []byte{0x1b, 'c', 'X'})
	if store.Epoch() == epoch || store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
		t.Fatalf("epoch=%d usage=%#v", store.Epoch(), store.Usage())
	}
	view := make([]core.Cell, terminal.Cols()*terminal.Rows())
	terminal.CopyView(view)
	if got := view[0].Rune; got != 'X' {
		t.Fatalf("parser reuse rune=%q", got)
	}
}
