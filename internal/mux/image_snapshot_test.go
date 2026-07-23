package mux

import (
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/termimage"
)

func TestPaneViewDeepDetachesImageAndSnapshotMetadata(t *testing.T) {
	mux, _, _ := newTestMux(t)
	pane := lookupPaneForTest(t, mux.sessions, 1)
	store := termimage.NewStore(termimage.NewProcessBudget(), termimage.DefaultLimits())
	if err := pane.terminal.AttachImageStore(store); err != nil {
		t.Fatal(err)
	}
	candidate, err := store.NewDecodedCandidate(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.WriteRGBAAt(0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	crop := termimage.PixelRect{Width: 1, Height: 1}
	if _, err = pane.terminal.CommitImage(core.ImageCommit{Candidate: candidate, Placement: &termimage.PlacementSpec{ID: 1, Anchor: termimage.CellAnchor{Row: 1}, Cols: 1, Rows: 1, Crop: &crop}}); err != nil {
		t.Fatal(err)
	}
	pane.terminal.PutRune('a')
	pane.terminal.PutRune('\u0301')
	pane.capture()
	view, ok := mux.PaneView(1)
	if !ok || len(view.Snapshot.Images) != 1 || view.Snapshot.Images[0].PaneObject != 1 || view.Snapshot.ImageGeneration == 0 {
		t.Fatalf("view=%#v", view.Snapshot.Images)
	}
	view.Snapshot.Images[0].Placement.Anchor.Row = 99
	view.Snapshot.Images[0].Placement.Crop.Width = 99
	view.Snapshot.Cells[0].AppendCombining('x')
	view.Snapshot.Wrapped[0] = !view.Snapshot.Wrapped[0]
	again, _ := mux.PaneView(1)
	if again.Snapshot.Images[0].Placement.Anchor.Row != 1 || again.Snapshot.Images[0].Placement.Crop.Width != 1 {
		t.Fatal("PaneView image metadata aliases pane snapshot")
	}
	if len(again.Snapshot.Cells[0].Combining()) != 1 {
		t.Fatal("PaneView combining storage aliases pane snapshot")
	}
}
